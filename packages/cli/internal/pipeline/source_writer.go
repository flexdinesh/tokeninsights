package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

const sourceWriteBatchSize = 64

// A rolled-back source may commit its failure audit, then stop the job.
type auditedWriteFailure struct{ err error }

func (e auditedWriteFailure) Error() string { return e.err.Error() }
func (e auditedWriteFailure) Unwrap() error { return e.err }

// validatePreparedSource runs before the write transaction. A source may have
// changed while its prepared result waited for earlier sources to commit.
func validatePreparedSource(prepared preparedSource) preparedSource {
	if prepared.parseErr != nil || prepared.status == "deferred" {
		return prepared
	}
	changed := false
	if prepared.hasMetadata && prepared.source.Harness != HarnessOpenCode {
		current, ok := sourceRefreshMetadataFor(prepared.source)
		changed = !ok || current != prepared.metadata
		if prepared.sourceInfo != nil {
			info, err := os.Stat(prepared.source.Path)
			changed = changed || err != nil || !os.SameFile(prepared.sourceInfo, info)
		}
	}
	for _, dependency := range prepared.dependencies {
		current, ok := sourceRefreshMetadataFor(dependency.source)
		if !ok || current != dependency.metadata {
			changed = true
			break
		}
		if dependency.sourceInfo != nil {
			info, err := os.Stat(dependency.source.Path)
			if err != nil || !os.SameFile(dependency.sourceInfo, info) {
				changed = true
				break
			}
		}
	}
	if changed {
		prepared.status, prepared.unchanged, prepared.cursor = "deferred", false, nil
		prepared.diagnostics = append(prepared.diagnostics, Diagnostic{Harness: prepared.source.Harness, Severity: "info", Code: "jsonl_source_changed", Message: "source changed during snapshot read; new records await next sync"})
	}
	return prepared
}

func commitPreparedSources(ctx context.Context, database *sql.DB, options SyncOptions, sources []preparedSource, seen map[string]int64) (Summary, error) {
	var summary Summary
	validated := make([]preparedSource, len(sources))
	for i, prepared := range sources {
		validated[i] = validatePreparedSource(prepared)
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return summary, err
	}
	defer func() { _ = tx.Rollback() }()
	committedKeys := map[string]int64{}
	var sourceErrors []error
	var fatal error
	for _, prepared := range validated {
		keys := make(map[string]int64, len(prepared.facts))
		for _, fact := range prepared.facts {
			if id, ok := seen[fact.DedupeKey]; ok {
				keys[fact.DedupeKey] = id
			}
			if id, ok := committedKeys[fact.DedupeKey]; ok {
				keys[fact.DedupeKey] = id
			}
		}
		part, ingestErr := writePreparedSource(ctx, tx, options, prepared, keys)
		if ingestErr != nil {
			var recoverable sourceFailure
			if !errors.As(ingestErr, &recoverable) {
				var audited auditedWriteFailure
				if !errors.As(ingestErr, &audited) {
					return Summary{}, ingestErr
				}
				fatal = ingestErr
				break
			}
			sourceErrors = append(sourceErrors, ingestErr)
		} else {
			for key, id := range keys {
				committedKeys[key] = id
			}
		}
		mergeSummary(&summary, part)
	}
	if err := commitSyncTransaction(ctx, tx); err != nil {
		return Summary{}, err
	}
	for key, id := range committedKeys {
		seen[key] = id
	}
	if fatal != nil {
		return summary, fatal
	}
	if len(sourceErrors) > 0 {
		return summary, sourceFailure{err: errors.Join(sourceErrors...)}
	}
	return summary, nil
}

