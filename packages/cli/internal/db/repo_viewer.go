package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
)

// RepoGroup selects the location identity used for each row.
type RepoGroup string

const (
	RepoGroupRepository RepoGroup = "repository"
	RepoGroupDirectory  RepoGroup = "directory"
)

// UnknownLocationKey is a filter value; missing values remain NULL in storage.
const UnknownLocationKey = "unknown"

type ViewerRepoGroupRow struct {
	Key                 string
	Name                string
	RepositoryKey       string
	RepositoryName      string
	Models              string
	Providers           string
	Harnesses           string
	DirectoryNames      []string
	HasUnknownDirectory bool
	SessionCount        int64
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CacheReadTokens     int64
	CacheWriteTokens    int64
	ContextUsedTokens   int64
	TotalTokens         int64
	LatestAtMs          int64
}

type LocationOption struct {
	Key  string
	Name string
}

type AvailableLocationValues struct {
	Repositories []LocationOption
	Directories  []LocationOption
}

const unknownDirectoryName = "CASE WHEN COUNT(DISTINCT ul.directory_key) = 1 AND COUNT(ul.directory_key) = COUNT(*) THEN COALESCE('unknown · ' || MIN(ul.directory_name), 'unknown') ELSE 'unknown' END"

func repoGroupExpressions(group RepoGroup) (string, string, error) {
	switch group {
	case RepoGroupRepository:
		return "COALESCE(ul.repository_key, 'unknown')", "CASE WHEN ul.repository_key IS NULL THEN " + unknownDirectoryName + " ELSE COALESCE(MIN(ul.repository_name), 'unknown') END", nil
	case RepoGroupDirectory:
		return "COALESCE(ul.directory_key, 'unknown')", "COALESCE(MIN(ul.directory_name), 'unknown')", nil
	default:
		return "", "", fmt.Errorf("unsupported repo group %q", group)
	}
}

func repoWhereClause(f Filter) (string, []interface{}) {
	where, args := canonicalWhereClause(f)
	for _, facet := range []struct {
		values []string
		expr   string
	}{
		{f.RepositoryKeys, "COALESCE(ul.repository_key, 'unknown')"},
		{f.DirectoryKeys, "COALESCE(ul.directory_key, 'unknown')"},
	} {
		if len(facet.values) == 0 {
			continue
		}
		where += " AND " + facet.expr + " IN (" + placeholders(len(facet.values)) + ")"
		for _, value := range facet.values {
			args = append(args, value)
		}
	}
	return where, args
}

func hasLocationFilters(f Filter) bool {
	return len(f.RepositoryKeys) > 0 || len(f.DirectoryKeys) > 0
}

func facetWhereClause(f Filter) (string, string, []interface{}) {
	if hasLocationFilters(f) {
		where, args := repoWhereClause(f)
		return "LEFT JOIN usage_locations ul ON ul.id = ctu.location_id", where, args
	}
	where, args := canonicalWhereClause(f)
	return "", where, args
}

