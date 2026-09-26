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
	// Any parser change that can alter emitted facts or identity must also bump
	// db.CurrentDataGeneration so recovery replaces, rather than mixes, raw data.
	defaultCollector        = "tokeninsights-sync-go"
	defaultParser           = "sync-first-v1"
	opencodeSQLiteParserV2  = "opencode-sqlite-v1-v2"
	piJSONLParserV2         = "pi-jsonl-token-semantics-v2"
	codexJSONLParserV3      = "codex-jsonl-replay-v3"
	claudeCodeJSONLParserV2 = "claude-code-jsonl-token-semantics-v2"
)

var runSequence atomic.Uint64

type sqlRunner interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func Sync(ctx context.Context, options SyncOptions) (Summary, error) {
	options = defaultSyncOptions(options)
	if options.DryRun && strings.TrimSpace(options.DBPath) == "" {
		return dryRunSync(ctx, options)
	}
	compatibility, err := db.InspectCompatibility(ctx, options.DBPath)
	if err != nil {
		return Summary{}, err
	}
	if options.DryRun {
		return previewSync(ctx, options, compatibility)
	}
	if err := validateRecoveryScope(options, compatibility); err != nil {
		return Summary{}, err
	}
	reportSyncProgress(options, SyncProgressEvent{Status: SyncProgressWaiting})
	release, err := db.AcquireWriterLock(ctx, options.DBPath)
	if err != nil {
		return Summary{}, recoveryFailure(compatibility, err)
	}
	defer release()
	compatibility, err = db.InspectCompatibility(ctx, options.DBPath)
	if err != nil {
		return Summary{}, err
	}
	if compatibility.MigrationRequired {
		if err := db.UpgradeMetadata(ctx, options.DBPath); err != nil {
			return Summary{}, err
		}
		compatibility, err = db.InspectCompatibility(ctx, options.DBPath)
		if err != nil {
			return Summary{}, err
		}
	}
	if needsRecovery(compatibility) {
		if err := validateRecoveryScope(options, compatibility); err != nil {
			return Summary{}, err
		}
		return recoverDatabase(ctx, options, compatibility)
	}
	return syncPrepared(ctx, options)
}

