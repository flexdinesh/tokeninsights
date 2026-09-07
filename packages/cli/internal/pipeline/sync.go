package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

const (
	defaultCollector       = "tokeninsights-sync-go"
	defaultParser          = "sync-first-v1"
	opencodeSQLiteParserV2 = "opencode-sqlite-v1-v2"
)

var runSequence atomic.Uint64

type sqlRunner interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func Sync(ctx context.Context, options SyncOptions) (Summary, error) {
	if options.Collector == "" {
		options.Collector = defaultCollector
	}
	if options.Parser == "" {
		options.Parser = defaultParser
	}
	if options.Normalize && options.DryRun {
		options.Normalize = false
	}

	summary := Summary{RequestedHarnesses: len(options.Harnesses)}
	if options.DryRun {
		return dryRunSync(ctx, options)
	}

	database, created, err := db.CreateIfMissing(options.DBPath)
	if err != nil {
		return summary, err
	}
	defer database.Close()
	if created {
		summary.Skipped = 0
	}

	for _, harness := range options.Harnesses {
		reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressDiscovering})
		harnessSummary, err := syncHarness(ctx, database, options, harness)
		mergeSummary(&summary, harnessSummary)
		if err != nil {
			reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressFailed})
			summary.Failed++
			summary.Errors = append(summary.Errors, err)
			continue
		}
		if harnessSummary.RawFacts == 0 && harnessSummary.Observations == 0 {
			reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressSkipped})
			summary.Skipped++
			continue
		}
		reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressSynced})
		summary.Synced++
	}

	shouldNormalize := false
	if options.Normalize && summary.Observations > 0 {
		shouldNormalize = true
	} else if options.Normalize {
		hasPendingWork, err := hasPendingNormalizationWork(ctx, database, options.Harnesses)
		if err != nil {
			summary.Errors = append(summary.Errors, err)
		}
		shouldNormalize = hasPendingWork
	}

	if shouldNormalize {
		reportSyncProgress(options, SyncProgressEvent{Status: SyncProgressNormalizing})
		normalSummary, err := Normalize(ctx, NormalizeOptions{
			DBPath:    options.DBPath,
			Harnesses: options.Harnesses,
			Now:       options.Now,
		})
		mergeSummary(&summary, normalSummary)
		if err != nil {
			summary.Errors = append(summary.Errors, err)
		}
	}

	return summary, errors.Join(summary.Errors...)
}