func ViewerRepoGroups(ctx context.Context, reader Reader, f Filter, group RepoGroup) ([]ViewerRepoGroupRow, error) {
	groupKey, groupName, err := repoGroupExpressions(group)
	if err != nil {
		return nil, err
	}
	where, args := repoWhereClause(f)
	query := `SELECT ` + groupKey + `, ` + groupName + `,
		CASE WHEN COUNT(DISTINCT COALESCE(ul.repository_key, 'unknown')) = 1
			THEN COALESCE(MIN(ul.repository_key), 'unknown') ELSE 'unknown' END,
		CASE WHEN COUNT(DISTINCT COALESCE(ul.repository_key, 'unknown')) = 1
			THEN COALESCE(MIN(ul.repository_name), 'unknown') ELSE 'unknown' END,
		GROUP_CONCAT(DISTINCT ctu.model), GROUP_CONCAT(DISTINCT ctu.provider), GROUP_CONCAT(DISTINCT ctu.harness),
		COALESCE(json_group_array(DISTINCT ul.directory_name) FILTER (
			WHERE ul.directory_key IS NOT NULL AND ul.directory_name IS NOT NULL AND ul.directory_name != ''), '[]'),
		MAX(CASE WHEN ul.directory_key IS NULL OR ul.directory_name IS NULL OR ul.directory_name = '' THEN 1 ELSE 0 END),
		COUNT(DISTINCT ctu.session_id),
		COALESCE(SUM(ctu.input_tokens), 0), COALESCE(SUM(ctu.output_tokens), 0),
		COALESCE(SUM(ctu.reasoning_tokens), 0), COALESCE(SUM(ctu.cache_read_tokens), 0),
		COALESCE(SUM(ctu.cache_write_tokens), 0), COALESCE(MAX(` + contextUsedExpression("ctu") + `), 0),
		COALESCE(SUM(ctu.total_tokens), 0), MAX(ctu.recorded_at_ms)
		FROM canonical_token_usage ctu
		JOIN canonical_sessions cs ON cs.id = ctu.session_id
		LEFT JOIN usage_locations ul ON ul.id = ctu.location_id
		` + where + `
		GROUP BY ` + groupKey + `
		ORDER BY SUM(ctu.total_tokens) DESC, ` + groupKey
	rows, err := reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []ViewerRepoGroupRow{}
	for rows.Next() {
		var row ViewerRepoGroupRow
		var models, providers, harnesses sql.NullString
		var directoryNamesJSON string
		if err := rows.Scan(&row.Key, &row.Name, &row.RepositoryKey, &row.RepositoryName,
			&models, &providers, &harnesses,
			&directoryNamesJSON, &row.HasUnknownDirectory,
			&row.SessionCount, &row.InputTokens, &row.OutputTokens, &row.ReasoningTokens,
			&row.CacheReadTokens, &row.CacheWriteTokens, &row.ContextUsedTokens,
			&row.TotalTokens, &row.LatestAtMs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(directoryNamesJSON), &row.DirectoryNames); err != nil {
			return nil, fmt.Errorf("decode repo directories: %w", err)
		}
		sort.Strings(row.DirectoryNames)
		row.Models = summaryValues(models.String)
		row.Providers = summaryValues(providers.String)
		row.Harnesses = summaryValues(harnesses.String)
		result = append(result, row)
	}
	return result, rows.Err()
}

func ViewerRepoSessionCount(ctx context.Context, reader Reader, f Filter) (int64, error) {
	where, args := repoWhereClause(f)
	var count int64
	err := reader.QueryRowContext(ctx, `SELECT COUNT(DISTINCT ctu.session_id)
		FROM canonical_token_usage ctu
		JOIN canonical_sessions cs ON cs.id = ctu.session_id
		LEFT JOIN usage_locations ul ON ul.id = ctu.location_id `+where, args...).Scan(&count)
	return count, err
}

func AvailableLocations(ctx context.Context, reader Reader, f Filter) (AvailableLocationValues, error) {
	var result AvailableLocationValues
	for _, facet := range []struct {
		values *[]LocationOption
		clear  func(*Filter)
		key    string
		name   string
	}{
		{&result.Repositories, func(f *Filter) { f.RepositoryKeys = nil }, "COALESCE(ul.repository_key, 'unknown')", "CASE WHEN ul.repository_key IS NULL THEN " + unknownDirectoryName + " ELSE COALESCE(MIN(ul.repository_name), 'unknown') END"},
		{&result.Directories, func(f *Filter) { f.DirectoryKeys = nil }, "COALESCE(ul.directory_key, 'unknown')", "COALESCE(MIN(ul.directory_name), 'unknown')"},
	} {
		scope := f
		facet.clear(&scope)
		where, args := repoWhereClause(scope)
		query := `SELECT ` + facet.key + `, ` + facet.name + `
			FROM canonical_token_usage ctu
			JOIN canonical_sessions cs ON cs.id = ctu.session_id
			LEFT JOIN usage_locations ul ON ul.id = ctu.location_id
			` + where + ` GROUP BY ` + facet.key + ` ORDER BY 2, 1`
		rows, err := reader.QueryContext(ctx, query, args...)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var option LocationOption
			if err := rows.Scan(&option.Key, &option.Name); err != nil {
				_ = rows.Close()
				return result, err
			}
			*facet.values = append(*facet.values, option)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func LocationDisplayName(option LocationOption) string {
	if option.Name == "" {
		return "unknown"
	}
	return option.Name
}
