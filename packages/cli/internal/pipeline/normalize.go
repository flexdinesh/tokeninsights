package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"tokeninsights-cli/internal/db"
)

type rawTokenRow struct {
	ID               int64
	RawFactKey       string
	Harness          Harness
	SourceID         string
	ObservedAtMs     int64
	OccurredAtMs     sql.NullInt64
	SessionID        sql.NullString
	MessageID        sql.NullString
	Provider         sql.NullString
	Model            sql.NullString
	UsageScope       string
	Quality          string
	InputTokens      sql.NullInt64
	OutputTokens     sql.NullInt64
	ReasoningTokens  sql.NullInt64
	CacheReadTokens  sql.NullInt64
	CacheWriteTokens sql.NullInt64
	TotalTokens      sql.NullInt64
	LastRunID        sql.NullInt64
}

func Normalize(ctx context.Context, options NormalizeOptions) (Summary, error) {
	summary := Summary{}
	if options.DryRun {
		database, err := db.Open(options.DBPath)
		if err != nil {
			return summary, err
		}
		defer database.Close()
		rows, err := loadRawTokenRows(ctx, database, options.Harnesses)
		if err != nil {
			return summary, err
		}
		for _, row := range rows {
			if !row.SessionID.Valid || strings.TrimSpace(row.SessionID.String) == "" {
				summary.Diagnostics++
				continue
			}
			summary.Canonical++
		}
		return summary, nil
	}

	database, _, err := db.CreateIfMissing(options.DBPath)
	if err != nil {
		return summary, err
	}
	defer database.Close()

	rows, err := loadRawTokenRows(ctx, database, options.Harnesses)
	if err != nil {
		return summary, err
	}
	for _, row := range rows {
		count, diagnostic, err := normalizeRawTokenRow(ctx, database, row, options)
		if err != nil {
			return summary, err
		}
		summary.Canonical += count
		summary.Diagnostics += diagnostic
	}
	return summary, nil
}

func loadRawTokenRows(ctx context.Context, database *sql.DB, harnesses []Harness) ([]rawTokenRow, error) {
	var args []interface{}
	where := ""
	if len(harnesses) > 0 {
		parts := make([]string, len(harnesses))
		for i, harness := range harnesses {
			parts[i] = "?"
			args = append(args, harness)
		}
		where = "WHERE r.harness IN (" + strings.Join(parts, ",") + ")"
	}

	query := `
		SELECT
			r.id, r.raw_fact_key, r.harness, r.source_id, r.observed_at_ms, r.occurred_at_ms,
			r.session_id, r.message_id, r.provider, r.model, r.usage_scope, r.quality,
			r.input_tokens, r.output_tokens, r.reasoning_tokens, r.cache_read_tokens, r.cache_write_tokens, r.total_tokens,
			(
				SELECT ro.ingest_run_id
				FROM raw_observations ro
				WHERE ro.raw_fact_id = r.id
				ORDER BY ro.id DESC
				LIMIT 1
			) AS ingest_run_id
		FROM raw_token_usage r
		` + where + `
		ORDER BY r.id
	`
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []rawTokenRow
	for rows.Next() {
		var row rawTokenRow
		var harness string
		if err := rows.Scan(
			&row.ID,
			&row.RawFactKey,
			&harness,
			&row.SourceID,
			&row.ObservedAtMs,
			&row.OccurredAtMs,
			&row.SessionID,
			&row.MessageID,
			&row.Provider,
			&row.Model,
			&row.UsageScope,
			&row.Quality,
			&row.InputTokens,
			&row.OutputTokens,
			&row.ReasoningTokens,
			&row.CacheReadTokens,
			&row.CacheWriteTokens,
			&row.TotalTokens,
			&row.LastRunID,
		); err != nil {
			return nil, err
		}
		row.Harness = Harness(harness)
		result = append(result, row)
	}
	return result, rows.Err()
}

func normalizeRawTokenRow(ctx context.Context, database *sql.DB, row rawTokenRow, options NormalizeOptions) (int, int, error) {
	if !row.SessionID.Valid || strings.TrimSpace(row.SessionID.String) == "" {
		diagnostic := Diagnostic{
			Harness:    row.Harness,
			RawFactKey: row.RawFactKey,
			Severity:   "warning",
			Code:       "missing_session",
			Message:    "raw token fact skipped because no stable session identity is available",
		}
		if err := insertDiagnostic(ctx, database, diagnostic, &row.ID, rawRunPointer(row), nowMs(options)); err != nil {
			return 0, 0, err
		}
		return 0, 1, nil
	}

	sessionDBID, err := upsertCanonicalSession(ctx, database, row)
	if err != nil {
		return 0, 0, err
	}
	messageDBID, err := upsertCanonicalMessage(ctx, database, row, sessionDBID)
	if err != nil {
		return 0, 0, err
	}
	inserted, err := upsertCanonicalTokenUsage(ctx, database, row, sessionDBID, messageDBID)
	if err != nil {
		return 0, 0, err
	}
	if inserted {
		return 1, 0, nil
	}
	return 0, 0, nil
}