func writePreparedSource(ctx context.Context, tx *sql.Tx, options SyncOptions, prepared preparedSource, keys map[string]int64) (Summary, error) {
	source := prepared.source
	runID := newRunID(source.Harness, source.ID, options.Now)
	runDBID, err := createIngestRunAt(ctx, tx, runID, source, options, prepared.startedAtMs)
	if err != nil {
		return Summary{}, err
	}
	if prepared.parseErr != nil {
		if err := completeIngestRun(ctx, tx, runDBID, "failed", prepared.parseErr.Error(), Summary{}, syncWallNow(options).UnixMilli()); err != nil {
			return Summary{}, err
		}
		if err := completePreparedAttempt(ctx, tx, options, source, true); err != nil {
			return Summary{}, err
		}
		return Summary{}, sourceFailure{err: fmt.Errorf("%s parse: %w", source.Harness, prepared.parseErr)}
	}
	if err := createSourceWriteSavepoint(ctx, tx); err != nil {
		return Summary{}, err
	}
	fail := func(cause error) (Summary, error) {
		if rollbackErr := rollbackSourceWrites(ctx, tx); rollbackErr != nil {
			return Summary{}, errors.Join(cause, rollbackErr)
		}
		if finishErr := completeIngestRun(ctx, tx, runDBID, "failed", cause.Error(), Summary{}, syncWallNow(options).UnixMilli()); finishErr != nil {
			return Summary{}, errors.Join(cause, finishErr)
		}
		if finishErr := completePreparedAttempt(ctx, tx, options, source, true); finishErr != nil {
			return Summary{}, errors.Join(cause, finishErr)
		}
		return Summary{}, auditedWriteFailure{err: cause}
	}
	summary, err := writeSourceIngest(ctx, tx, runDBID, prepared.facts, prepared.diagnostics, options, keys)
	if err != nil {
		return fail(err)
	}
	if err := writePreparedMarkers(ctx, tx, options, prepared); err != nil {
		return fail(err)
	}
	first, last := prepared.minOccurredAtMs, prepared.maxOccurredAtMs
	if prepared.status == "deferred" {
		first, last = nil, nil
	}
	if err := updateSourceStatus(ctx, tx, options, source, prepared.status, "", nullableInt(first), nullableInt(last)); err != nil {
		return fail(err)
	}
	if err := completeIngestRun(ctx, tx, runDBID, "completed", "", summary, syncWallNow(options).UnixMilli()); err != nil {
		return fail(err)
	}
	if err := completePreparedAttempt(ctx, tx, options, source, false); err != nil {
		return fail(err)
	}
	if err := releaseSourceWriteSavepoint(ctx, tx); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func writePreparedMarkers(ctx context.Context, runner sqlRunner, options SyncOptions, prepared preparedSource) error {
	source := prepared.source
	if prepared.status == "deferred" {
		for _, table := range []string{db.TableSourceRefreshState, db.TableSourceCursorState} {
			if _, err := runner.ExecContext(ctx, "DELETE FROM "+table+" WHERE harness = ? AND source_kind = ? AND source_state_key = ?", source.Harness, source.Kind, source.ID); err != nil {
				return err
			}
		}
		return nil
	}
	if prepared.hasMetadata {
		if prepared.cursor != nil {
			state := prepared.cursor
			if _, err := runner.ExecContext(ctx, `INSERT INTO source_cursor_state (
				harness, source_kind, source_state_key, collector, parser, cursor_kind, byte_offset,
				source_mtime_ms, source_size_bytes, prefix_hash, boundary_hash, location_fingerprint, updated_at_ms
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (harness, source_kind, source_state_key) DO UPDATE SET
				collector = excluded.collector, parser = excluded.parser, cursor_kind = excluded.cursor_kind,
				byte_offset = excluded.byte_offset, source_mtime_ms = excluded.source_mtime_ms,
				source_size_bytes = excluded.source_size_bytes, prefix_hash = excluded.prefix_hash,
				boundary_hash = excluded.boundary_hash, location_fingerprint = excluded.location_fingerprint,
				updated_at_ms = excluded.updated_at_ms`, source.Harness, source.Kind, source.ID, state.collector,
				state.parser, state.kind, state.offset, state.mtimeMs, state.sizeBytes, state.prefixHash,
				state.boundaryHash, state.locationFingerprint, syncNowMs(options.Now)); err != nil {
				return err
			}
		} else if !prepared.unchanged {
			if _, err := runner.ExecContext(ctx, "DELETE FROM source_cursor_state WHERE harness = ? AND source_kind = ? AND source_state_key = ?", source.Harness, source.Kind, source.ID); err != nil {
				return err
			}
		}
	}
	return upsertSourceRefreshState(ctx, runner, source, options, prepared.metadata, prepared.hasMetadata)
}

func completePreparedAttempt(ctx context.Context, runner sqlRunner, options SyncOptions, source Source, failed bool) error {
	if options.jobID == 0 {
		return nil
	}
	if failed {
		if err := updateSourceStatus(ctx, runner, options, source, "failed", "read_or_parse_failed", nil, nil); err != nil {
			return err
		}
	}
	if _, err := runner.ExecContext(ctx, "UPDATE sync_harnesses SET checked_sources = checked_sources + 1, failed_sources = failed_sources + ? WHERE job_id = ? AND harness = ?", failed, options.jobID, source.Harness); err != nil {
		return err
	}
	_, err := runner.ExecContext(ctx, "UPDATE sync_jobs SET updated_at_ms = ? WHERE id = ?", syncWallNow(options).UnixMilli(), options.jobID)
	return err
}
