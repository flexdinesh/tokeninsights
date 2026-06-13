package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Filter struct {
	Start      time.Time
	End        time.Time
	SessionIDs []string
	Providers  []string
	Models     []string
	Harnesses  []string
	DayFrom    string
	DayTo      string
}

func LastCompletedSync(ctx context.Context, db *sql.DB) (int64, error) {
	var value sql.NullInt64
	if err := db.QueryRowContext(ctx, `
		SELECT MAX(completed_at_ms)
		FROM ingest_runs
		WHERE status = 'completed'
	`).Scan(&value); err != nil {
		return 0, err
	}
	if !value.Valid {
		return 0, nil
	}
	return value.Int64, nil
}

type Event struct {
	RecordedAtMs     int64
	SessionID        string
	Provider         string
	Model            string
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func buildFilterArgs(f Filter) ([]interface{}, []string) {
	whereClause, args := canonicalWhereClause(f)
	trimmed := strings.TrimPrefix(whereClause, "WHERE ")
	if trimmed == "" {
		return args, nil
	}
	return args, []string{trimmed}
}

func Events(ctx context.Context, db *sql.DB, f Filter) ([]Event, error) {
	whereClause, args := canonicalWhereClause(f)
	query := fmt.Sprintf(`
		SELECT ctu.%s, cs.%s, ctu.%s, ctu.%s, ctu.%s, ctu.%s, ctu.%s, ctu.%s, ctu.%s, ctu.%s
		FROM %s ctu
		INNER JOIN %s cs ON cs.%s = ctu.%s
		%s
		ORDER BY ctu.%s DESC
	`,
		ColRecordedAtMs,
		ColSessionID,
		ColProvider,
		ColModel,
		ColInputTokens,
		ColOutputTokens,
		ColReasoningTokens,
		ColCacheReadTokens,
		ColCacheWriteTokens,
		ColTotalTokens,
		TableCanonicalTokenUsage,
		TableCanonicalSessions,
		ColID,
		ColSessionID,
		whereClause,
		ColRecordedAtMs,
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(
			&event.RecordedAtMs,
			&event.SessionID,
			&event.Provider,
			&event.Model,
			&event.InputTokens,
			&event.OutputTokens,
			&event.ReasoningTokens,
			&event.CacheReadTokens,
			&event.CacheWriteTokens,
			&event.TotalTokens,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}
