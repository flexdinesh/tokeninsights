package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

//go:embed schema/schema.sql
var schemaFS embed.FS

const embeddedSchemaPath = "schema/schema.sql"

const DomainTokenUsage = "token_usage"

func Open(dbPath string) (*sql.DB, error) {
	return openExisting(dbPath, true)
}

func OpenWritable(dbPath string) (*sql.DB, error) {
	return openExisting(dbPath, false)
}

func CreateIfMissing(dbPath string) (*sql.DB, bool, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, false, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, false, err
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, false, err
		}
		db, err := openSQLite(absPath, false)
		if err != nil {
			return nil, false, err
		}
		if err := ApplySchema(context.Background(), db); err != nil {
			_ = db.Close()
			return nil, false, err
		}
		return db, true, nil
	}
	if info.IsDir() {
		return nil, false, fmt.Errorf("db path is a directory: %s", absPath)
	}

	db, err := openExisting(absPath, false)
	if err != nil {
		return nil, false, err
	}
	return db, false, nil
}

func ApplySchema(ctx context.Context, db *sql.DB) error {
	return createSchema(ctx, db)
}

func ResetAll(dbPath string) error {
	ctx := context.Background()
	release, err := AcquireWriterLock(ctx, dbPath)
	if err != nil {
		return err
	}
	defer release()
	absPath, err := canonicalDBPath(dbPath)
	if err != nil {
		return err
	}
	db, err := openSQLite(absPath, false)
	if err != nil {
		return err
	}
	defer db.Close()
	return replaceSchema(ctx, db, "")
}

func ResetCanonical(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	state, err := inspectCompatibility(ctx, tx)
	if err != nil {
		return err
	}
	if err := requireCompatible(state, true); err != nil {
		return err
	}
	statements := []string{
		"DELETE FROM " + TableNormalizationDiagnostics,
		"DELETE FROM " + TableCanonicalTokenUsage,
		"DELETE FROM " + TableCanonicalMessages,
		"DELETE FROM " + TableCanonicalSessions,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO normalization_work_queue (
			raw_fact_id, domain, enqueued_at_ms
		)
		SELECT id, ?, ? FROM raw_token_usage
	`, DomainTokenUsage, time.Now().UnixMilli()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func openExisting(dbPath string, readOnly bool) (*sql.DB, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("db not found: %s (run `tokeninsights sync --all` or `tokeninsights reset-all --confirm`)", absPath)
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("db path is a directory: %s", absPath)
	}

	// Inspect through a read-only connection before opening a writer. Unsupported
	// databases must not receive schema initialization or connection-level writes.
	if !readOnly {
		state, err := inspectPath(context.Background(), absPath, false)
		if err != nil {
			return nil, err
		}
		if err := requireCompatible(state, false); err != nil {
			return nil, err
		}
	}
	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	db, err := openSQLiteMode(context.Background(), absPath, mode)
	if err != nil {
		return nil, err
	}
	state, err := inspectDatabase(context.Background(), db, false)
	if err == nil {
		err = requireCompatible(state, readOnly)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func openSQLite(absPath string, readOnly bool) (*sql.DB, error) {
	mode := "rwc"
	if readOnly {
		mode = "ro"
	}
	return openSQLiteMode(context.Background(), absPath, mode)
}

func openSQLiteMode(ctx context.Context, absPath, mode string) (*sql.DB, error) {
	fileURL := url.URL{Scheme: "file", Path: absPath}
	query := fileURL.Query()
	query.Set("mode", mode)
	if mode == "ro" {
		query.Add("_pragma", "query_only(true)")
	}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(on)")
	fileURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", fileURL.String())
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db ping failed: %w", err)
	}
	return db, nil
}
