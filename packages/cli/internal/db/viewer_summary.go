package db

import "context"

type ViewerSummaryRow struct {
	TotalTokens      int64 `json:"total"`
	InputTokens      int64 `json:"input"`
	OutputTokens     int64 `json:"output"`
	ReasoningTokens  int64 `json:"reasoning"`
	CacheReadTokens  int64 `json:"cacheRead"`
	CacheWriteTokens int64 `json:"cacheWrite"`
	SessionCount     int64 `json:"sessions"`
	SyncedSessions   int64 `json:"syncedSessions"`
}

func ViewerSummary(ctx context.Context, reader Reader, f Filter) (ViewerSummaryRow, error) {
	where, args := canonicalWhereClause(f)
	var result ViewerSummaryRow
	err := reader.QueryRowContext(ctx, `SELECT COALESCE(SUM(ctu.total_tokens),0),
		COALESCE(SUM(ctu.input_tokens),0), COALESCE(SUM(ctu.output_tokens),0),
		COALESCE(SUM(ctu.reasoning_tokens),0), COALESCE(SUM(ctu.cache_read_tokens),0),
		COALESCE(SUM(ctu.cache_write_tokens),0)
		FROM canonical_token_usage ctu JOIN canonical_sessions cs ON cs.id = ctu.session_id `+where, args...).Scan(
		&result.TotalTokens, &result.InputTokens, &result.OutputTokens, &result.ReasoningTokens,
		&result.CacheReadTokens, &result.CacheWriteTokens)
	if err != nil {
		return result, err
	}
	counts, err := ViewerSessionCounts(ctx, reader, f)
	result.SessionCount, result.SyncedSessions = counts.Shown, counts.Synced
	return result, err
}

// AvailableSessions returns a bounded, literal substring search ignoring its own facet.
func AvailableSessions(ctx context.Context, reader Reader, f Filter, search string, limit int) ([]string, error) {
	f.SessionIDs = nil
	where, args := canonicalWhereClause(f)
	args = append(args, search, limit)
	rows, err := reader.QueryContext(ctx, `SELECT DISTINCT cs.session_id
		FROM canonical_token_usage ctu JOIN canonical_sessions cs ON cs.id = ctu.session_id `+where+`
		AND instr(lower(cs.session_id), lower(?)) > 0 ORDER BY cs.session_id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
