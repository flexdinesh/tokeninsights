package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type GroupBy int

const (
	GroupByDay GroupBy = iota
	GroupByDayHour
	GroupByDaySession
)

const ColContextUsedTokens = "context_used_tokens"

type Row struct {
	Harness           string
	Day               string
	Hour              string
	SessionID         string
	Provider          string
	Model             string
	InputTokens       int64
	OutputTokens      int64
	ReasoningTokens   int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	ContextUsedTokens int64
	TotalTokens       int64
	ThroughputTokens  int64
	DurationMs        int64
	TpsMean           float64
	TpsMedian         float64
	Requests          int64
	Retries           int64
	ToolName          string
	ToolCalls         int64
	ToolErrors        int64
	ThinkingLevels    string
	LatestAtMs        int64
}

type TimeBucket string

const (
	BucketDay   TimeBucket = "day"
	BucketWeek  TimeBucket = "week"
	BucketMonth TimeBucket = "month"
	BucketYear  TimeBucket = "year"
)

type ViewerTokenBucketRow struct {
	Bucket            string
	SessionCount      int64
	InputTokens       int64
	OutputTokens      int64
	ReasoningTokens   int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	ContextUsedTokens int64
	TotalTokens       int64
	LatestAtMs        int64
}

type ViewerDimensionRow struct {
	Model             string
	Provider          string
	Harness           string
	Models            string
	Providers         string
	Harnesses         string
	SessionCount      int64
	InputTokens       int64
	OutputTokens      int64
	ReasoningTokens   int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	ContextUsedTokens int64
	TotalTokens       int64
	LatestAtMs        int64
}

type ViewerSessionRow struct {
	LatestAtMs        int64
	SessionID         string
	Harness           string
	Providers         string
	Models            string
	InputTokens       int64
	OutputTokens      int64
	ReasoningTokens   int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	ContextUsedTokens int64
	TotalTokens       int64
}

type ViewerContextRow struct {
	Harness                  string
	Provider                 string
	Model                    string
	SessionCount             int64
	AverageContextUsedTokens int64
	MedianContextUsedTokens  int64
	MaxContextUsedTokens     int64
	LatestAtMs               int64
}

const (
	canonicalAlias = "ctu"
	sessionAlias   = "cs"
	exprDay        = "date(ctu.recorded_at_ms/1000, 'unixepoch', 'localtime')"
	exprHour       = "strftime('%H:00', ctu.recorded_at_ms/1000, 'unixepoch', 'localtime')"
)

func contextUsedExpression(alias string) string {
	return fmt.Sprintf("COALESCE(%s.%s, 0) + COALESCE(%s.%s, 0) + COALESCE(%s.%s, 0)",
		alias, ColInputTokens,
		alias, ColCacheReadTokens,
		alias, ColCacheWriteTokens,
	)
}

func viewerBucketExpression(bucket TimeBucket) (string, error) {
	switch bucket {
	case BucketDay:
		return exprDay, nil
	case BucketWeek:
		return fmt.Sprintf("date(%s, printf('-%%d days', (CAST(strftime('%%w', %s) AS INTEGER) + 6) %% 7))", exprDay, exprDay), nil
	case BucketMonth:
		return "strftime('%Y-%m', ctu.recorded_at_ms/1000, 'unixepoch', 'localtime')", nil
	case BucketYear:
		return "strftime('%Y', ctu.recorded_at_ms/1000, 'unixepoch', 'localtime')", nil
	default:
		return "", fmt.Errorf("unsupported time bucket %q", bucket)
	}
}

func summaryValues(csv string) string {
	if strings.TrimSpace(csv) == "" {
		return ""
	}
	seen := make(map[string]bool)
	for _, value := range strings.Split(csv, ",") {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			seen[trimmed] = true
		}
	}
	values := sortedKeys(seen)
	return strings.Join(values, ", ")
}

