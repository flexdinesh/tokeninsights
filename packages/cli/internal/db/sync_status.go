package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type SyncStatus struct {
	JobID              int64
	Running            bool
	Phase              string
	Error              string
	Revision           int64
	StartedAtMs        int64
	UpdatedAtMs        int64
	LastSuccessfulAtMs int64
	Harnesses          map[string]string
	TotalSources       int64
	CheckedSources     int64
	ReadySources       int64
	FailedSources      int64
	DiscoveryComplete  bool
	AllHarnesses       bool
	Normalize          bool
}

func emptySyncStatus() SyncStatus { return SyncStatus{Phase: "ready", Harnesses: map[string]string{}} }

// ReadSyncStatus permits compatible rebuild-pending metadata, never analytics.
func ReadSyncStatus(ctx context.Context, path string) (SyncStatus, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return emptySyncStatus(), nil
	}
	held, release, err := observeWriter(path)
	if err != nil {
		return SyncStatus{}, err
	}
	defer release()
	absolute, err := filepath.Abs(path)
	if err != nil {
		return SyncStatus{}, err
	}
	database, err := openSQLiteMode(ctx, absolute, "ro")
	if err != nil {
		return SyncStatus{}, err
	}
	defer func() { _ = database.Close() }()
	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncStatus{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := inspectCompatibility(ctx, tx)
	if err != nil {
		return SyncStatus{}, err
	}
	if err := requireCompatible(state, false); err != nil {
		return SyncStatus{}, err
	}
	status, err := LoadSyncStatus(ctx, tx)
	if err != nil {
		return SyncStatus{}, err
	}
	if status.Running && !held {
		status.Running, status.Phase, status.Error = false, "interrupted", "Sync interrupted. Retry to check remaining sources."
	}
	if state.RebuildPending {
		status.Phase = "rebuilding"
		if !status.Running {
			status.Phase = "rebuild_failed"
			status.Error = "Usage recovery is incomplete. Retry sync with the original source configuration."
		}
	}
	return status, tx.Commit()
}

func LoadSyncStatus(ctx context.Context, reader Reader) (SyncStatus, error) {
	s := emptySyncStatus()
	if err := reader.QueryRowContext(ctx, "SELECT revision, last_successful_sync_at_ms FROM sync_state WHERE id = 1").Scan(&s.Revision, &s.LastSuccessfulAtMs); err != nil {
		return s, err
	}
	var outcome string
	err := reader.QueryRowContext(ctx, "SELECT id, status, phase, started_at_ms, updated_at_ms, all_harnesses, normalize FROM sync_jobs ORDER BY id DESC LIMIT 1").Scan(&s.JobID, &outcome, &s.Phase, &s.StartedAtMs, &s.UpdatedAtMs, &s.AllHarnesses, &s.Normalize)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.Running = outcome == "running"
	if outcome == "failed" {
		s.Error = "Sync incomplete. Some sources failed; saved usage remains available."
	}
	if outcome == "cancelled" || outcome == "interrupted" {
		s.Error = "Sync interrupted. Retry to check remaining sources."
	}
	rows, err := reader.QueryContext(ctx, "SELECT harness, status, discovered, total_sources, checked_sources, failed_sources FROM sync_harnesses WHERE job_id = ?", s.JobID)
	if err != nil {
		return s, err
	}
	defer func() { _ = rows.Close() }()
	s.DiscoveryComplete = true
	for rows.Next() {
		var h, status string
		var discovered bool
		var total, checked, failed int64
		if err := rows.Scan(&h, &status, &discovered, &total, &checked, &failed); err != nil {
			return s, err
		}
		s.Harnesses[h] = status
		s.TotalSources += total
		s.CheckedSources += checked
		s.FailedSources += failed
		s.DiscoveryComplete = s.DiscoveryComplete && discovered
	}
	if err := rows.Err(); err != nil {
		return s, err
	}
	if err := reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_sources WHERE job_id = ? AND status IN ('ready', 'unchanged')", s.JobID).Scan(&s.ReadySources); err != nil {
		return s, err
	}
	return s, nil
}

// AdvanceAnalyticsRevision must share the canonical write transaction.
func AdvanceAnalyticsRevision(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "UPDATE sync_state SET revision = revision + 1 WHERE id = 1")
	return err
}

type DayCoverage struct {
	Total          *int64 `json:"total"`
	Day            string `json:"day"`
	Status         string `json:"status"`
	CheckedAtMs    int64  `json:"checkedAt"`
	PendingSources int64  `json:"pendingSources"`
	FailedSources  int64  `json:"failedSources"`
	HasUsage       bool   `json:"hasUsage"`
}

const MaxCoverageDays = 366

