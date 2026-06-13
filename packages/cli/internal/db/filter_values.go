package db

import (
	"context"
	"database/sql"
	"sort"
)

func AvailableProviders(ctx context.Context, db *sql.DB, f Filter) ([]string, error) {
	providerFilter := f
	providerFilter.Providers = nil
	whereClause, args := canonicalWhereClause(providerFilter)

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT ctu.provider
		FROM canonical_token_usage ctu
		INNER JOIN canonical_sessions cs ON cs.id = ctu.session_id
		`+whereClause+`
		ORDER BY ctu.provider
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if value != "" {
			values = append(values, value)
		}
	}
	return values, rows.Err()
}

func AvailableModels(ctx context.Context, db *sql.DB, f Filter) ([]string, error) {
	modelFilter := f
	modelFilter.Models = nil
	whereClause, args := canonicalWhereClause(modelFilter)

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT ctu.model
		FROM canonical_token_usage ctu
		INNER JOIN canonical_sessions cs ON cs.id = ctu.session_id
		`+whereClause+`
		ORDER BY ctu.model
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if value != "" {
			values = append(values, value)
		}
	}
	return values, rows.Err()
}

func AvailableHarnesses(ctx context.Context, db *sql.DB, f Filter) ([]string, error) {
	harnessFilter := f
	harnessFilter.Harnesses = nil
	whereClause, args := canonicalWhereClause(harnessFilter)

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT ctu.harness
		FROM canonical_token_usage ctu
		INNER JOIN canonical_sessions cs ON cs.id = ctu.session_id
		`+whereClause+`
		ORDER BY ctu.harness
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if value != "" {
			values = append(values, value)
		}
	}
	return values, rows.Err()
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