func canonicalGroupSelect(g GroupBy) string {
	switch g {
	case GroupByDayHour:
		return fmt.Sprintf("%s AS day, %s AS hour, ctu.%s AS provider, ctu.%s AS model, ctu.%s AS harness", exprDay, exprHour, ColProvider, ColModel, ColHarness)
	case GroupByDaySession:
		return fmt.Sprintf("%s AS day, cs.%s AS session_id, ctu.%s AS provider, ctu.%s AS model, ctu.%s AS harness", exprDay, ColSessionID, ColProvider, ColModel, ColHarness)
	default:
		return fmt.Sprintf("%s AS day, ctu.%s AS provider, ctu.%s AS model, ctu.%s AS harness", exprDay, ColProvider, ColModel, ColHarness)
	}
}

func canonicalGroupBy(g GroupBy) string {
	switch g {
	case GroupByDayHour:
		return exprDay + ", " + exprHour + ", ctu.provider, ctu.model, ctu.harness"
	case GroupByDaySession:
		return exprDay + ", cs.session_id, ctu.provider, ctu.model, ctu.harness"
	default:
		return exprDay + ", ctu.provider, ctu.model, ctu.harness"
	}
}

func scanCanonicalTokenRow(rows *sql.Rows, g GroupBy) (Row, error) {
	var row Row
	var scanArgs []interface{}
	switch g {
	case GroupByDayHour:
		scanArgs = []interface{}{&row.Day, &row.Hour, &row.Provider, &row.Model, &row.Harness}
	case GroupByDaySession:
		scanArgs = []interface{}{&row.Day, &row.SessionID, &row.Provider, &row.Model, &row.Harness}
	default:
		scanArgs = []interface{}{&row.Day, &row.Provider, &row.Model, &row.Harness}
	}
	scanArgs = append(scanArgs,
		&row.InputTokens,
		&row.OutputTokens,
		&row.ReasoningTokens,
		&row.CacheReadTokens,
		&row.CacheWriteTokens,
		&row.ContextUsedTokens,
		&row.TotalTokens,
		&row.LatestAtMs,
	)
	if err := rows.Scan(scanArgs...); err != nil {
		return Row{}, err
	}
	return row, nil
}

func canonicalWhereClause(f Filter) (string, []interface{}) {
	var args []interface{}
	var where []string

	where = append(where, "ctu."+ColIsCountable+" = 1")
	if !f.Start.IsZero() {
		where = append(where, "ctu."+ColRecordedAtMs+" >= ?")
		args = append(args, f.Start.UnixMilli())
	}
	if !f.End.IsZero() {
		where = append(where, "ctu."+ColRecordedAtMs+" < ?")
		args = append(args, f.End.UnixMilli())
	}
	if len(f.SessionIDs) > 0 {
		where = append(where, "cs."+ColSessionID+" IN ("+placeholders(len(f.SessionIDs))+")")
		for _, value := range f.SessionIDs {
			args = append(args, value)
		}
	}
	if len(f.Providers) > 0 {
		where = append(where, "ctu."+ColProvider+" IN ("+placeholders(len(f.Providers))+")")
		for _, value := range f.Providers {
			args = append(args, value)
		}
	}
	if len(f.Models) > 0 {
		where = append(where, "ctu."+ColModel+" IN ("+placeholders(len(f.Models))+")")
		for _, value := range f.Models {
			args = append(args, value)
		}
	}
	if len(f.Harnesses) > 0 {
		where = append(where, "ctu."+ColHarness+" IN ("+placeholders(len(f.Harnesses))+")")
		for _, value := range f.Harnesses {
			args = append(args, value)
		}
	}
	if f.DayFrom != "" {
		where = append(where, "date(ctu."+ColRecordedAtMs+"/1000, 'unixepoch', 'localtime') >= ?")
		args = append(args, f.DayFrom)
	}
	if f.DayTo != "" {
		where = append(where, "date(ctu."+ColRecordedAtMs+"/1000, 'unixepoch', 'localtime') <= ?")
		args = append(args, f.DayTo)
	}

	return "WHERE " + strings.Join(where, " AND "), args
}

func Aggregate(ctx context.Context, db *sql.DB, f Filter, g GroupBy) ([]Row, error) {
	return AggregateTokens(ctx, db, f, g)
}

