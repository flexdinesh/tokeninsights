package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

func dataTable(q query) string {
	if q.Quality == "estimated" {
		return "analytics.estimated"
	}
	return "analytics.confirmed"
}
func duckWhere(f db.Filter, timezone string) (string, []interface{}) {
	where := " WHERE countable"
	args := []interface{}{}
	if !f.Start.IsZero() {
		where += " AND occurred_at_ms>=?"
		args = append(args, f.Start.UnixMilli())
	}
	if !f.End.IsZero() {
		where += " AND occurred_at_ms<?"
		args = append(args, f.End.UnixMilli())
	}
	for _, filter := range []struct {
		column string
		values []string
	}{{"provider", f.Providers}, {"model", f.Models}, {"harness", f.Harnesses}, {"session_native_id", f.SessionIDs}, {"COALESCE(NULLIF(repository_key,''),'unknown')", f.RepositoryKeys}, {"COALESCE(NULLIF(directory_key,''),'unknown')", f.DirectoryKeys}} {
		if len(filter.values) == 0 {
			continue
		}
		where += " AND " + filter.column + " IN(" + strings.TrimSuffix(strings.Repeat("?,", len(filter.values)), ",") + ")"
		for _, value := range filter.values {
			args = append(args, value)
		}
	}
	for _, filter := range []struct{ value, op string }{{f.DayFrom, ">="}, {f.DayTo, "<="}} {
		if filter.value != "" {
			expression, zone := duckTimeExpression(timezone)
			where += " AND CAST(" + expression + " AS DATE) " + filter.op + " CAST(? AS DATE)"
			args = append(args, zone, filter.value)
		}
	}
	return where, args
}
func duckTimezone(now time.Time) string {
	name := reportingTimezone(time.Local, now)
	if strings.HasPrefix(name, "UTC+") || strings.HasPrefix(name, "UTC-") {
		_, offset := now.In(time.Local).Zone()
		return "@" + strconv.Itoa(offset)
	}
	return name
}

func duckTimeExpression(zone string) (string, string) {
	if strings.HasPrefix(zone, "@") {
		return "(epoch_ms(occurred_at_ms)+CAST(? AS BIGINT)*INTERVAL 1 SECOND)", strings.TrimPrefix(zone, "@")
	}
	return "timezone(?,timezone('UTC',epoch_ms(occurred_at_ms)))", zone
}
func duckTimeParameter(zone string) string {
	_, parameter := duckTimeExpression(zone)
	return parameter
}
func safeAggregate(values ...int64) error {
	for _, value := range values {
		if value < 0 || value > publication.SafeInteger {
			return fmt.Errorf("aggregate_limit")
		}
	}
	return nil
}
func queryFilter(q query, now time.Time) db.Filter {
	f := q.Selection.Filter(now)
	if q.Tab == "repo" {
		f.RepositoryKeys = q.RepositoryKeys
		f.DirectoryKeys = q.DirectoryKeys
	}
	return f
}
func duckSum(column string) string { return "CAST(COALESCE(SUM(" + column + "),0) AS BIGINT)" }
func dimensionSummary(column string) string {
	return "string_agg(DISTINCT " + column + ", ', ' ORDER BY " + column + ")"
}