func defaultSyncOptions(options SyncOptions) SyncOptions {
	if options.hostname == "" {
		hostname, err := os.Hostname()
		if err == nil {
			options.hostname = strings.TrimSpace(hostname)
		}
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.locationResolver == nil {
		options.locationResolver = &locationResolver{}
	}
	if options.Collector == "" {
		options.Collector = defaultCollector
	}
	if options.Parser == "" {
		options.Parser = defaultParser
	}
	if options.Normalize && options.DryRun {
		options.Normalize = false
	}
	return options
}

// syncPrepared runs under the caller's writer lock, including during recovery.
func syncPrepared(ctx context.Context, options SyncOptions) (summary Summary, resultErr error) {
	summary = Summary{RequestedHarnesses: len(options.Harnesses)}
	database, _, err := db.CreateIfMissing(options.DBPath)
	if err != nil {
		return summary, err
	}
	defer func() { _ = database.Close() }()
	options.jobID, err = startSyncJob(ctx, database, options)
	if err != nil {
		return summary, err
	}
	defer func() { resultErr = errors.Join(resultErr, finishSyncJob(database, options, resultErr)) }()
	plans := make(map[Harness]harnessPlan, len(options.Harnesses))
	for _, harness := range options.Harnesses {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if err := setHarnessStatus(ctx, database, options, harness, SyncProgressDiscovering); err != nil {
			return summary, err
		}
		reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressDiscovering})
		adapter, ok := AdapterFor(harness)
		plan := harnessPlan{adapter: adapter}
		if !ok {
			plan.err = fmt.Errorf("unsupported harness %q", harness)
		} else {
			plan.sources, plan.err = adapter.Discover(ctx, discoverOptions(parserOptionsForHarness(options, harness)))
		}
		if plan.err == nil {
			if err := recordDiscoveredSources(ctx, database, options, harness, plan.sources); err != nil {
				return summary, err
			}
		}
		plans[harness] = plan
	}
	for _, harness := range options.Harnesses {
		if err := ctx.Err(); err != nil {
			return summary, errors.Join(errors.Join(summary.Errors...), err)
		}
		harnessSummary, harnessErr := syncHarness(ctx, database, options, harness, plans[harness])
		mergeSummary(&summary, harnessSummary)
		if harnessErr != nil {
			var recoverable sourceFailure
			if !errors.As(harnessErr, &recoverable) || ctx.Err() != nil {
				return summary, errors.Join(errors.Join(summary.Errors...), harnessErr)
			}
		}
		published := false
		if options.Normalize {
			pending, err := hasPendingNormalizationWork(ctx, database, []Harness{harness})
			if err != nil {
				return summary, err
			}
			if pending {
				if err := setHarnessStatus(ctx, database, options, harness, SyncProgressNormalizing); err != nil {
					return summary, err
				}
				reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressNormalizing})
			}
			normalSummary, err := normalizePrepared(ctx, database, NormalizeOptions{DBPath: options.DBPath, Harnesses: []Harness{harness}, Now: options.Now})
			mergeSummary(&summary, normalSummary)
			if err != nil {
				return summary, errors.Join(harnessErr, err)
			}
			if err := publishHarnessSources(ctx, database, options, harness); err != nil {
				return summary, err
			}
			published = pending || normalSummary.Canonical > 0
		}
		status := SyncProgressSynced
		if harnessErr != nil {
			status = SyncProgressFailed
			summary.Failed++
			summary.Errors = append(summary.Errors, harnessErr)
		} else if harnessSummary.RawFacts == 0 && harnessSummary.Observations == 0 {
			status = SyncProgressSkipped
			summary.Skipped++
		} else {
			summary.Synced++
		}
		if err := setHarnessStatus(ctx, database, options, harness, status); err != nil {
			return summary, err
		}
		reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: status, Published: published})
	}
	resultErr = errors.Join(summary.Errors...)
	if resultErr == nil && options.recovering {
		resultErr = db.CompleteRecovery(ctx, database)
	}
	return summary, resultErr
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
		defer func() { _ = stateDB.Close() }()
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
			if source.Harness == HarnessPi {
				if dryRunSourceIsUpToDate(ctx, stateDB, source, harnessOptions) {
					continue
				}
			} else if stateDB != nil {
				metadata, hasMetadata := sourceRefreshMetadataFor(source)
				var skip bool
				_, skip, err = planSourceReuse(ctx, stateDB, source, harnessOptions, metadata, hasMetadata)
				if err != nil {
					return summary, err
				}
				if skip {
					continue
				}
			}
			var facts []RawTokenFact
			var diagnostics []Diagnostic
			var err error
			if cursorAdapter, ok := adapter.(byteCursorAdapter); ok && stateDB != nil {
				metadata, hasMetadata := sourceRefreshMetadataFor(source)
				offset, skip, cursorErr := planPiCursor(ctx, stateDB, source, harnessOptions, metadata, hasMetadata)
				if cursorErr != nil {
					return summary, cursorErr
				}
				if skip {
					continue
				}
				facts, diagnostics, _, err = cursorAdapter.ParseFrom(ctx, source, harnessOptions, offset)
			} else {
				facts, diagnostics, err = adapter.Parse(ctx, source, harnessOptions)
			}
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

type harnessPlan struct {
	adapter Adapter
	sources []Source
	err     error
}

func syncHarness(ctx context.Context, database *sql.DB, options SyncOptions, harness Harness, plan harnessPlan) (Summary, error) {
	var summary Summary
	options = parserOptionsForHarness(options, harness)
	if plan.err != nil {
		return summary, sourceFailure{err: fmt.Errorf("%s discover: %w", harness, plan.err)}
	}
	adapter, sources := plan.adapter, plan.sources
	if len(sources) == 0 {
		return summary, nil
	}
	seenDedupeKeys := map[string]int64{}
	if err := setHarnessStatus(ctx, database, options, harness, SyncProgressSyncing); err != nil {
		return summary, err
	}
	reportSyncProgress(options, SyncProgressEvent{Harness: harness, Status: SyncProgressSyncing})
	var failures []error
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if err := updateSourceStatus(ctx, database, options, source, "reading", "", nil, nil); err != nil {
			return summary, err
		}
		sourceSummary, ingestErr := ingestSource(ctx, database, adapter, options, harness, source, seenDedupeKeys)
		if ingestErr != nil {
			var recoverable sourceFailure
			if !errors.As(ingestErr, &recoverable) || ctx.Err() != nil {
				return summary, ingestErr
			}
			failures = append(failures, ingestErr)
		}
		if err := completeSourceAttempt(ctx, database, options, source, ingestErr); err != nil {
			return summary, err
		}
		mergeSummary(&summary, sourceSummary)
	}
	if len(failures) > 0 {
		return summary, sourceFailure{err: errors.Join(failures...)}
	}

	return summary, nil
}