func AggregateTokens(ctx context.Context, db *sql.DB, f Filter, g GroupBy) ([]Row, error) {
	whereClause, args := canonicalWhereClause(f)
	query := fmt.Sprintf(`
		SELECT %s,
			SUM(ctu.%s) AS input_tokens,
			SUM(ctu.%s) AS output_tokens,
			SUM(ctu.%s) AS reasoning_tokens,
			SUM(ctu.%s) AS cache_read_tokens,
			SUM(ctu.%s) AS cache_write_tokens,
			MAX(%s) AS %s,
			SUM(ctu.%s) AS total_tokens,
			MAX(ctu.%s) AS latest_at_ms
		FROM %s ctu
		INNER JOIN %s cs ON cs.%s = ctu.%s
		%s
		GROUP BY %s
	`,
		canonicalGroupSelect(g),
		ColInputTokens,
		ColOutputTokens,
		ColReasoningTokens,
		ColCacheReadTokens,
		ColCacheWriteTokens,
		contextUsedExpression(canonicalAlias),
		ColContextUsedTokens,
		ColTotalTokens,
		ColRecordedAtMs,
		TableCanonicalTokenUsage,
		TableCanonicalSessions,
		ColID,
		ColSessionID,
		whereClause,
		canonicalGroupBy(g),
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		row, err := scanCanonicalTokenRow(rows, g)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortRows(result, g)
	return result, nil
}

func ViewerTokenBuckets(ctx context.Context, db *sql.DB, f Filter, bucket TimeBucket) ([]ViewerTokenBucketRow, error) {
	bucketExpr, err := viewerBucketExpression(bucket)
	if err != nil {
		return nil, err
	}
	whereClause, args := canonicalWhereClause(f)
	query := fmt.Sprintf(`
		SELECT %s AS bucket,
			COUNT(DISTINCT ctu.%s) AS session_count,
			SUM(ctu.%s) AS input_tokens,
			SUM(ctu.%s) AS output_tokens,
			SUM(ctu.%s) AS reasoning_tokens,
			SUM(ctu.%s) AS cache_read_tokens,
			SUM(ctu.%s) AS cache_write_tokens,
			MAX(%s) AS %s,
			SUM(ctu.%s) AS total_tokens,
			MAX(ctu.%s) AS latest_at_ms
		FROM %s ctu
		INNER JOIN %s cs ON cs.%s = ctu.%s
		%s
		GROUP BY %s
		ORDER BY latest_at_ms DESC, bucket DESC
	`,
		bucketExpr,
		ColSessionID,
		ColInputTokens,
		ColOutputTokens,
		ColReasoningTokens,
		ColCacheReadTokens,
		ColCacheWriteTokens,
		contextUsedExpression(canonicalAlias),
		ColContextUsedTokens,
		ColTotalTokens,
		ColRecordedAtMs,
		TableCanonicalTokenUsage,
		TableCanonicalSessions,
		ColID,
		ColSessionID,
		whereClause,
		bucketExpr,
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ViewerTokenBucketRow
	for rows.Next() {
		var row ViewerTokenBucketRow
		if err := rows.Scan(
			&row.Bucket,
			&row.SessionCount,
			&row.InputTokens,
			&row.OutputTokens,
			&row.ReasoningTokens,
			&row.CacheReadTokens,
			&row.CacheWriteTokens,
			&row.ContextUsedTokens,
			&row.TotalTokens,
			&row.LatestAtMs,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func ViewerModels(ctx context.Context, db *sql.DB, f Filter) ([]ViewerDimensionRow, error) {
	return viewerDimensions(ctx, db, f, ColModel)
}

func ViewerProviders(ctx context.Context, db *sql.DB, f Filter) ([]ViewerDimensionRow, error) {
	return viewerDimensions(ctx, db, f, ColProvider)
}

func ViewerHarnesses(ctx context.Context, db *sql.DB, f Filter) ([]ViewerDimensionRow, error) {
	return viewerDimensions(ctx, db, f, ColHarness)
}

func ViewerContext(ctx context.Context, db *sql.DB, f Filter) ([]ViewerContextRow, error) {
	whereClause, args := canonicalWhereClause(f)
	query := fmt.Sprintf(`
		WITH per_session AS (
			SELECT ctu.%s AS harness,
				ctu.%s AS provider,
				ctu.%s AS model,
				ctu.%s AS session_id,
				MAX(%s) AS session_peak_context,
				MAX(ctu.%s) AS latest_at_ms
			FROM %s ctu
			INNER JOIN %s cs ON cs.%s = ctu.%s
			%s
			GROUP BY ctu.%s, ctu.%s, ctu.%s, ctu.%s
		),
		ranked AS (
			SELECT harness,
				provider,
				model,
				session_peak_context,
				latest_at_ms,
				ROW_NUMBER() OVER (
					PARTITION BY harness, provider, model
					ORDER BY session_peak_context
				) AS rank,
				COUNT(*) OVER (
					PARTITION BY harness, provider, model
				) AS session_count
			FROM per_session
		)
		SELECT harness,
			provider,
			model,
			session_count,
			CAST(SUM(session_peak_context) / session_count AS INTEGER) AS average_context_used_tokens,
			CAST(AVG(CASE
				WHEN rank IN ((session_count + 1) / 2, (session_count + 2) / 2)
				THEN session_peak_context
			END) AS INTEGER) AS median_context_used_tokens,
			MAX(session_peak_context) AS max_context_used_tokens,
			MAX(latest_at_ms) AS latest_at_ms
		FROM ranked
		GROUP BY harness, provider, model, session_count
		ORDER BY average_context_used_tokens DESC, harness ASC, provider ASC, model ASC
	`,
		ColHarness,
		ColProvider,
		ColModel,
		ColSessionID,
		contextUsedExpression(canonicalAlias),
		ColRecordedAtMs,
		TableCanonicalTokenUsage,
		TableCanonicalSessions,
		ColID,
		ColSessionID,
		whereClause,
		ColHarness,
		ColProvider,
		ColModel,
		ColSessionID,
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ViewerContextRow
	for rows.Next() {
		var row ViewerContextRow
		if err := rows.Scan(
			&row.Harness,
			&row.Provider,
			&row.Model,
			&row.SessionCount,
			&row.AverageContextUsedTokens,
			&row.MedianContextUsedTokens,
			&row.MaxContextUsedTokens,
			&row.LatestAtMs,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func viewerDimensions(ctx context.Context, db *sql.DB, f Filter, primaryColumn string) ([]ViewerDimensionRow, error) {
	whereClause, args := canonicalWhereClause(f)
	query := fmt.Sprintf(`
		SELECT ctu.%s AS primary_value,
			GROUP_CONCAT(DISTINCT ctu.%s) AS providers,
			GROUP_CONCAT(DISTINCT ctu.%s) AS models,
			GROUP_CONCAT(DISTINCT ctu.%s) AS harnesses,
			COUNT(DISTINCT ctu.%s) AS session_count,
			SUM(ctu.%s) AS input_tokens,
			SUM(ctu.%s) AS output_tokens,
			SUM(ctu.%s) AS reasoning_tokens,
			SUM(ctu.%s) AS cache_read_tokens,
			SUM(ctu.%s) AS cache_write_tokens,
			MAX(%s) AS %s,
			SUM(ctu.%s) AS total_tokens,
			MAX(ctu.%s) AS latest_at_ms
		FROM %s ctu
		INNER JOIN %s cs ON cs.%s = ctu.%s
		%s
		GROUP BY ctu.%s
		ORDER BY total_tokens DESC, primary_value ASC
	`,
		primaryColumn,
		ColProvider,
		ColModel,
		ColHarness,
		ColSessionID,
		ColInputTokens,
		ColOutputTokens,
		ColReasoningTokens,
		ColCacheReadTokens,
		ColCacheWriteTokens,
		contextUsedExpression(canonicalAlias),
		ColContextUsedTokens,
		ColTotalTokens,
		ColRecordedAtMs,
		TableCanonicalTokenUsage,
		TableCanonicalSessions,
		ColID,
		ColSessionID,
		whereClause,
		primaryColumn,
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ViewerDimensionRow
	for rows.Next() {
		var primaryValue string
		var providers string
		var models string
		var harnesses string
		var row ViewerDimensionRow
		if err := rows.Scan(
			&primaryValue,
			&providers,
			&models,
			&harnesses,
			&row.SessionCount,
			&row.InputTokens,
			&row.OutputTokens,
			&row.ReasoningTokens,
			&row.CacheReadTokens,
			&row.CacheWriteTokens,
			&row.ContextUsedTokens,
			&row.TotalTokens,
			&row.LatestAtMs,
		); err != nil {
			return nil, err
		}
		switch primaryColumn {
		case ColModel:
			row.Model = primaryValue
		case ColProvider:
			row.Provider = primaryValue
		case ColHarness:
			row.Harness = primaryValue
		}
		row.Providers = summaryValues(providers)
		row.Models = summaryValues(models)
		row.Harnesses = summaryValues(harnesses)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func ViewerSessions(ctx context.Context, db *sql.DB, f Filter) ([]ViewerSessionRow, error) {
	whereClause, args := canonicalWhereClause(f)
	query := fmt.Sprintf(`
		SELECT MAX(ctu.%s) AS latest_at_ms,
			cs.%s AS session_id,
			GROUP_CONCAT(DISTINCT ctu.%s) AS harnesses,
			GROUP_CONCAT(DISTINCT ctu.%s) AS providers,
			GROUP_CONCAT(DISTINCT ctu.%s) AS models,
			SUM(ctu.%s) AS input_tokens,
			SUM(ctu.%s) AS output_tokens,
			SUM(ctu.%s) AS reasoning_tokens,
			SUM(ctu.%s) AS cache_read_tokens,
			SUM(ctu.%s) AS cache_write_tokens,
			MAX(%s) AS %s,
			SUM(ctu.%s) AS total_tokens
		FROM %s ctu
		INNER JOIN %s cs ON cs.%s = ctu.%s
		%s
		GROUP BY ctu.%s, cs.%s
		ORDER BY latest_at_ms DESC, session_id ASC
	`,
		ColRecordedAtMs,
		ColSessionID,
		ColHarness,
		ColProvider,
		ColModel,
		ColInputTokens,
		ColOutputTokens,
		ColReasoningTokens,
		ColCacheReadTokens,
		ColCacheWriteTokens,
		contextUsedExpression(canonicalAlias),
		ColContextUsedTokens,
		ColTotalTokens,
		TableCanonicalTokenUsage,
		TableCanonicalSessions,
		ColID,
		ColSessionID,
		whereClause,
		ColSessionID,
		ColSessionID,
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ViewerSessionRow
	for rows.Next() {
		var harnesses string
		var providers string
		var models string
		var row ViewerSessionRow
		if err := rows.Scan(
			&row.LatestAtMs,
			&row.SessionID,
			&harnesses,
			&providers,
			&models,
			&row.InputTokens,
			&row.OutputTokens,
			&row.ReasoningTokens,
			&row.CacheReadTokens,
			&row.CacheWriteTokens,
			&row.ContextUsedTokens,
			&row.TotalTokens,
		); err != nil {
			return nil, err
		}
		row.Harness = summaryValues(harnesses)
		row.Providers = summaryValues(providers)
		row.Models = summaryValues(models)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func AggregateTPS(context.Context, *sql.DB, Filter, GroupBy) ([]Row, error) {
	return []Row{}, nil
}

func AggregateRequests(context.Context, *sql.DB, Filter, GroupBy) ([]Row, error) {
	return []Row{}, nil
}

func AggregateToolCalls(context.Context, *sql.DB, Filter, GroupBy) ([]Row, error) {
	return []Row{}, nil
}

func AggregateToolBreakdown(context.Context, *sql.DB, Filter, GroupBy) ([]Row, error) {
	return []Row{}, nil
}

func sortRows(rows []Row, g GroupBy) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LatestAtMs != rows[j].LatestAtMs {
			return rows[i].LatestAtMs > rows[j].LatestAtMs
		}
		if rows[i].Day != rows[j].Day {
			return rows[i].Day > rows[j].Day
		}
		if g == GroupByDayHour && rows[i].Hour != rows[j].Hour {
			return rows[i].Hour > rows[j].Hour
		}
		if rows[i].Harness != rows[j].Harness {
			return rows[i].Harness < rows[j].Harness
		}
		if g == GroupByDaySession && rows[i].SessionID != rows[j].SessionID {
			return rows[i].SessionID < rows[j].SessionID
		}
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		return rows[i].Model < rows[j].Model
	})
}
