package sqlanalytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
)

func dataTable(q analytics.Query) string {
	if q.Quality == "estimated" {
		return "analytics_estimated"
	}
	return "analytics_confirmed"
}
func queryWhere(f querymodel.Filter, timezone, datasetID string) (string, []interface{}, error) {
	where := " WHERE countable AND dataset_id=?"
	args := []interface{}{datasetID}
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
	loc, err := location(timezone)
	if err != nil {
		return "", nil, err
	}
	for _, filter := range []struct {
		value, op string
		next      bool
	}{{f.DayFrom, ">=", false}, {f.DayTo, "<", true}} {
		if filter.value == "" {
			continue
		}
		day, err := time.ParseInLocation(time.DateOnly, filter.value, loc)
		if err != nil {
			return "", nil, err
		}
		if filter.next {
			day = day.AddDate(0, 0, 1)
		}
		where += " AND occurred_at_ms" + filter.op + "?"
		args = append(args, day.UnixMilli())
	}
	return where, args, nil
}

func reportingZone(now time.Time) string {
	name := analytics.ReportingTimezone(time.Local, now)
	if strings.HasPrefix(name, "UTC+") || strings.HasPrefix(name, "UTC-") {
		_, offset := now.In(time.Local).Zone()
		return "@" + strconv.Itoa(offset)
	}
	return name
}

func safeAggregate(values ...int64) error {
	for _, value := range values {
		if value < 0 || value > publication.SafeInteger {
			return fmt.Errorf("aggregate_limit")
		}
	}
	return nil
}
func queryFilter(q analytics.Query, now time.Time) querymodel.Filter {
	f := q.Selection.Filter(now)
	if q.Tab == "repo" {
		f.RepositoryKeys = q.RepositoryKeys
		f.DirectoryKeys = q.DirectoryKeys
	}
	return f
}
func sumSQL(column string) string { return "CAST(COALESCE(SUM(" + column + "),0) AS BIGINT)" }
func dimensionSummary(column string, postgres bool) string {
	if postgres {
		return "string_agg(DISTINCT " + column + ", ', ' ORDER BY " + column + ")"
	}
	return "ti_dimensions(" + column + ")"
}