func hasPendingNormalizationWork(ctx context.Context, database *sql.DB, harnesses []Harness) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM normalization_work_queue q
		JOIN raw_token_usage r ON r.id = q.raw_fact_id
		WHERE q.domain = ?
	`
	args := []interface{}{db.DomainTokenUsage}
	if len(harnesses) > 0 {
		placeholders := make([]string, 0, len(harnesses))
		for _, harness := range harnesses {
			placeholders = append(placeholders, "?")
			args = append(args, harness)
		}
		query += " AND r.harness IN (" + strings.Join(placeholders, ", ") + ")"
	}
	var count int
	if err := database.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func dryRunSync(ctx context.Context, options SyncOptions) (Summary, error) {
	var summary Summary
	summary.RequestedHarnesses = len(options.Harnesses)
	stateDB := openDryRunSourceRefreshDB(options.DBPath)
	if stateDB != nil {
		defer stateDB.Close()
	}
	for _, harness := range options.Harnesses {
		harnessOptions := parserOptionsForHarness(options, harness)
		adapter, ok := AdapterFor(harness)
		if !ok {
			summary.Failed++
			summary.Errors = append(summary.Errors, fmt.Errorf("unsupported harness %q", harness))
			continue
		}
		sources, err := adapter.Discover(ctx, discoverOptions(harnessOptions))
		if err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, fmt.Errorf("%s discover: %w", harness, err))
			continue
		}
		if len(sources) == 0 {
			summary.Skipped++
			continue
		}
		parsedSources := 0
		harnessFailed := false
		for _, source := range sources {
			if dryRunSourceIsUpToDate(ctx, stateDB, source, harnessOptions) {
				continue
			}
			facts, diagnostics, err := adapter.Parse(ctx, source, harnessOptions)
			if err != nil {
				harnessFailed = true
				summary.Failed++
				summary.Errors = append(summary.Errors, fmt.Errorf("%s parse: %w", harness, err))
				continue
			}
			parsedSources++
			summary.RawFacts += len(facts)
			summary.Diagnostics += len(diagnostics)
		}
		if parsedSources > 0 {
			summary.Synced++
		} else if !harnessFailed {
			summary.Skipped++
		}
	}
	return summary, errors.Join(summary.Errors...)
}

func openDryRunSourceRefreshDB(dbPath string) *sql.DB {
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return nil
	}
	return database
}

func dryRunSourceIsUpToDate(ctx context.Context, database *sql.DB, source Source, options SyncOptions) bool {
	if database == nil {
		return false
	}
	metadata, ok := sourceRefreshMetadataFor(source)
	if !ok {
		return false
	}
	skip, err := shouldSkipSourceRefresh(ctx, database, source, options, metadata, ok)
	return err == nil && skip
}

func syncHarness(ctx context.Context, database *sql.DB, options SyncOptions, harness Harness) (Summary, error) {
	var summary Summary
	options = parserOptionsForHarness(options, harness)
	adapter, ok := AdapterFor(harness)
	if !ok {
		return summary, fmt.Errorf("unsupported harness %q", harness)
	}

	sources, err := adapter.Discover(ctx, discoverOptions(options))
	if err != nil {
		return summary, fmt.Errorf("%s discover: %w", harness, err)
	}
	if len(sources) == 0 {
		return summary, nil
	}

	seenDedupeKeys := map[string]bool{}
	reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressSyncing})
	for _, source := range sources {
		sourceSummary, err := ingestSource(ctx, database, adapter, options, harness, source, seenDedupeKeys)
		if err != nil {
			return summary, err
		}
		mergeSummary(&summary, sourceSummary)
	}
	return summary, nil
}

func parserOptionsForHarness(options SyncOptions, harness Harness) SyncOptions {
	if harness == HarnessOpenCode && options.Parser == defaultParser {
		options.Parser = opencodeSQLiteParserV2
	}
	return options
}

func reportSyncProgress(options SyncOptions, event SyncProgressEvent) {
	if options.Progress != nil {
		options.Progress(event)
	}
}

func ingestSource(ctx context.Context, database *sql.DB, adapter Adapter, options SyncOptions, harness Harness, source Source, seenDedupeKeys map[string]bool) (Summary, error) {
	var summary Summary
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return summary, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	runID := newRunID(harness, source.ID, options.Now)
	runDBID, err := createIngestRun(ctx, tx, runID, source, options)
	if err != nil {
		return summary, err
	}
	refreshMetadata, hasRefreshMetadata := sourceRefreshMetadataFor(source)
	skipSource, err := shouldSkipSourceRefresh(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata)
	if err != nil {
		return summary, err
	}
	if skipSource {
		if err := upsertSourceRefreshState(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata); err != nil {
			return summary, err
		}
		if err := completeIngestRun(ctx, tx, runDBID, "completed", "", Summary{}, syncNowMs(options.Now)); err != nil {
			return summary, err
		}
		if err := tx.Commit(); err != nil {
			return summary, err
		}
		committed = true
		return summary, nil
	}
	if err := createSourceWriteSavepoint(ctx, tx); err != nil {
		return summary, err
	}

	facts, diagnostics, parseErr := adapter.Parse(ctx, source, options)
	if parseErr != nil {
		if err := completeIngestRun(ctx, tx, runDBID, "failed", parseErr.Error(), Summary{}, syncNowMs(options.Now)); err != nil {
			return summary, err
		}
		if err := tx.Commit(); err != nil {
			return summary, err
		}
		committed = true
		return summary, fmt.Errorf("%s parse: %w", harness, parseErr)
	}
	facts, duplicateDiagnostics := suppressDuplicateFacts(facts, seenDedupeKeys)
	diagnostics = append(diagnostics, duplicateDiagnostics...)

	sourceSummary, err := writeSourceIngest(ctx, tx, runDBID, facts, diagnostics, options)
	if err != nil {
		if rollbackErr := rollbackSourceWrites(ctx, tx); rollbackErr != nil {
			return summary, errors.Join(err, rollbackErr)
		}
		if completeErr := completeIngestRun(ctx, tx, runDBID, "failed", err.Error(), Summary{}, syncNowMs(options.Now)); completeErr != nil {
			return summary, errors.Join(err, completeErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return summary, errors.Join(err, commitErr)
		}
		committed = true
		return summary, err
	}
	if err := releaseSourceWriteSavepoint(ctx, tx); err != nil {
		return summary, err
	}
	if err := upsertSourceRefreshState(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata); err != nil {
		return summary, err
	}
	if err := completeIngestRun(ctx, tx, runDBID, "completed", "", sourceSummary, syncNowMs(options.Now)); err != nil {
		return summary, err
	}
	if err := tx.Commit(); err != nil {
		return summary, err
	}
	committed = true
	return sourceSummary, nil
}

func suppressDuplicateFacts(facts []RawTokenFact, seen map[string]bool) ([]RawTokenFact, []Diagnostic) {
	if len(facts) == 0 {
		return facts, nil
	}
	filtered := make([]RawTokenFact, 0, len(facts))
	var diagnostics []Diagnostic
	for _, fact := range facts {
		if fact.DedupeKey == "" {
			filtered = append(filtered, fact)
			continue
		}
		if seen[fact.DedupeKey] {
			diagnostics = append(diagnostics, duplicateSuppressedDiagnostic(fact.Harness))
			continue
		}
		seen[fact.DedupeKey] = true
		filtered = append(filtered, fact)
	}
	return filtered, diagnostics
}

func duplicateSuppressedDiagnostic(harness Harness) Diagnostic {
	switch harness {
	case HarnessClaudeCode:
		return Diagnostic{
			Harness:  harness,
			Severity: "info",
			Code:     "claude_code_jsonl_duplicate_suppressed",
			Message:  "suppressed duplicate Claude Code assistant token row from copied transcript",
		}
	default:
		return Diagnostic{
			Harness:  harness,
			Severity: "info",
			Code:     "opencode_sqlite_duplicate_suppressed",
			Message:  "suppressed duplicate OpenCode assistant token row from channel or fork copy",
		}
	}
}

func writeSourceIngest(ctx context.Context, runner sqlRunner, runDBID int64, facts []RawTokenFact, diagnostics []Diagnostic, options SyncOptions) (Summary, error) {
	sourceSummary := Summary{}
	for _, fact := range facts {
		rawID, inserted, err := upsertRawTokenFact(ctx, runner, fact)
		if err != nil {
			return sourceSummary, err
		}
		if inserted {
			sourceSummary.RawFacts++
			if err := enqueueNormalizationWork(ctx, runner, rawID, db.DomainTokenUsage, syncNowMs(options.Now)); err != nil {
				return sourceSummary, err
			}
		}
		observed, err := insertObservation(ctx, runner, runDBID, rawID, fact)
		if err != nil {
			return sourceSummary, err
		}
		if observed {
			sourceSummary.Observations++
		}
	}

	for _, diagnostic := range diagnostics {
		inserted, err := insertDiagnostic(ctx, runner, diagnostic, nil, &runDBID, syncNowMs(options.Now))
		if err != nil {
			return sourceSummary, err
		}
		if inserted {
			sourceSummary.Diagnostics++
		}
	}
	return sourceSummary, nil
}

func enqueueNormalizationWork(ctx context.Context, runner sqlRunner, rawID int64, domain string, enqueuedAtMs int64) error {
	_, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO normalization_work_queue (
			raw_fact_id, domain, enqueued_at_ms
		) VALUES (?, ?, ?)
	`, rawID, domain, enqueuedAtMs)
	return err
}