// Only fixed SQL identifiers enter these statements. Filters, zone, limits and
// offsets are bound parameters; aggregation and pagination stay in DuckDB.
func groupedSQL(q query, where, zone string) string {
	key, name, group := "model", "model", "model"
	switch q.Tab {
	case "providers":
		key, name, group = "provider", "provider", "provider"
	case "harnesses":
		key, name, group = "harness", "harness", "harness"
	case "sessions":
		key, name, group = "harness || ':' || session_native_id", "session_native_id", "harness,session_native_id"
	case "tokens":
		local, _ := duckTimeExpression(zone)
		bucket := "strftime(" + local + ",'%Y-%m-%d')"
		switch q.Selection.Bucket {
		case "week":
			bucket = "strftime(date_trunc('week'," + local + "),'%Y-%m-%d')"
		case "month":
			bucket = "strftime(" + local + ",'%Y-%m')"
		case "year":
			bucket = "strftime(" + local + ",'%Y')"
		}
		key, name, group = "bucket", "bucket", "bucket"
		return "WITH filtered AS (SELECT *," + bucket + " AS bucket FROM " + dataTable(q) + where + ") " + groupedSelect(q, key, name, group, "filtered")
	case "repo":
		key = "COALESCE(NULLIF(repository_key,''),'unknown')"
		name = "COALESCE(NULLIF(MIN(repository_name),''),'unknown')"
		if q.LocationGroup == db.RepoGroupDirectory {
			key = "COALESCE(NULLIF(directory_key,''),'unknown')"
			name = "COALESCE(NULLIF(MIN(directory_name),''),'unknown')"
		}
		group = key
	case "context":
		return `WITH peaks AS (SELECT harness,provider,model,session_id,MAX(input_tokens+cache_read_tokens+cache_write_tokens) AS peak,MAX(occurred_at_ms) AS latest FROM ` + dataTable(q) + where + ` GROUP BY harness,provider,model,session_id)
 SELECT harness||':'||provider||':'||model AS key,model AS name,harness,provider,model,MAX(latest) AS date,COUNT(*) AS sessions,
 CAST(0 AS BIGINT) AS input,CAST(0 AS BIGINT) AS output,CAST(0 AS BIGINT) AS reasoning,CAST(0 AS BIGINT) AS cache_read,CAST(0 AS BIGINT) AS cache_write,CAST(0 AS BIGINT) AS total,CAST(0 AS BIGINT) AS context,
 CAST(FLOOR(AVG(peak)) AS BIGINT) AS average_context,CAST(FLOOR(MEDIAN(peak)) AS BIGINT) AS median_context,MAX(peak) AS max_context,
 '' AS location_key,'' AS location_name,'[]' AS directory_names,FALSE AS unknown_directory,'' AS repository_key,'' AS repository_name
 FROM peaks GROUP BY harness,provider,model`
	}
	return groupedSelect(q, key, name, group, dataTable(q)+where)
}
func groupedSelect(q query, key, name, group, from string) string {
	locationKey, locationName, dirs, unknown, repoKey, repoName := "''", "''", "'[]'", "FALSE", "''", "''"
	if q.Tab == "repo" {
		locationKey, locationName = key, name
		dirs = "CAST(to_json(list_sort(list(DISTINCT directory_name) FILTER(WHERE directory_name<>''))) AS VARCHAR)"
		dirs = "COALESCE(" + dirs + ",'[]')"
		unknown = "bool_or(directory_key='' OR directory_name='')"
		repoKey = "CASE WHEN COUNT(DISTINCT repository_key)=1 THEN MIN(repository_key) ELSE 'unknown' END"
		repoName = "CASE WHEN COUNT(DISTINCT repository_key)=1 THEN MIN(repository_name) ELSE 'unknown' END"
	}
	return "SELECT " + key + " AS key," + name + " AS name," + dimensionSummary("harness") + " AS harness," + dimensionSummary("provider") + " AS provider," + dimensionSummary("model") + " AS model,MAX(occurred_at_ms) AS date,COUNT(DISTINCT session_id) AS sessions," +
		duckSum("input_tokens") + " AS input," + duckSum("output_tokens") + " AS output," + duckSum("reasoning_tokens") + " AS reasoning," + duckSum("cache_read_tokens") + " AS cache_read," + duckSum("cache_write_tokens") + " AS cache_write," + duckSum("total_tokens") + " AS total,MAX(input_tokens+cache_read_tokens+cache_write_tokens) AS context," +
		"CAST(0 AS BIGINT) AS average_context,CAST(0 AS BIGINT) AS median_context,CAST(0 AS BIGINT) AS max_context," + locationKey + " AS location_key," + locationName + " AS location_name," + dirs + " AS directory_names," + unknown + " AS unknown_directory," + repoKey + " AS repository_key," + repoName + " AS repository_name FROM " + from + " GROUP BY " + group
}
func duckOrder(q query, sort, direction string) string {
	columns := map[string]string{"name": "name", "date": "date", "total": "total", "input": "input", "output": "output", "reasoning": "reasoning", "cacheRead": "cache_read", "cacheWrite": "cache_write", "sessions": "sessions", "context": "context", "averageContext": "average_context", "medianContext": "median_context", "maxContext": "max_context", "harness": "harness", "provider": "provider", "model": "model"}
	column := columns[sort]
	if column == "" {
		column = "total"
	}
	if direction != "asc" {
		direction = "desc"
	}
	return " ORDER BY " + column + " " + direction + ",key ASC"
}
func readDuckRows(ctx context.Context, tx *sql.Tx, statement string, args []interface{}) ([]Row, error) {
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []Row{}
	for rows.Next() {
		var row Row
		var dirs string
		if err := rows.Scan(&row.Key, &row.Name, &row.Harness, &row.Provider, &row.Model, &row.Date, &row.Sessions, &row.Input, &row.Output, &row.Reasoning, &row.CacheRead, &row.CacheWrite, &row.Total, &row.Context, &row.AverageContext, &row.MedianContext, &row.MaxContext, &row.LocationKey, &row.LocationName, &dirs, &row.HasUnknownDirectory, &row.RepositoryKey, &row.RepositoryName); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(dirs), &row.DirectoryNames); err != nil {
			return nil, err
		}
		if err := safeAggregate(row.Sessions, row.Input, row.Output, row.Reasoning, row.CacheRead, row.CacheWrite, row.Total, row.Context, row.AverageContext, row.MedianContext, row.MaxContext); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func loadDataDashboard(ctx context.Context, store *datastore.Store, q query, now time.Time) (dashboard, error) {
	result := dashboard{Page: q.Page, PageSize: q.PageSize, Quality: q.Quality}
	tx, err := store.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := datastore.ReadMetadata(ctx, tx)
	if err != nil {
		return result, err
	}
	result.DatabaseID = m.DatabaseID
	result.Revision = m.Revision
	result.InputRevision = m.InputRevision
	result.Generation = m.Generation
	result.LastSynced = m.LastIngestionAtMs
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM processing.scopes WHERE processed_revision<>revision OR generation<>?", m.TargetGeneration).Scan(&result.Pending); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM processing.outcomes o WHERE o.generation=? AND disposition='ambiguous' AND o.code='unusable_usage'", m.Generation).Scan(&result.Unresolved); err != nil {
		return result, err
	}
	where, args := duckWhere(queryFilter(q, now), duckTimezone(now))
	if err := tx.QueryRowContext(ctx, "SELECT "+duckSum("total_tokens")+","+duckSum("input_tokens")+","+duckSum("output_tokens")+","+duckSum("reasoning_tokens")+","+duckSum("cache_read_tokens")+","+duckSum("cache_write_tokens")+",COUNT(DISTINCT session_id) FROM "+dataTable(q)+where, args...).Scan(&result.Summary.TotalTokens, &result.Summary.InputTokens, &result.Summary.OutputTokens, &result.Summary.ReasoningTokens, &result.Summary.CacheReadTokens, &result.Summary.CacheWriteTokens, &result.Summary.SessionCount); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(DISTINCT session_id) FROM "+dataTable(q)+" WHERE countable").Scan(&result.Summary.SyncedSessions); err != nil {
		return result, err
	}
	if err := safeAggregate(result.Summary.TotalTokens, result.Summary.InputTokens, result.Summary.OutputTokens, result.Summary.ReasoningTokens, result.Summary.CacheReadTokens, result.Summary.CacheWriteTokens, result.Summary.SessionCount, result.Summary.SyncedSessions); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+dataTable(q)+where, args...).Scan(&result.FactCount); err != nil {
		return result, err
	}
	grouped := groupedSQL(q, where, duckTimezone(now))
	groupArgs := append([]interface{}{}, args...)
	if q.Tab == "tokens" {
		groupArgs = append([]interface{}{duckTimeParameter(duckTimezone(now))}, args...)
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM("+grouped+") grouped", groupArgs...).Scan(&result.RowCount); err != nil {
		return result, err
	}
	pages := max(1, (result.RowCount+q.PageSize-1)/q.PageSize)
	result.Page = min(q.Page, pages)
	pageArgs := append(append([]interface{}{}, groupArgs...), q.PageSize, (result.Page-1)*q.PageSize)
	result.Rows, err = readDuckRows(ctx, tx, grouped+duckOrder(q, q.Sort, q.Direction)+" LIMIT ? OFFSET ?", pageArgs)
	if err != nil {
		return result, err
	}
	chartQuery := q
	chartArgs := groupArgs
	chartSQL := grouped
	chartSort, chartDirection := "total", "desc"
	limit := chartLimit
	switch q.Tab {
	case "tokens", "sessions":
		chartQuery.Tab = "tokens"
		chartSQL = groupedSQL(chartQuery, where, duckTimezone(now))
		chartArgs = append([]interface{}{duckTimeParameter(duckTimezone(now))}, args...)
		chartSort, chartDirection = "date", "asc"
		limit = 1000
	case "context":
		chartSort = "averageContext"
	}
	result.Chart, err = readDuckRows(ctx, tx, chartSQL+duckOrder(chartQuery, chartSort, chartDirection)+" LIMIT ?", append(append([]interface{}{}, chartArgs...), limit))
	if err != nil {
		return result, err
	}
	result.Range = q.Selection.Period
	if result.Range == "all" {
		result.Range = "all time"
	}
	if q.Selection.From != "" || q.Selection.To != "" {
		result.Range = q.Selection.From + ".." + q.Selection.To
	}
	return result, tx.Commit()
}