// Coverage describes retained sources, never provider-account completeness.
// Unknown source dates conservatively affect every day in the bounded range.
func ViewerDayCoverage(ctx context.Context, reader Reader, f Filter, now time.Time) ([]DayCoverage, error) {
	local := now.Local()
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	start, end := f.Start, f.End
	if start.IsZero() {
		start = today.AddDate(0, 0, -6)
	}
	if end.IsZero() || end.After(today.AddDate(0, 0, 1)) {
		end = today.AddDate(0, 0, 1)
	}
	if f.DayFrom != "" {
		var err error
		start, err = time.ParseInLocation(time.DateOnly, f.DayFrom, local.Location())
		if err != nil {
			return nil, err
		}
	}
	if f.DayTo != "" {
		bound, err := time.ParseInLocation(time.DateOnly, f.DayTo, local.Location())
		if err != nil {
			return nil, err
		}
		end = bound.AddDate(0, 0, 1)
		if end.After(today.AddDate(0, 0, 1)) {
			end = today.AddDate(0, 0, 1)
		}
	}
	if start.Before(end.AddDate(0, 0, -MaxCoverageDays)) {
		start = end.AddDate(0, 0, -MaxCoverageDays)
	}
	result := []DayCoverage{}
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		result = append(result, DayCoverage{Day: day.Format(time.DateOnly), Status: "checked"})
	}
	if len(result) == 0 {
		return result, nil
	}
	harnesses := f.Harnesses
	if len(harnesses) == 0 {
		harnesses = []string{"opencode", "pi", "codex", "claude-code"}
	}
	for _, h := range harnesses {
		var jobID, checkedAt int64
		var status string
		var discovered, normalized bool
		err := reader.QueryRowContext(ctx, `SELECT h.job_id, h.status, h.discovered, h.checked_at_ms, j.normalize FROM sync_harnesses h JOIN sync_jobs j ON j.id = h.job_id WHERE h.harness = ? ORDER BY h.job_id DESC LIMIT 1`, h).Scan(&jobID, &status, &discovered, &checkedAt, &normalized)
		if errors.Is(err, sql.ErrNoRows) {
			for i := range result {
				result[i].Status = "unverified"
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if !discovered || !normalized {
			for i := range result {
				result[i].Status = "unverified"
				if status == "failed" {
					result[i].FailedSources++
				}
			}
			continue
		}
		for i := range result {
			if checkedAt > 0 && (result[i].CheckedAtMs == 0 || checkedAt < result[i].CheckedAtMs) {
				result[i].CheckedAtMs = checkedAt
			}
		}
		rows, err := reader.QueryContext(ctx, "SELECT status, min_occurred_at_ms, max_occurred_at_ms FROM sync_sources WHERE job_id = ? AND harness = ? AND status NOT IN ('ready', 'unchanged')", jobID, h)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var sourceStatus string
			var first, last sql.NullInt64
			if err := rows.Scan(&sourceStatus, &first, &last); err != nil {
				_ = rows.Close()
				return nil, err
			}
			for i := range result {
				day, err := time.ParseInLocation(time.DateOnly, result[i].Day, local.Location())
				if err != nil {
					_ = rows.Close()
					return nil, err
				}
				if first.Valid && last.Valid && (last.Int64 < day.UnixMilli() || first.Int64 >= day.AddDate(0, 0, 1).UnixMilli()) {
					continue
				}
				if sourceStatus == "failed" {
					result[i].FailedSources++
				} else {
					result[i].PendingSources++
				}
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
		if status == "failed" {
			for i := range result {
				if result[i].FailedSources == 0 {
					result[i].FailedSources++
				}
			}
		}
		var pendingWork bool
		if err := reader.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM normalization_work_queue q JOIN raw_token_usage r ON r.id = q.raw_fact_id WHERE r.harness = ?)", h).Scan(&pendingWork); err != nil {
			return nil, err
		}
		if status == "normalizing" || pendingWork {
			for i := range result {
				if result[i].PendingSources == 0 {
					result[i].PendingSources++
				}
			}
		}
	}
	buckets, err := ViewerTokenBuckets(ctx, reader, f, BucketDay)
	if err != nil {
		return nil, err
	}
	hasUsage := map[string]bool{}
	totals := map[string]int64{}
	for _, bucket := range buckets {
		hasUsage[bucket.Bucket] = true
		totals[bucket.Bucket] = bucket.TotalTokens
	}
	for i := range result {
		d := &result[i]
		d.HasUsage = hasUsage[d.Day]
		if d.HasUsage {
			total := totals[d.Day]
			d.Total = &total
		}
		if d.PendingSources > 0 || d.FailedSources > 0 {
			d.Status = "pending"
			if d.HasUsage || d.FailedSources > 0 {
				d.Status = "partial"
			}
		} else if d.Status == "checked" && !d.HasUsage {
			d.Status = "empty"
			total := int64(0)
			d.Total = &total
		}
		if d.Status != "checked" && d.Status != "empty" {
			d.CheckedAtMs = 0
		}
	}
	return result, nil
}
