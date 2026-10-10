// Package db owns collector SQLite lifecycle and the cross-process writer lock.
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlitecore"
	_ "modernc.org/sqlite"
)

//go:embed schema/schema.sql
var schema string

func openSQLiteMode(ctx context.Context, path, mode string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	target := url.URL{Scheme: "file", Path: absolute}
	query := target.Query()
	query.Set("mode", mode)
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(on)")
	if mode == "ro" {
		query.Add("_pragma", "query_only(true)")
	}
	target.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", target.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func validate(ctx context.Context, database *sql.DB) error {
	var role, version int
	if err := database.QueryRowContext(ctx, "PRAGMA application_id").Scan(&role); err != nil {
		return err
	}
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if role != CollectorApplicationID || version != SupportedSchemaVersion {
		return fmt.Errorf("incompatible collector database: role=%d schema=%d (expected %d)", role, version, SupportedSchemaVersion)
	}
	return sqlitecore.Validate(ctx, database, schema)
}

func Open(path string) (*sql.DB, error) {
	database, err := openSQLiteMode(context.Background(), path, "ro")
	if err != nil {
		return nil, err
	}
	if err := validate(context.Background(), database); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func OpenWritable(path string) (*sql.DB, error) {
	reader, err := Open(path)
	if err != nil {
		return nil, err
	}
	if err := reader.Close(); err != nil {
		return nil, err
	}
	return openSQLiteMode(context.Background(), path, "rw")
}

func OpenEvidence(ctx context.Context, path string) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	database, _, err := CreateIfMissing(path)
	return database, err
}

// CreateIfMissing publishes only a fully initialized database, without replacing
// an existing inode. Existing contracts are inspected read-only before writing.
func CreateIfMissing(path string) (*sql.DB, bool, error) {
	if _, err := os.Stat(path); err == nil {
		database, err := OpenWritable(path)
		return database, false, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, false, err
	}
	directory := filepath.Dir(absolute)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, false, err
	}
	file, err := os.CreateTemp(directory, ".collector-*")
	if err != nil {
		return nil, false, err
	}
	candidate := file.Name()
	if err := file.Close(); err != nil {
		return nil, false, err
	}
	defer func() { _ = os.Remove(candidate); _ = os.Remove(candidate + "-wal"); _ = os.Remove(candidate + "-shm") }()
	database, err := openSQLiteMode(context.Background(), candidate, "rw")
	if err != nil {
		return nil, false, err
	}
	_, initializeErr := database.Exec(schema)
	if initializeErr == nil {
		_, initializeErr = database.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}
	closeErr := database.Close()
	if err := errors.Join(initializeErr, closeErr); err != nil {
		return nil, false, err
	}
	if err := os.Link(candidate, absolute); err != nil {
		if errors.Is(err, os.ErrExist) {
			database, err := OpenWritable(absolute)
			return database, false, err
		}
		return nil, false, err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return nil, true, err
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return nil, true, err
	}
	database, err = OpenWritable(absolute)
	return database, true, err
}

// ResetAllLocked is used by the controlled development fixture command.
// The caller owns the writer lock; unpublished evidence is never discarded.
func ResetAllLocked(ctx context.Context, path string) error {
	database, _, err := CreateIfMissing(path)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var pending bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM evidence_outbox WHERE
 NOT EXISTS(SELECT 1 FROM evidence_destinations) OR sequence >
 (SELECT MIN(acknowledged_sequence) FROM evidence_destinations))`).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return errors.New("collector has unpublished evidence")
	}
	if _, err := tx.ExecContext(ctx, "DROP TRIGGER evidence_outbox_immutable_delete"); err != nil {
		return err
	}
	for _, table := range []string{"evidence_batches", "evidence_destinations", "evidence_outbox", "evidence_sources", "evidence_quarantine", "evidence_state", "sqlite_sequence"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "CREATE TRIGGER evidence_outbox_immutable_delete BEFORE DELETE ON evidence_outbox BEGIN SELECT RAISE(ABORT, 'evidence is immutable'); END"); err != nil {
		return err
	}
	return tx.Commit()
}