func upsertCanonicalSession(ctx context.Context, database *sql.DB, row rawTokenRow) (int64, error) {
	key := stableHash(fmt.Sprintf("session:%s:%s", row.Harness, row.SessionID.String))
	timestamp := canonicalTime(row)
	_, err := database.ExecContext(ctx, `
		INSERT INTO canonical_sessions (
			semantic_key, harness, session_id, first_seen_at_ms, last_seen_at_ms, primary_raw_fact_id
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(semantic_key) DO UPDATE SET
			first_seen_at_ms = MIN(first_seen_at_ms, excluded.first_seen_at_ms),
			last_seen_at_ms = MAX(last_seen_at_ms, excluded.last_seen_at_ms),
			primary_raw_fact_id = COALESCE(primary_raw_fact_id, excluded.primary_raw_fact_id)
	`, key, row.Harness, row.SessionID.String, timestamp, timestamp, row.ID)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := database.QueryRowContext(ctx, "SELECT id FROM canonical_sessions WHERE semantic_key = ?", key).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func upsertCanonicalMessage(ctx context.Context, database *sql.DB, row rawTokenRow, sessionDBID int64) (*int64, error) {
	if !row.MessageID.Valid || strings.TrimSpace(row.MessageID.String) == "" {
		return nil, nil
	}
	key := stableHash(fmt.Sprintf("message:%s:%s:%s", row.Harness, row.SessionID.String, row.MessageID.String))
	_, err := database.ExecContext(ctx, `
		INSERT INTO canonical_messages (
			semantic_key, session_id, harness, harness_message_id, occurred_at_ms, primary_raw_fact_id
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(semantic_key) DO UPDATE SET
			occurred_at_ms = COALESCE(occurred_at_ms, excluded.occurred_at_ms),
			primary_raw_fact_id = COALESCE(primary_raw_fact_id, excluded.primary_raw_fact_id)
	`, key, sessionDBID, row.Harness, row.MessageID.String, nullableNullInt(row.OccurredAtMs), row.ID)
	if err != nil {
		return nil, err
	}
	var id int64
	if err := database.QueryRowContext(ctx, "SELECT id FROM canonical_messages WHERE semantic_key = ?", key).Scan(&id); err != nil {
		return nil, err
	}
	return &id, nil
}

func upsertCanonicalTokenUsage(ctx context.Context, database *sql.DB, row rawTokenRow, sessionDBID int64, messageDBID *int64) (bool, error) {
	key := canonicalTokenKey(row)
	result, err := database.ExecContext(ctx, `
		INSERT INTO canonical_token_usage (
			semantic_key, recorded_at_ms, harness, session_id, message_id, provider, model, usage_scope, quality,
			is_countable, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens,
			total_tokens, primary_raw_fact_id, ingest_run_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(semantic_key) DO UPDATE SET
			recorded_at_ms = excluded.recorded_at_ms,
			provider = excluded.provider,
			model = excluded.model,
			quality = excluded.quality,
			is_countable = excluded.is_countable,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			reasoning_tokens = excluded.reasoning_tokens,
			cache_read_tokens = excluded.cache_read_tokens,
			cache_write_tokens = excluded.cache_write_tokens,
			total_tokens = excluded.total_tokens,
			primary_raw_fact_id = excluded.primary_raw_fact_id,
			ingest_run_id = excluded.ingest_run_id
	`, key, canonicalTime(row), row.Harness, sessionDBID, nullableInt64Ptr(messageDBID), normalizedText(row.Provider, "unknown"), normalizedText(row.Model, "unknown"),
		row.UsageScope, row.Quality, countable(row), nullIntValue(row.InputTokens), nullIntValue(row.OutputTokens), nullIntValue(row.ReasoningTokens),
		nullIntValue(row.CacheReadTokens), nullIntValue(row.CacheWriteTokens), canonicalTotal(row), row.ID, nullableNullInt(row.LastRunID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func insertDiagnostic(ctx context.Context, database *sql.DB, diagnostic Diagnostic, rawID *int64, runID *int64, recordedAtMs int64) error {
	keyParts := []string{
		string(diagnostic.Harness),
		diagnostic.RawFactKey,
		int64PtrValue(rawID),
		int64PtrValue(runID),
		diagnostic.Code,
	}
	_, err := database.ExecContext(ctx, `
		INSERT OR IGNORE INTO normalization_diagnostics (
			diagnostic_key, recorded_at_ms, harness, raw_fact_id, ingest_run_id, severity, code, message, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, stableHash(strings.Join(keyParts, "|")), recordedAtMs, diagnostic.Harness, nullableInt64Ptr(rawID), nullableInt64Ptr(runID), diagnostic.Severity, diagnostic.Code, diagnostic.Message, nullableString(diagnostic.MetadataJSON))
	return err
}

func canonicalTokenKey(row rawTokenRow) string {
	parts := []string{
		"token",
		string(row.Harness),
		row.SessionID.String,
		nullStringValue(row.MessageID),
		fmt.Sprint(canonicalTime(row)),
		row.UsageScope,
	}
	return stableHash(strings.Join(parts, "|"))
}

func canonicalTime(row rawTokenRow) int64 {
	if row.OccurredAtMs.Valid {
		return row.OccurredAtMs.Int64
	}
	return row.ObservedAtMs
}

func canonicalTotal(row rawTokenRow) int64 {
	if row.TotalTokens.Valid {
		return row.TotalTokens.Int64
	}
	return nullIntValue(row.InputTokens) +
		nullIntValue(row.OutputTokens) +
		nullIntValue(row.ReasoningTokens) +
		nullIntValue(row.CacheReadTokens) +
		nullIntValue(row.CacheWriteTokens)
}

func countable(row rawTokenRow) int {
	if strings.Contains(row.UsageScope, "fallback") {
		return 0
	}
	return 1
}

func normalizedText(value sql.NullString, fallback string) string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return fallback
	}
	return strings.TrimSpace(value.String)
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullIntValue(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableNullInt(value sql.NullInt64) interface{} {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableInt64Ptr(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func int64PtrValue(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func rawRunPointer(row rawTokenRow) *int64 {
	if !row.LastRunID.Valid {
		return nil
	}
	return &row.LastRunID.Int64
}

func nowMs(options NormalizeOptions) int64 {
	if options.Now.IsZero() {
		return time.Now().UnixMilli()
	}
	return options.Now.UnixMilli()
}