func createSourceWriteSavepoint(ctx context.Context, runner sqlRunner) error {
	_, err := runner.ExecContext(ctx, "SAVEPOINT source_ingest_writes")
	return err
}

func rollbackSourceWrites(ctx context.Context, runner sqlRunner) error {
	if _, err := runner.ExecContext(ctx, "ROLLBACK TO source_ingest_writes"); err != nil {
		return err
	}
	_, err := runner.ExecContext(ctx, "RELEASE source_ingest_writes")
	return err
}

func releaseSourceWriteSavepoint(ctx context.Context, runner sqlRunner) error {
	_, err := runner.ExecContext(ctx, "RELEASE source_ingest_writes")
	return err
}

func newRunID(harness Harness, sourceID string, now time.Time) string {
	startedAt := now.UnixNano()
	if now.IsZero() {
		startedAt = time.Now().UnixNano()
	}
	sequence := runSequence.Add(1)
	return stableHash(fmt.Sprintf("%s:%s:%d:%d", harness, sourceID, startedAt, sequence))
}

func createIngestRun(ctx context.Context, runner sqlRunner, runID string, source Source, options SyncOptions) (int64, error) {
	result, err := runner.ExecContext(ctx, `
		INSERT INTO ingest_runs (
			run_id, harness, collector, parser, source_id, source_kind, status, started_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, 'running', ?)
	`, runID, source.Harness, options.Collector, options.Parser, source.ID, source.Kind, syncNowMs(options.Now))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func completeIngestRun(ctx context.Context, runner sqlRunner, runID int64, status string, message string, summary Summary, completedAtMs int64) error {
	_, err := runner.ExecContext(ctx, `
		UPDATE ingest_runs
		SET status = ?,
			completed_at_ms = CASE WHEN completed_at_ms IS NULL THEN ? ELSE completed_at_ms END,
			error_message = NULLIF(?, ''),
			raw_fact_count = ?,
			observation_count = ?,
			canonical_count = ?,
			diagnostic_count = ?
		WHERE id = ? AND status = 'running'
	`, status, completedAtMs, message, summary.RawFacts, summary.Observations, summary.Canonical, summary.Diagnostics, runID)
	return err
}

func upsertRawTokenFact(ctx context.Context, runner sqlRunner, fact RawTokenFact) (int64, bool, error) {
	key := rawFactKey(fact)
	result, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO raw_token_usage (
			raw_fact_key, harness, source_id, source_kind, collector, parser, observed_at_ms, occurred_at_ms,
			session_id, message_id, provider, model, usage_scope, quality,
			input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, key, fact.Harness, fact.SourceID, fact.SourceKind, fact.Collector, fact.Parser, fact.ObservedAtMs, nullableInt(fact.OccurredAtMs),
		nullableString(fact.SessionID), nullableString(fact.MessageID), nullableString(fact.Provider), nullableString(fact.Model), fact.UsageScope, fact.Quality,
		nullableInt(fact.InputTokens), nullableInt(fact.OutputTokens), nullableInt(fact.ReasoningTokens), nullableInt(fact.CacheReadTokens), nullableInt(fact.CacheWriteTokens), nullableInt(fact.TotalTokens), nullableString(fact.MetadataJSON))
	if err != nil {
		return 0, false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	var id int64
	if err := runner.QueryRowContext(ctx, "SELECT id FROM raw_token_usage WHERE raw_fact_key = ?", key).Scan(&id); err != nil {
		return 0, false, err
	}
	return id, inserted > 0, nil
}

func insertObservation(ctx context.Context, runner sqlRunner, runID int64, rawID int64, fact RawTokenFact) (bool, error) {
	key := stableHash(fmt.Sprintf("%d:%s", runID, rawFactKey(fact)))
	result, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO raw_observations (
			ingest_run_id, raw_fact_id, observed_at_ms, observation_key
		) VALUES (?, ?, ?, ?)
	`, runID, rawID, fact.ObservedAtMs, key)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func rawFactKey(fact RawTokenFact) string {
	parts := []string{
		string(fact.Harness),
		fact.SourceID,
		stringValueOrEmpty(fact.SessionID),
		stringValueOrEmpty(fact.MessageID),
		fmt.Sprint(intValueOrZero(fact.OccurredAtMs)),
		fact.UsageScope,
		fmt.Sprint(intValueOrZero(fact.InputTokens)),
		fmt.Sprint(intValueOrZero(fact.OutputTokens)),
		fmt.Sprint(intValueOrZero(fact.ReasoningTokens)),
		fmt.Sprint(intValueOrZero(fact.CacheReadTokens)),
		fmt.Sprint(intValueOrZero(fact.CacheWriteTokens)),
		fmt.Sprint(intValueOrZero(fact.TotalTokens)),
		fact.Parser,
	}
	return stableHash(strings.Join(parts, "|"))
}

func nullableString(value *string) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func stringValueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func syncNowMs(now time.Time) int64 {
	if now.IsZero() {
		return time.Now().UnixMilli()
	}
	return now.UnixMilli()
}

func discoverOptions(options SyncOptions) DiscoverOptions {
	return DiscoverOptions{
		SourceDir:         options.SourceDir,
		HarnessSubdirOnly: strings.TrimSpace(options.SourceDir) != "" && len(options.Harnesses) > 1,
	}
}

func mergeSummary(target *Summary, source Summary) {
	target.RequestedHarnesses += source.RequestedHarnesses
	target.Synced += source.Synced
	target.Skipped += source.Skipped
	target.Failed += source.Failed
	target.RawFacts += source.RawFacts
	target.Observations += source.Observations
	target.Canonical += source.Canonical
	target.Diagnostics += source.Diagnostics
	target.Errors = append(target.Errors, source.Errors...)
}
