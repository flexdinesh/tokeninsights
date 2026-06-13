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

type Row struct {
	Harness          string
	Day              string
	Hour             string
	SessionID        string
	Provider         string
	Model            string
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
	ThroughputTokens int64
	DurationMs       int64
	TpsMean          float64
	TpsMedian        float64
	Requests         int64
	Retries          int64
	ToolName         string
	ToolCalls        int64
	ToolErrors       int64
	ThinkingLevels   string
	LatestAtMs       int64
}

const (
	canonicalAlias = "ctu"
	sessionAlias   = "cs"
	exprDay        = "date(ctu.recorded_at_ms/1000, 'unixepoch', 'localtime')"
	exprHour       = "strftime('%H:00', ctu.recorded_at_ms/1000, 'unixepoch', 'localtime')"
)

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
