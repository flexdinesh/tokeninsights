// Package sqlitecore shares physical SQLite checks across independent role stores.
package sqlitecore

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecompat"
	_ "modernc.org/sqlite"
)

var references sync.Map

// ErrSchemaMismatch reports objects that differ from the authoritative DDL.
var ErrSchemaMismatch = errors.New("incompatible_sqlite_schema")

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
		return ErrSchemaMismatch
	}
	return nil
}

// Inspect classifies an existing file read-only; it never creates or changes it.
func Inspect(ctx context.Context, path string, applicationID, version int, schema string) (storagecompat.Result, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return storagecompat.Result{}, err
	}
	info, err := os.Stat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return storagecompat.Result{Status: storagecompat.Missing, Supported: version}, nil
	}
	if err != nil {
		return storagecompat.Result{}, err
	}
	if !info.Mode().IsRegular() {
		return storagecompat.Result{}, errors.New("database must be a regular file")
	}
	target := url.URL{Scheme: "file", Path: absolute, RawQuery: url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(5000)"}}.Encode()}
	database, err := sql.Open("sqlite", target.String())
	if err != nil {
		return storagecompat.Result{}, err
	}
	defer func() { _ = database.Close() }()
	database.SetMaxOpenConns(1)
	var role, found int
	if err := database.QueryRowContext(ctx, "PRAGMA application_id").Scan(&role); err != nil {
		return storagecompat.Result{}, err
	}
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&found); err != nil {
		return storagecompat.Result{}, err
	}
	matches := true
	if role == applicationID && found == version {
		err := Validate(ctx, database, schema)
		if err != nil && !errors.Is(err, ErrSchemaMismatch) {
			return storagecompat.Result{}, err
		}
		matches = err == nil
	}
	return storagecompat.Classify(role, found, applicationID, version, matches), nil
}
