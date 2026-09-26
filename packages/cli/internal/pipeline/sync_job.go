package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func startSyncJob(ctx context.Context, database *sql.DB, options SyncOptions) (int64, error) {
	key, err := recoverySourceKey(options)
	if err != nil {
		return 0, err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	now := syncWallNow(options).UnixMilli()
	// Only the writer-lock owner can abandon prior running jobs.
	if _, err := tx.ExecContext(ctx, "UPDATE sync_jobs SET status = 'interrupted', phase = 'interrupted', completed_at_ms = MAX(started_at_ms, ?), updated_at_ms = ? WHERE status = 'running'", now, now); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO sync_jobs (scope_key, status, phase, started_at_ms, updated_at_ms, normalize, all_harnesses) VALUES (?, 'running', 'discovering', ?, ?, ?, ?)", key, now, now, options.Normalize, selectsAllHarnesses(options.Harnesses))
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, h := range options.Harnesses {
		if _, err := tx.ExecContext(ctx, "INSERT INTO sync_harnesses (job_id, harness, status) VALUES (?, ?, 'pending')", id, h); err != nil {
			return 0, err
		}
	}
	return id, commitSyncTransaction(ctx, tx)
}

func finishSyncJob(database *sql.DB, options SyncOptions, runErr error) error {
	ctx, cancel := context.WithTimeout(withSyncStats(context.Background(), options.stats), 5*time.Second)
	defer cancel()
	outcome, phase, code := "completed", "ready", ""
	if runErr != nil {
		outcome, phase, code = "failed", "failed", "source_failure"
	}
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		outcome, phase, code = "cancelled", "cancelled", "cancelled"
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := syncWallNow(options).UnixMilli()
	if _, err := tx.ExecContext(ctx, "UPDATE sync_jobs SET status = ?, phase = ?, error_code = ?, completed_at_ms = MAX(started_at_ms, ?), updated_at_ms = ? WHERE id = ?", outcome, phase, code, now, now, options.jobID); err != nil {
		return err
	}
	var deferred bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sync_sources WHERE job_id = ? AND status = 'deferred')", options.jobID).Scan(&deferred); err != nil {
		return err
	}
	if runErr == nil && !deferred && options.Normalize && selectsAllHarnesses(options.Harnesses) {
		if _, err := tx.ExecContext(ctx, "UPDATE sync_state SET last_successful_sync_at_ms = ? WHERE id = 1", now); err != nil {
			return err
		}
	}
	if err := db.AdvanceAnalyticsRevision(ctx, tx); err != nil {
		return err
	}
	return commitSyncTransaction(ctx, tx)
}

func setHarnessStatus(ctx context.Context, database *sql.DB, options SyncOptions, h Harness, status SyncProgressStatus) error {
	now := syncWallNow(options).UnixMilli()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "UPDATE sync_harnesses SET status = ?, checked_at_ms = CASE WHEN ? IN ('synced', 'skipped') AND ? THEN ? ELSE 0 END WHERE job_id = ? AND harness = ?", status, status, options.Normalize, now, options.jobID, h); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sync_jobs SET phase = ?, updated_at_ms = ? WHERE id = ?", status, now, options.jobID); err != nil {
		return err
	}
	if status == SyncProgressSynced || status == SyncProgressSkipped || status == SyncProgressFailed {
		if err := db.AdvanceAnalyticsRevision(ctx, tx); err != nil {
			return err
		}
	}
	return commitSyncTransaction(ctx, tx)
}

func recordDiscoveredSources(ctx context.Context, database *sql.DB, options SyncOptions, h Harness, sources []Source) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "UPDATE sync_harnesses SET discovered = 1, total_sources = ? WHERE job_id = ? AND harness = ?", len(sources), options.jobID, h); err != nil {
		return err
	}
	for _, source := range sources {
		if _, err := tx.ExecContext(ctx, "INSERT INTO sync_sources (job_id, harness, source_id, source_kind, status, updated_at_ms) VALUES (?, ?, ?, ?, 'pending', ?)", options.jobID, h, source.ID, source.Kind, syncWallNow(options).UnixMilli()); err != nil {
			return err
		}
	}
	return commitSyncTransaction(ctx, tx)
}

func updateSourceStatus(ctx context.Context, runner sqlRunner, options SyncOptions, source Source, status, code string, first, last interface{}) error {
	if options.jobID == 0 {
		return nil
	}
	_, err := runner.ExecContext(ctx, "UPDATE sync_sources SET status = ?, error_code = ?, min_occurred_at_ms = ?, max_occurred_at_ms = ?, updated_at_ms = ? WHERE job_id = ? AND harness = ? AND source_kind = ? AND source_id = ?", status, code, first, last, syncWallNow(options).UnixMilli(), options.jobID, source.Harness, source.Kind, source.ID)
	return err
}

type sourceFailure struct{ err error }

func (e sourceFailure) Error() string { return e.err.Error() }
func (e sourceFailure) Unwrap() error { return e.err }

func publishHarnessSources(ctx context.Context, database *sql.DB, options SyncOptions, h Harness) error {
	if !options.Normalize {
		return nil
	}
	_, err := database.ExecContext(ctx, "UPDATE sync_sources SET status = 'ready', updated_at_ms = ? WHERE job_id = ? AND harness = ? AND status = 'ingested'", syncWallNow(options).UnixMilli(), options.jobID, h)
	if err != nil {
		return fmt.Errorf("publish source coverage: %w", err)
	}
	if stats := statsForSync(ctx); stats != nil {
		stats.writerCommits.Add(1)
	}
	return nil
}

func syncWallNow(options SyncOptions) time.Time {
	if options.Clock != nil {
		return options.Clock()
	}
	return time.Now()
}
