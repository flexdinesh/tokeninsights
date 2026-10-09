package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

// Only the embedded reference contract is cached. Every inspected database is
// checked afresh, including constraints and indexes, before writable opening.
var currentSchemaContracts = sync.OnceValues(func() (map[string]string, error) {
	reference, err := connect("", false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reference.Close() }()
	if _, err := reference.ExecContext(context.Background(), Schema); err != nil {
		return nil, err
	}
	return schemaContracts(context.Background(), reference)
})

func schemaContracts(ctx context.Context, database *sql.DB) (map[string]string, error) {
	contracts := map[string]*strings.Builder{}
	contract := func(schema, table string) *strings.Builder {
		key := schema + "." + table
		if contracts[key] == nil {
			contracts[key] = &strings.Builder{}
		}
		return contracts[key]
	}
	columns, err := database.QueryContext(ctx, "SELECT table_schema,table_name,column_name,data_type,is_nullable,column_default FROM information_schema.columns ORDER BY table_schema,table_name,ordinal_position")
	if err != nil {
		return nil, err
	}
	defer func() { _ = columns.Close() }()
	for columns.Next() {
		var schema, table, name, kind, nullable string
		var defaultValue sql.NullString
		if err := columns.Scan(&schema, &table, &name, &kind, &nullable, &defaultValue); err != nil {
			return nil, err
		}
		fmt.Fprintf(contract(schema, table), "%s:%s:%s:%v\n", name, kind, nullable, defaultValue)
	}
	if err := columns.Err(); err != nil {
		return nil, err
	}
	if err := columns.Close(); err != nil {
		return nil, err
	}
	constraints, err := database.QueryContext(ctx, "SELECT schema_name,table_name,constraint_type,constraint_text,CAST(constraint_column_names AS VARCHAR) FROM duckdb_constraints() ORDER BY schema_name,table_name,constraint_type,constraint_text")
	if err != nil {
		return nil, err
	}
	defer func() { _ = constraints.Close() }()
	for constraints.Next() {
		var schema, table, kind, text, columns string
		if err := constraints.Scan(&schema, &table, &kind, &text, &columns); err != nil {
			return nil, err
		}
		fmt.Fprintf(contract(schema, table), "constraint:%s:%s:%s\n", kind, text, columns)
	}
	if err := constraints.Err(); err != nil {
		return nil, err
	}
	if err := constraints.Close(); err != nil {
		return nil, err
	}
	indexes, err := database.QueryContext(ctx, "SELECT schema_name,table_name,index_name,is_unique,expressions FROM duckdb_indexes() ORDER BY schema_name,table_name,index_name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = indexes.Close() }()
	for indexes.Next() {
		var schema, table, name, expressions string
		var unique bool
		if err := indexes.Scan(&schema, &table, &name, &unique, &expressions); err != nil {
			return nil, err
		}
		fmt.Fprintf(contract(schema, table), "index:%s:%v:%s\n", name, unique, expressions)
	}
	if err := indexes.Err(); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(contracts))
	for table, text := range contracts {
		result[table] = text.String()
	}
	return result, nil
}
