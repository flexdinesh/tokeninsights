package server

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const defaultPageSize = 50
const maxPageSize = 200
const chartLimit = 12
const sessionOptionLimit = 100

type query struct {
	Selection viewer.Selection
	Tab       string
	Sort      string
	Direction string
	Page      int
	PageSize  int
}

func parseQuery(values url.Values) (query, error) {
	q := query{Selection: viewer.Selection{Period: "month", Bucket: "day"}, Tab: "tokens", Page: 1, PageSize: defaultPageSize}
	if values.Has("period") {
		q.Selection.Period = values.Get("period")
	}
	if values.Has("bucket") {
		q.Selection.Bucket = values.Get("bucket")
	}
	q.Selection.From, q.Selection.To = values.Get("from"), values.Get("to")
	q.Selection.Providers, q.Selection.Models = values["provider"], values["model"]
	q.Selection.Harnesses, q.Selection.Sessions = values["harness"], values["session"]
	if err := q.Selection.Validate(); err != nil {
		return q, err
	}
	if values.Has("tab") {
		q.Tab = values.Get("tab")
	}
	switch q.Tab {
	case "tokens", "models", "providers", "harnesses", "sessions", "context":
	default:
		return q, fmt.Errorf("invalid tab")
	}
	q.Sort = values.Get("sort")
	if q.Sort == "" {
		q.Sort = "total"
		if q.Tab == "tokens" || q.Tab == "sessions" {
			q.Sort = "date"
		}
		if q.Tab == "context" {
			q.Sort = "averageContext"
		}
	}
	allowed := map[string]bool{"name": true, "date": true, "total": true, "input": true, "output": true, "reasoning": true, "cacheRead": true, "cacheWrite": true, "sessions": true, "context": true}
	if q.Tab == "context" {
		allowed = map[string]bool{"averageContext": true, "medianContext": true, "maxContext": true, "sessions": true, "harness": true, "provider": true, "model": true}
	}
	if !allowed[q.Sort] {
		return q, fmt.Errorf("invalid sort for %s", q.Tab)
	}
	q.Direction = values.Get("direction")
	if q.Direction == "" {
		q.Direction = "desc"
		if q.Sort == "name" || q.Sort == "harness" || q.Sort == "provider" || q.Sort == "model" {
			q.Direction = "asc"
		}
	}
	if q.Direction != "asc" && q.Direction != "desc" {
		return q, fmt.Errorf("invalid sort direction")
	}
	for key, target := range map[string]*int{"page": &q.Page, "pageSize": &q.PageSize} {
		if values.Has(key) {
			n, err := strconv.Atoi(values.Get(key))
			if err != nil || n < 1 {
				return q, fmt.Errorf("invalid %s", key)
			}
			*target = n
		}
	}
	if q.PageSize > maxPageSize {
		return q, fmt.Errorf("pageSize must not exceed %d", maxPageSize)
	}
	return q, nil
}

// Row is a numeric analytics result, independent of API transport.
type Row struct {
	Key            string
	Name           string
	Harness        string
	Provider       string
	Model          string
	Date           int64
	Sessions       int64
	Input          int64
	Output         int64
	Reasoning      int64
	CacheRead      int64
	CacheWrite     int64
	Total          int64
	Context        int64
	AverageContext int64
	MedianContext  int64
	MaxContext     int64
}

type dashboard struct {
	Rows       []Row
	Chart      []Row
	RowCount   int
	Page       int
	PageSize   int
	Summary    db.ViewerSummaryRow
	LastSynced int64
	Range      string
}

func loadRows(ctx context.Context, reader db.Reader, f db.Filter, q query) ([]Row, error) {
	result := []Row{}
	switch q.Tab {
	case "tokens":
		rows, err := db.ViewerTokenBuckets(ctx, reader, f, db.TimeBucket(q.Selection.Bucket))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			result = append(result, Row{Key: r.Bucket, Name: r.Bucket, Date: r.LatestAtMs, Sessions: r.SessionCount, Input: r.InputTokens, Output: r.OutputTokens, Reasoning: r.ReasoningTokens, CacheRead: r.CacheReadTokens, CacheWrite: r.CacheWriteTokens, Total: r.TotalTokens, Context: r.ContextUsedTokens})
		}
	case "sessions":
		rows, err := db.ViewerSessions(ctx, reader, f)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			result = append(result, Row{Key: r.Harness + ":" + r.SessionID, Name: r.SessionID, Date: r.LatestAtMs, Harness: r.Harness, Provider: r.Providers, Model: r.Models, Sessions: 1, Input: r.InputTokens, Output: r.OutputTokens, Reasoning: r.ReasoningTokens, CacheRead: r.CacheReadTokens, CacheWrite: r.CacheWriteTokens, Total: r.TotalTokens, Context: r.ContextUsedTokens})
		}
	case "context":
		rows, err := db.ViewerContext(ctx, reader, f)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			result = append(result, Row{Key: strings.Join([]string{r.Harness, r.Provider, r.Model}, "\x00"), Name: r.Model, Harness: r.Harness, Provider: r.Provider, Model: r.Model, Date: r.LatestAtMs, Sessions: r.SessionCount, AverageContext: r.AverageContextUsedTokens, MedianContext: r.MedianContextUsedTokens, MaxContext: r.MaxContextUsedTokens})
		}
	default:
		var rows []db.ViewerDimensionRow
		var err error
		switch q.Tab {
		case "models":
			rows, err = db.ViewerModels(ctx, reader, f)
		case "providers":
			rows, err = db.ViewerProviders(ctx, reader, f)
		case "harnesses":
			rows, err = db.ViewerHarnesses(ctx, reader, f)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			name := r.Model
			if q.Tab == "providers" {
				name = r.Provider
			}
			if q.Tab == "harnesses" {
				name = r.Harness
			}
			result = append(result, Row{Key: name, Name: name, Harness: r.Harnesses, Provider: r.Providers, Model: r.Models, Date: r.LatestAtMs, Sessions: r.SessionCount, Input: r.InputTokens, Output: r.OutputTokens, Reasoning: r.ReasoningTokens, CacheRead: r.CacheReadTokens, CacheWrite: r.CacheWriteTokens, Total: r.TotalTokens, Context: r.ContextUsedTokens})
		}
	}
	return result, nil
}