func loadDataFacets(ctx context.Context, store *datastore.Store, q query, search string, now time.Time) (serverapi.UsageFacetsResponse, error) {
	result := serverapi.UsageFacetsResponse{Providers: []string{}, Models: []string{}, Harnesses: []serverapi.Harness{}, Sessions: []string{}, Repositories: []serverapi.LocationOption{}, Directories: []serverapi.LocationOption{}}
	tx, err := store.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, field := range []string{"provider", "model", "harness", "session_native_id", "repository_key", "directory_key"} {
		if (field == "repository_key" || field == "directory_key") && q.Tab != "repo" {
			continue
		}
		f := queryFilter(q, now)
		switch field {
		case "provider":
			f.Providers = nil
		case "model":
			f.Models = nil
		case "harness":
			f.Harnesses = nil
		case "session_native_id":
			f.SessionIDs = nil
		case "repository_key":
			f.RepositoryKeys = nil
		case "directory_key":
			f.DirectoryKeys = nil
		}
		where, args := duckWhere(f, duckTimezone(now))
		statement := "SELECT DISTINCT " + field + " FROM " + dataTable(q) + where
		limit := 1000
		if field == "session_native_id" {
			statement += " AND contains(lower(session_native_id),lower(?))"
			args = append(args, search)
			limit = sessionOptionLimit
		}
		location := field == "repository_key" || field == "directory_key"
		if location {
			label := strings.Replace(field, "_key", "_name", 1)
			statement = "SELECT COALESCE(NULLIF(" + field + ",''),'unknown'),COALESCE(NULLIF(MIN(" + label + "),''),'unknown') FROM " + dataTable(q) + where + " GROUP BY " + field
		}
		statement += " ORDER BY 1 LIMIT ?"
		args = append(args, limit)
		rows, err := tx.QueryContext(ctx, statement, args...)
		if err != nil {
			return result, fmt.Errorf("facets %s: %w", field, err)
		}
		for rows.Next() {
			if location {
				var option serverapi.LocationOption
				if err := rows.Scan(&option.Key, &option.Name); err != nil {
					_ = rows.Close()
					return result, err
				}
				if field == "repository_key" {
					result.Repositories = append(result.Repositories, option)
				} else {
					result.Directories = append(result.Directories, option)
				}
				continue
			}
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				return result, err
			}
			switch field {
			case "provider":
				result.Providers = append(result.Providers, value)
			case "model":
				result.Models = append(result.Models, value)
			case "harness":
				result.Harnesses = append(result.Harnesses, serverapi.Harness(value))
			case "session_native_id":
				result.Sessions = append(result.Sessions, value)
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return result, err
		}
	}
	m, err := datastore.ReadMetadata(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Revision = m.Revision
	result.DataEpoch = m.DatabaseID
	result.Generation = &m.Generation
	result.InputRevision = &m.InputRevision
	return result, tx.Commit()
}
