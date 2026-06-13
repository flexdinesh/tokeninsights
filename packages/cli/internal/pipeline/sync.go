package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"tokeninsights-cli/internal/db"
)

const (
	defaultCollector = "tokeninsights-sync-go"
	defaultParser    = "sync-first-v1"
)

var runSequence atomic.Uint64

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
		harnessSummary, err := syncHarness(ctx, database, options, harness)
		mergeSummary(&summary, harnessSummary)
		if err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, err)
			continue
		}
		if harnessSummary.RawFacts == 0 && harnessSummary.Observations == 0 {
			summary.Skipped++
			continue
		}
		summary.Synced++
	}

	if options.Normalize && summary.Observations > 0 {
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

func dryRunSync(ctx context.Context, options SyncOptions) (Summary, error) {
	var summary Summary
	summary.RequestedHarnesses = len(options.Harnesses)
	for _, harness := range options.Harnesses {
		adapter, ok := AdapterFor(harness)
		if !ok {
			summary.Failed++
			summary.Errors = append(summary.Errors, fmt.Errorf("unsupported harness %q", harness))
			continue
		}
		sources, err := adapter.Discover(ctx, options.SourceDir)
		if err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, fmt.Errorf("%s discover: %w", harness, err))
			continue
		}
		if len(sources) == 0 {
			summary.Skipped++
			continue
		}
		summary.Synced++
		for _, source := range sources {
			facts, diagnostics, err := adapter.Parse(ctx, source, options)
			if err != nil {
				summary.Failed++
				summary.Errors = append(summary.Errors, fmt.Errorf("%s parse: %w", harness, err))
				continue
			}
			summary.RawFacts += len(facts)
			summary.Diagnostics += len(diagnostics)
		}
	}
	return summary, errors.Join(summary.Errors...)
}

func syncHarness(ctx context.Context, database *sql.DB, options SyncOptions, harness Harness) (Summary, error) {
	var summary Summary
	adapter, ok := AdapterFor(harness)
	if !ok {
		return summary, fmt.Errorf("unsupported harness %q", harness)
	}

	sources, err := adapter.Discover(ctx, options.SourceDir)
	if err != nil {
		return summary, fmt.Errorf("%s discover: %w", harness, err)
	}
	if len(sources) == 0 {
		return summary, nil
	}

	for _, source := range sources {
		runID := newRunID(harness, source.ID, options.Now)
		runDBID, err := createIngestRun(ctx, database, runID, source, options)
		if err != nil {
			return summary, err
		}
		facts, diagnostics, parseErr := adapter.Parse(ctx, source, options)
		if parseErr != nil {
			_ = completeIngestRun(ctx, database, runDBID, "failed", parseErr.Error(), Summary{})
			return summary, fmt.Errorf("%s parse: %w", harness, parseErr)
		}

		sourceSummary := Summary{Diagnostics: len(diagnostics)}
		for _, fact := range facts {
			rawID, inserted, err := upsertRawTokenFact(ctx, database, fact)
			if err != nil {
				_ = completeIngestRun(ctx, database, runDBID, "failed", err.Error(), sourceSummary)
				return summary, err
			}
			if inserted {
				sourceSummary.RawFacts++
			}
			observed, err := insertObservation(ctx, database, runDBID, rawID, fact)
			if err != nil {
				_ = completeIngestRun(ctx, database, runDBID, "failed", err.Error(), sourceSummary)
				return summary, err
			}
			if observed {
				sourceSummary.Observations++
			}
		}

		for _, diagnostic := range diagnostics {
			if err := insertDiagnostic(ctx, database, diagnostic, nil, &runDBID, syncNowMs(options.Now)); err != nil {
				_ = completeIngestRun(ctx, database, runDBID, "failed", err.Error(), sourceSummary)
				return summary, err
			}
		}
		if err := completeIngestRun(ctx, database, runDBID, "completed", "", sourceSummary); err != nil {
			return summary, err
		}
		mergeSummary(&summary, sourceSummary)
	}
	return summary, nil
}

func newRunID(harness Harness, sourceID string, now time.Time) string {
	startedAt := now.UnixNano()
	if now.IsZero() {
		startedAt = time.Now().UnixNano()
	}
	sequence := runSequence.Add(1)
	return stableHash(fmt.Sprintf("%s:%s:%d:%d", harness, sourceID, startedAt, sequence))
}

func createIngestRun(ctx context.Context, database *sql.DB, runID string, source Source, options SyncOptions) (int64, error) {
	result, err := database.ExecContext(ctx, `
		INSERT INTO ingest_runs (
			run_id, harness, collector, parser, source_id, source_kind, status, started_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, 'running', ?)
	`, runID, source.Harness, options.Collector, options.Parser, source.ID, source.Kind, syncNowMs(options.Now))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func completeIngestRun(ctx context.Context, database *sql.DB, runID int64, status string, message string, summary Summary) error {
	_, err := database.ExecContext(ctx, `
		UPDATE ingest_runs
		SET status = ?,
			completed_at_ms = CASE WHEN completed_at_ms IS NULL THEN strftime('%s','now') * 1000 ELSE completed_at_ms END,
			error_message = NULLIF(?, ''),
			raw_fact_count = ?,
			observation_count = ?,
			canonical_count = ?,
			diagnostic_count = ?
		WHERE id = ? AND status = 'running'
	`, status, message, summary.RawFacts, summary.Observations, summary.Canonical, summary.Diagnostics, runID)
	return err
}

func upsertRawTokenFact(ctx context.Context, database *sql.DB, fact RawTokenFact) (int64, bool, error) {
	key := rawFactKey(fact)
	result, err := database.ExecContext(ctx, `
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
	if err := database.QueryRowContext(ctx, "SELECT id FROM raw_token_usage WHERE raw_fact_key = ?", key).Scan(&id); err != nil {
		return 0, false, err
	}
	return id, inserted > 0, nil
}

func insertObservation(ctx context.Context, database *sql.DB, runID int64, rawID int64, fact RawTokenFact) (bool, error) {
	key := stableHash(fmt.Sprintf("%d:%s", runID, rawFactKey(fact)))
	result, err := database.ExecContext(ctx, `
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