func numberValue(r Row, key string) int64 {
	switch key {
	case "date":
		return r.Date
	case "sessions":
		return r.Sessions
	case "input":
		return r.Input
	case "output":
		return r.Output
	case "reasoning":
		return r.Reasoning
	case "cacheRead":
		return r.CacheRead
	case "cacheWrite":
		return r.CacheWrite
	case "context":
		return r.Context
	case "averageContext":
		return r.AverageContext
	case "medianContext":
		return r.MedianContext
	case "maxContext":
		return r.MaxContext
	default:
		return r.Total
	}
}

func sortRows(rows []Row, key, direction, tab string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		comparison := 0
		switch key {
		case "name":
			comparison = strings.Compare(a.Name, b.Name)
		case "harness":
			comparison = strings.Compare(a.Harness, b.Harness)
		case "provider":
			comparison = strings.Compare(a.Provider, b.Provider)
		case "model":
			comparison = strings.Compare(a.Model, b.Model)
		default:
			if key == "date" && tab == "tokens" {
				comparison = strings.Compare(a.Name, b.Name)
			} else {
				x, y := numberValue(a, key), numberValue(b, key)
				if x < y {
					comparison = -1
				}
				if x > y {
					comparison = 1
				}
			}
		}
		if comparison == 0 {
			if a.Total != b.Total {
				return a.Total > b.Total
			}
			return a.Key < b.Key
		}
		if direction == "asc" {
			return comparison < 0
		}
		return comparison > 0
	})
}

func loadDashboard(ctx context.Context, path string, q query, now time.Time) (dashboard, error) {
	result := dashboard{Page: q.Page, PageSize: q.PageSize}
	database, err := db.Open(path)
	if err != nil {
		return result, err
	}
	defer func() { _ = database.Close() }()
	tx, err := db.BeginAnalyticsRead(ctx, database)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	f := q.Selection.Filter(now)
	rows, err := loadRows(ctx, tx, f, q)
	if err != nil {
		return result, err
	}
	result.Summary, err = db.ViewerSummary(ctx, tx, f)
	if err != nil {
		return result, err
	}
	result.LastSynced, err = db.LastCompletedSync(ctx, tx)
	if err != nil {
		return result, err
	}
	result.RowCount = len(rows)
	result.Chart = append([]Row{}, rows...)
	if q.Tab == "sessions" {
		chartQuery := q
		chartQuery.Tab = "tokens"
		result.Chart, err = loadRows(ctx, tx, f, chartQuery)
		if err != nil {
			return result, err
		}
	}
	if q.Tab == "tokens" || q.Tab == "sessions" {
		sortRows(result.Chart, "date", "asc", "tokens")
	} else {
		metric := "total"
		if q.Tab == "context" {
			metric = "averageContext"
		}
		sortRows(result.Chart, metric, "desc", q.Tab)
		if len(result.Chart) > chartLimit {
			result.Chart = result.Chart[:chartLimit]
		}
	}
	sortRows(rows, q.Sort, q.Direction, q.Tab)
	pages := max(1, (len(rows)+q.PageSize-1)/q.PageSize)
	result.Page = min(q.Page, pages)
	start := (result.Page - 1) * q.PageSize
	result.Rows = rows[start:min(start+q.PageSize, len(rows))]
	result.Range = q.Selection.Period
	if result.Range == "all" {
		result.Range = "all time"
	}
	if q.Selection.From != "" || q.Selection.To != "" {
		result.Range = q.Selection.From + ".." + q.Selection.To
	}
	return result, tx.Commit()
}
