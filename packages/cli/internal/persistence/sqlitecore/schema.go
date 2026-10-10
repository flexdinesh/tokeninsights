// Package sqlitecore shares physical SQLite checks across independent role stores.
package sqlitecore

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var references sync.Map

func Objects(ctx context.Context, database *sql.DB) (map[string]string, error) {
	rows, err := database.QueryContext(ctx, "SELECT type,name,sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' AND sql IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	objects := map[string]string{}
	for rows.Next() {
		var kind, name, statement string
		if err := rows.Scan(&kind, &name, &statement); err != nil {
			return nil, err
		}
		objects[kind+":"+name] = strings.Join(strings.Fields(statement), " ")
	}
	return objects, rows.Err()
}

// Validate compares actual objects with a reference built from the authoritative
// DDL. Only the reference is cached; a changed file is never trusted from memory.
func Validate(ctx context.Context, database *sql.DB, schema string) error {
	value, _ := references.LoadOrStore(schema, sync.OnceValues(func() (map[string]string, error) {
		reference, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			return nil, err
		}
		defer func() { _ = reference.Close() }()
		reference.SetMaxOpenConns(1)
		if _, err := reference.ExecContext(context.Background(), schema); err != nil {
			return nil, err
		}
		return Objects(context.Background(), reference)
	}))
	expected, err := value.(func() (map[string]string, error))()
	if err != nil {
		return err
	}
	actual, err := Objects(ctx, database)
	if err != nil {
		return err
	}
	if !maps.Equal(actual, expected) {
		return fmt.Errorf("incompatible_sqlite_schema")
	}
	return nil
}