func parserOptionsForHarness(options SyncOptions, harness Harness) SyncOptions {
	if harness == HarnessOpenCode && options.Parser == defaultParser {
		options.Parser = opencodeSQLiteParserV2
	}
	if harness == HarnessPi && options.Parser == defaultParser {
		options.Parser = piJSONLParserV2
	}
	if harness == HarnessCodex && options.Parser == defaultParser {
		options.Parser = codexJSONLParserV3
	}
	if harness == HarnessClaudeCode && options.Parser == defaultParser {
		options.Parser = claudeCodeJSONLParserV2
	}
	return options
}

func reportSyncProgress(options SyncOptions, event SyncProgressEvent) {
	if options.Progress != nil {
		options.Progress(event)
	}
}

func ingestSource(ctx context.Context, database *sql.DB, adapter Adapter, options SyncOptions, harness Harness, source Source, seenDedupeKeys map[string]int64) (Summary, error) {
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
	var skipSource bool
	var reuseFingerprint sourceFingerprint
	if source.Harness == HarnessPi {
		skipSource, err = shouldSkipSourceRefresh(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata)
	} else {
		reuseFingerprint, skipSource, err = planSourceReuse(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata)
	}
	if err != nil {
		return summary, err
	}
	cursorAdapter, cursorCapable := adapter.(byteCursorAdapter)
	var cursorOffset int64
	if !skipSource && cursorCapable {
		cursorOffset, skipSource, err = planPiCursor(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata)
		if err != nil {
			return summary, err
		}
	}
	if skipSource {
		if err := updateSourceStatus(ctx, tx, options, source, "unchanged", "", nil, nil); err != nil {
			return summary, err
		}
		if err := upsertSourceRefreshState(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata); err != nil {
			return summary, err
		}
		if err := completeIngestRun(ctx, tx, runDBID, "completed", "", Summary{}, syncWallNow(options).UnixMilli()); err != nil {
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

	var facts []RawTokenFact
	var diagnostics []Diagnostic
	var cursorEligible bool
	var parseErr error
	if cursorCapable {
		facts, diagnostics, cursorEligible, parseErr = cursorAdapter.ParseFrom(ctx, source, options, cursorOffset)
	} else {
		facts, diagnostics, parseErr = adapter.Parse(ctx, source, options)
	}
	if parseErr != nil {
		if err := completeIngestRun(ctx, tx, runDBID, "failed", parseErr.Error(), Summary{}, syncWallNow(options).UnixMilli()); err != nil {
			return summary, err
		}
		if err := tx.Commit(); err != nil {
			return summary, err
		}
		committed = true
		return summary, sourceFailure{err: fmt.Errorf("%s parse: %w", harness, parseErr)}
	}
	sourceStatus := "ingested"
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "jsonl_incomplete_tail" {
			sourceStatus = "deferred"
		}
	}
	if source.Harness != HarnessOpenCode && hasRefreshMetadata {
		current, valid := sourceRefreshMetadataFor(source)
		if !valid || current.mtimeMs != refreshMetadata.mtimeMs || current.sizeBytes != refreshMetadata.sizeBytes {
			sourceStatus = "deferred"
			diagnostics = append(diagnostics, Diagnostic{Harness: harness, Severity: "info", Code: "jsonl_source_changed", Message: "source changed during snapshot read; new records await next sync"})
		}
	}
	sourceDedupeKeys := make(map[string]int64, len(facts))
	for _, fact := range facts {
		if id, ok := seenDedupeKeys[fact.DedupeKey]; ok {
			sourceDedupeKeys[fact.DedupeKey] = id
		}
	}
	sourceSummary, err := writeSourceIngest(ctx, tx, runDBID, facts, diagnostics, options, sourceDedupeKeys)
	if err != nil {
		if rollbackErr := rollbackSourceWrites(ctx, tx); rollbackErr != nil {
			return summary, errors.Join(err, rollbackErr)
		}
		if completeErr := completeIngestRun(ctx, tx, runDBID, "failed", err.Error(), Summary{}, syncWallNow(options).UnixMilli()); completeErr != nil {
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
	if sourceStatus == "deferred" {
		for _, table := range []string{db.TableSourceRefreshState, db.TableSourceCursorState} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE harness = ? AND source_kind = ? AND source_state_key = ?", source.Harness, source.Kind, source.ID); err != nil {
				return summary, err
			}
		}
	} else {
		if cursorCapable {
			if err := storePiCursor(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata, cursorEligible); err != nil {
				return summary, err
			}
		} else {
			if err := storeSourceReuse(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata, reuseFingerprint); err != nil {
				return summary, err
			}
		}
		if err := upsertSourceRefreshState(ctx, tx, source, options, refreshMetadata, hasRefreshMetadata); err != nil {
			return summary, err
		}

	}
	var first, last interface{}
	for _, fact := range facts {
		if fact.OccurredAtMs != nil {
			if first == nil || *fact.OccurredAtMs < first.(int64) {
				first = *fact.OccurredAtMs
			}
			if last == nil || *fact.OccurredAtMs > last.(int64) {
				last = *fact.OccurredAtMs
			}
		}
	}
	if sourceStatus == "deferred" {
		first, last = nil, nil
	}
	if err := updateSourceStatus(ctx, tx, options, source, sourceStatus, "", first, last); err != nil {
		return summary, err
	}
	if err := completeIngestRun(ctx, tx, runDBID, "completed", "", sourceSummary, syncWallNow(options).UnixMilli()); err != nil {
		return summary, err
	}
	if err := tx.Commit(); err != nil {
		return summary, err
	}
	committed = true
	for key, id := range sourceDedupeKeys {
		seenDedupeKeys[key] = id
	}
	return sourceSummary, nil
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

func writeSourceIngest(ctx context.Context, runner sqlRunner, runDBID int64, facts []RawTokenFact, diagnostics []Diagnostic, options SyncOptions, seenDedupeKeys map[string]int64) (Summary, error) {
	sourceSummary := Summary{}
	for _, fact := range facts {
		if primaryID, seen := seenDedupeKeys[fact.DedupeKey]; fact.DedupeKey != "" && seen {
			changed, conflict, err := mergeRawFactLocation(ctx, runner, primaryID, fact.Location, fact.locationConflicts)
			if err != nil {
				return sourceSummary, err
			}
			if changed {
				if err := enqueueNormalizationWork(ctx, runner, primaryID, db.DomainTokenUsage, syncNowMs(options.Now)); err != nil {
					return sourceSummary, err
				}
			}
			if conflict {
				created, err := insertDiagnostic(ctx, runner, Diagnostic{Harness: fact.Harness, RawFactKey: fact.DedupeKey, Severity: "warning", Code: "location_conflict", Message: "conflicting location evidence for duplicate token fact; affected grouping is unknown"}, &primaryID, nil, syncNowMs(options.Now))
				if err != nil {
					return sourceSummary, err
				}
				if created {
					sourceSummary.Diagnostics++
				}
			}
			created, err := insertDiagnostic(ctx, runner, duplicateSuppressedDiagnostic(fact.Harness), nil, &runDBID, syncNowMs(options.Now))
			if err != nil {
				return sourceSummary, err
			}
			if created {
				sourceSummary.Diagnostics++
			}
			continue
		}
		rawID, inserted, locationChanged, locationConflict, err := upsertRawTokenFact(ctx, runner, fact)
		if err != nil {
			return sourceSummary, err
		}
		if fact.DedupeKey != "" {
			seenDedupeKeys[fact.DedupeKey] = rawID
		}
		if inserted {
			sourceSummary.RawFacts++
		}
		if inserted || locationChanged {
			if err := enqueueNormalizationWork(ctx, runner, rawID, db.DomainTokenUsage, syncNowMs(options.Now)); err != nil {
				return sourceSummary, err
			}
		}
		if locationConflict {
			diagnostic := Diagnostic{Harness: fact.Harness, RawFactKey: rawFactKey(fact), Severity: "warning", Code: "location_conflict", Message: "conflicting location evidence for duplicate token fact; affected grouping is unknown"}
			created, err := insertDiagnostic(ctx, runner, diagnostic, &rawID, nil, syncNowMs(options.Now))
			if err != nil {
				return sourceSummary, err
			}
			if created {
				sourceSummary.Diagnostics++
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
			run_id, hostname, harness, collector, parser, source_id, source_kind, status, started_at_ms
		) VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?, 'running', ?)
	`, runID, options.hostname, source.Harness, options.Collector, options.Parser, source.ID, source.Kind, syncWallNow(options).UnixMilli())
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

func upsertRawTokenFact(ctx context.Context, runner sqlRunner, fact RawTokenFact) (int64, bool, bool, bool, error) {
	key := rawFactKey(fact)
	var id int64
	err := runner.QueryRowContext(ctx, "SELECT id FROM raw_token_usage WHERE raw_fact_key = ?", key).Scan(&id)
	if err == nil {
		changed, conflict, err := mergeRawFactLocation(ctx, runner, id, fact.Location, fact.locationConflicts)
		if err != nil {
			return 0, false, false, false, err
		}
		return id, false, changed, conflict, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, false, false, err
	}
	locationID, err := upsertLocation(ctx, runner, fact.Location)
	if err != nil {
		return 0, false, false, false, err
	}
	result, err := runner.ExecContext(ctx, `
		INSERT INTO raw_token_usage (
			raw_fact_key, harness, source_id, source_kind, collector, parser, observed_at_ms, occurred_at_ms,
			session_id, message_id, provider, model, usage_scope, quality,
			input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens, metadata_json,
			location_id, location_conflicts
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, key, fact.Harness, fact.SourceID, fact.SourceKind, fact.Collector, fact.Parser, fact.ObservedAtMs, nullableInt(fact.OccurredAtMs),
		nullableString(fact.SessionID), nullableString(fact.MessageID), nullableString(fact.Provider), nullableString(fact.Model), fact.UsageScope, fact.Quality,
		nullableInt(fact.InputTokens), nullableInt(fact.OutputTokens), nullableInt(fact.ReasoningTokens), nullableInt(fact.CacheReadTokens), nullableInt(fact.CacheWriteTokens), nullableInt(fact.TotalTokens), nullableString(fact.MetadataJSON), nullableInt64Ptr(locationID), locationValue(fact.locationConflicts))
	if err != nil {
		return 0, false, false, false, err
	}
	id, err = result.LastInsertId()
	if err != nil {
		return 0, false, false, false, err
	}
	return id, true, false, fact.locationConflicts != "", nil
}

func mergeRawFactLocation(ctx context.Context, runner sqlRunner, id int64, incoming *Location, incomingConflicts string) (bool, bool, error) {
	var existingID sql.NullInt64
	var priorConflicts sql.NullString
	if err := runner.QueryRowContext(ctx, "SELECT location_id, location_conflicts FROM raw_token_usage WHERE id = ?", id).Scan(&existingID, &priorConflicts); err != nil {
		return false, false, err
	}
	existing, err := loadLocation(ctx, runner, existingID)
	if err != nil {
		return false, false, err
	}
	joined := strings.Trim(strings.Join([]string{priorConflicts.String, incomingConflicts}, ","), ",")
	merged, conflicts, changed, conflict := mergeLocations(existing, incoming, joined)
	if incomingConflicts != "" && !strings.Contains(priorConflicts.String, incomingConflicts) {
		conflict = true
	}
	if !changed && conflicts == priorConflicts.String {
		return false, conflict, nil
	}
	locationID, err := upsertLocation(ctx, runner, merged)
	if err != nil {
		return false, false, err
	}
	_, err = runner.ExecContext(ctx, "UPDATE raw_token_usage SET location_id = ?, location_conflicts = ? WHERE id = ?", nullableInt64Ptr(locationID), locationValue(conflicts), id)
	return true, conflict, err
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