// Only fixed SQL identifiers enter these statements. Filters, zone, limits and
// offsets are bound parameters; aggregation and pagination stay in SQL.
func groupedSQL(q analytics.Query, where, zone string, postgres bool) string {
	key, name, group := "model", "model", "model"
	switch q.Tab {
	case "providers":
		key, name, group = "provider", "provider", "provider"
	case "harnesses":
		key, name, group = "harness", "harness", "harness"
	case "sessions":
		key, name, group = "harness || ':' || session_native_id", "session_native_id", "harness,session_native_id"
	case "tokens":
		bucket := bucketSQL(zone, q.Selection.Bucket, postgres)
		key, name, group = "bucket", "bucket", "bucket"
		return "WITH filtered AS (SELECT *," + bucket + " AS bucket FROM " + dataTable(q) + where + ") " + groupedSelect(q, key, name, group, "filtered", postgres)
	case "repo":
		key = "COALESCE(NULLIF(repository_key,''),'unknown')"
		name = "COALESCE(NULLIF(MIN(repository_name),''),'unknown')"
		if q.LocationGroup == querymodel.RepoGroupDirectory {
			key = "COALESCE(NULLIF(directory_key,''),'unknown')"
			name = "COALESCE(NULLIF(MIN(directory_name),''),'unknown')"
		}
		group = key
	case "context":
		return `WITH peaks AS (SELECT harness,provider,model,session_id,MAX(input_tokens+cache_read_tokens+cache_write_tokens) AS peak,MAX(occurred_at_ms) AS latest FROM ` + dataTable(q) + where + ` GROUP BY harness,provider,model,session_id), ranked AS (SELECT *,ROW_NUMBER() OVER(PARTITION BY harness,provider,model ORDER BY peak) AS position,COUNT(*) OVER(PARTITION BY harness,provider,model) AS size FROM peaks)
 SELECT harness||':'||provider||':'||model AS key,model AS name,harness,provider,model,MAX(latest) AS date,COUNT(*) AS sessions,
 CAST(0 AS BIGINT) AS input,CAST(0 AS BIGINT) AS output,CAST(0 AS BIGINT) AS reasoning,CAST(0 AS BIGINT) AS cache_read,CAST(0 AS BIGINT) AS cache_write,CAST(0 AS BIGINT) AS total,CAST(0 AS BIGINT) AS context,
 CAST(FLOOR(SUM(peak)/COUNT(*)) AS BIGINT) AS average_context,CAST(FLOOR(SUM(CASE WHEN position IN((size+1)/2,(size+2)/2) THEN peak ELSE 0 END)/SUM(CASE WHEN position IN((size+1)/2,(size+2)/2) THEN 1 ELSE 0 END)) AS BIGINT) AS median_context,MAX(peak) AS max_context,
 '' AS location_key,'' AS location_name,'[]' AS directory_names,FALSE AS unknown_directory,'' AS repository_key,'' AS repository_name
 FROM ranked GROUP BY harness,provider,model`
	}
	return groupedSelect(q, key, name, group, dataTable(q)+where, postgres)
}
func groupedSelect(q analytics.Query, key, name, group, from string, postgres bool) string {
	locationKey, locationName, dirs, unknown, repoKey, repoName := "''", "''", "'[]'", "FALSE", "''", "''"
	if q.Tab == "repo" {
		locationKey, locationName = key, name
		dirs = "ti_directories(directory_name) FILTER(WHERE directory_name<>'')"
		if postgres {
			dirs = "CAST(json_agg(DISTINCT directory_name ORDER BY directory_name) FILTER(WHERE directory_name<>'') AS TEXT)"
		}
		dirs = "COALESCE(" + dirs + ",'[]')"
		unknown = "(MAX(CASE WHEN directory_key='' OR directory_name='' THEN 1 ELSE 0 END)=1)"
		repoKey = "CASE WHEN COUNT(DISTINCT repository_key)=1 THEN MIN(repository_key) ELSE 'unknown' END"
		repoName = "CASE WHEN COUNT(DISTINCT repository_key)=1 THEN MIN(repository_name) ELSE 'unknown' END"
	}
	return "SELECT " + key + " AS key," + name + " AS name," + dimensionSummary("harness", postgres) + " AS harness," + dimensionSummary("provider", postgres) + " AS provider," + dimensionSummary("model", postgres) + " AS model,MAX(occurred_at_ms) AS date,COUNT(DISTINCT session_id) AS sessions," +
		sumSQL("input_tokens") + " AS input," + sumSQL("output_tokens") + " AS output," + sumSQL("reasoning_tokens") + " AS reasoning," + sumSQL("cache_read_tokens") + " AS cache_read," + sumSQL("cache_write_tokens") + " AS cache_write," + sumSQL("total_tokens") + " AS total,MAX(input_tokens+cache_read_tokens+cache_write_tokens) AS context," +
		"CAST(0 AS BIGINT) AS average_context,CAST(0 AS BIGINT) AS median_context,CAST(0 AS BIGINT) AS max_context," + locationKey + " AS location_key," + locationName + " AS location_name," + dirs + " AS directory_names," + unknown + " AS unknown_directory," + repoKey + " AS repository_key," + repoName + " AS repository_name FROM " + from + " GROUP BY " + group
}
func orderSQL(q analytics.Query, sort, direction string) string {
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
func readRows(ctx context.Context, tx *sql.Tx, statement string, args []interface{}) ([]analytics.Row, error) {
	rows, err := tx.QueryContext(ctx, sqlutil.Bind(statement), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []analytics.Row{}
	for rows.Next() {
		var row analytics.Row
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

func LoadDashboard(ctx context.Context, store *datastore.Store, q analytics.Query, now time.Time) (analytics.Dashboard, error) {
	return loadDashboard(ctx, store, q, now, 0)
}

// LoadAllDashboard shares paginated analytics but reads all rows in one bounded
// snapshot. The page metadata retains the first page's requested size.
func LoadAllDashboard(ctx context.Context, store *datastore.Store, q analytics.Query, now time.Time, maxRows int) (analytics.Dashboard, error) {
	if maxRows < 1 {
		return analytics.Dashboard{}, fmt.Errorf("invalid analytics row limit")
	}
	q.Page = 1
	return loadDashboard(ctx, store, q, now, maxRows)
}

func loadDashboard(ctx context.Context, store *datastore.Store, q analytics.Query, now time.Time, maxRows int) (analytics.Dashboard, error) {
	result := analytics.Dashboard{Page: q.Page, PageSize: q.PageSize, Quality: q.Quality}
	tx, err := store.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := datastore.ReadMetadataForDataset(ctx, tx, store.DatasetID())
	if err != nil {
		return result, err
	}
	result.DatabaseID = m.DatabaseID
	result.DatasetID = m.DatasetID
	result.Revision = m.Revision
	result.InputRevision = m.InputRevision
	result.Generation = m.Generation
	result.LastSynced = m.LastIngestionAtMs
	if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT COUNT(*) FROM processing_scopes WHERE dataset_id=? AND (processed_revision<>revision OR generation<>?)"), store.DatasetID(), m.TargetGeneration).Scan(&result.Pending); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT COUNT(*) FROM processing_outcomes o WHERE o.dataset_id=? AND o.generation=? AND disposition='ambiguous' AND o.code='unusable_usage'"), store.DatasetID(), m.Generation).Scan(&result.Unresolved); err != nil {
		return result, err
	}
	where, args, err := queryWhere(queryFilter(q, now), reportingZone(now), store.DatasetID())
	if err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT "+sumSQL("total_tokens")+","+sumSQL("input_tokens")+","+sumSQL("output_tokens")+","+sumSQL("reasoning_tokens")+","+sumSQL("cache_read_tokens")+","+sumSQL("cache_write_tokens")+",COUNT(DISTINCT session_id) FROM "+dataTable(q)+where), args...).Scan(&result.Summary.TotalTokens, &result.Summary.InputTokens, &result.Summary.OutputTokens, &result.Summary.ReasoningTokens, &result.Summary.CacheReadTokens, &result.Summary.CacheWriteTokens, &result.Summary.SessionCount); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT COUNT(DISTINCT session_id) FROM "+dataTable(q)+" WHERE countable AND dataset_id=?"), store.DatasetID()).Scan(&result.Summary.SyncedSessions); err != nil {
		return result, err
	}
	if err := safeAggregate(result.Summary.TotalTokens, result.Summary.InputTokens, result.Summary.OutputTokens, result.Summary.ReasoningTokens, result.Summary.CacheReadTokens, result.Summary.CacheWriteTokens, result.Summary.SessionCount, result.Summary.SyncedSessions); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT COUNT(*) FROM "+dataTable(q)+where), args...).Scan(&result.FactCount); err != nil {
		return result, err
	}
	grouped := groupedSQL(q, where, reportingZone(now), store.PostgreSQL())
	groupArgs := append([]interface{}{}, args...)
	if q.Tab == "tokens" {
		groupArgs = append([]interface{}{reportingZone(now)}, args...)
	}
	if err := tx.QueryRowContext(ctx, sqlutil.Bind("SELECT COUNT(*) FROM("+grouped+") grouped"), groupArgs...).Scan(&result.RowCount); err != nil {
		return result, err
	}
	if maxRows > 0 && result.RowCount > maxRows {
		return analytics.Dashboard{}, fmt.Errorf("analytics row limit exceeded (%d)", maxRows)
	}
	pages := max(1, (result.RowCount+q.PageSize-1)/q.PageSize)
	result.Page = min(q.Page, pages)
	limit := q.PageSize
	if maxRows > 0 {
		limit = maxRows
	}
	pageArgs := append(append([]interface{}{}, groupArgs...), limit, (result.Page-1)*q.PageSize)
	result.Rows, err = readRows(ctx, tx, grouped+orderSQL(q, q.Sort, q.Direction)+" LIMIT ? OFFSET ?", pageArgs)
	if err != nil {
		return result, err
	}
	chartQuery := q
	chartArgs := groupArgs
	chartSQL := grouped
	chartSort, chartDirection := "total", "desc"
	limit = analytics.ChartLimit
	switch q.Tab {
	case "tokens", "sessions":
		chartQuery.Tab = "tokens"
		chartSQL = groupedSQL(chartQuery, where, reportingZone(now), store.PostgreSQL())
		chartArgs = append([]interface{}{reportingZone(now)}, args...)
		chartSort, chartDirection = "date", "asc"
		limit = 1000
	case "context":
		chartSort = "averageContext"
	}
	result.Chart, err = readRows(ctx, tx, chartSQL+orderSQL(chartQuery, chartSort, chartDirection)+" LIMIT ?", append(append([]interface{}{}, chartArgs...), limit))
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

func LoadFacets(ctx context.Context, store *datastore.Store, q analytics.Query, search string, now time.Time) (analytics.Facets, error) {
	result := analytics.Facets{Providers: []string{}, Models: []string{}, Harnesses: []string{}, Sessions: []string{}, Repositories: []querymodel.LocationOption{}, Directories: []querymodel.LocationOption{}}
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
		where, args, err := queryWhere(f, reportingZone(now), store.DatasetID())
		if err != nil {
			return result, err
		}
		statement := "SELECT DISTINCT " + field + " FROM " + dataTable(q) + where
		limit := 1000
		if field == "session_native_id" {
			if store.PostgreSQL() {
				statement += " AND position(lower(?) in lower(session_native_id))>0"
			} else {
				statement += " AND instr(lower(session_native_id),lower(?))>0"
			}
			args = append(args, search)
			limit = analytics.SessionOptionLimit
		}
		location := field == "repository_key" || field == "directory_key"
		if location {
			label := strings.Replace(field, "_key", "_name", 1)
			statement = "SELECT COALESCE(NULLIF(" + field + ",''),'unknown'),COALESCE(NULLIF(MIN(" + label + "),''),'unknown') FROM " + dataTable(q) + where + " GROUP BY " + field
		}
		statement += " ORDER BY 1 LIMIT ?"
		args = append(args, limit)
		rows, err := tx.QueryContext(ctx, sqlutil.Bind(statement), args...)
		if err != nil {
			return result, fmt.Errorf("facets %s: %w", field, err)
		}
		for rows.Next() {
			if location {
				var option querymodel.LocationOption
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
				result.Harnesses = append(result.Harnesses, value)
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
	m, err := datastore.ReadMetadataForDataset(ctx, tx, store.DatasetID())
	if err != nil {
		return result, err
	}
	result.Revision = m.Revision
	result.DatabaseID = m.DatabaseID
	result.DatasetID = m.DatasetID
	result.Generation = m.Generation
	result.InputRevision = m.InputRevision
	return result, tx.Commit()
}

// All identifiers here are fixed selections. Zones are bound, including fixed
// offsets, and calendar boundaries retain historical DST rules.
func bucketSQL(zone, bucket string, postgres bool) string {
	if bucket != "week" && bucket != "month" && bucket != "year" {
		bucket = "day"
	}
	if !postgres {
		return "ti_bucket(occurred_at_ms,?,'" + bucket + "')"
	}
	expression := "timezone(?,to_timestamp(occurred_at_ms / 1000.0))"
	if strings.HasPrefix(zone, "@") {
		expression = "timezone(make_interval(secs => CAST(substring(CAST(? AS TEXT) FROM 2) AS DOUBLE PRECISION)),to_timestamp(occurred_at_ms / 1000.0))"
	}
	format := "YYYY-MM-DD"
	if bucket == "month" {
		format = "YYYY-MM"
	}
	if bucket == "year" {
		format = "YYYY"
	}
	return "to_char(date_trunc('" + bucket + "'," + expression + "),'" + format + "')"
}
