package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Connection PRAGMAs cannot run inside a schema transaction: journal_mode and
// foreign_keys in particular must take effect before BEGIN.
func schemaParts() (string, string, error) {
	schema, err := schemaFS.ReadFile(embeddedSchemaPath)
	if err != nil {
		return "", "", fmt.Errorf("read embedded schema: %w", err)
	}
	preamble, body, ok := strings.Cut(string(schema), "CREATE TABLE")
	if !ok {
		return "", "", errors.New("embedded schema has no table definitions")
	}
	return preamble, "CREATE TABLE" + body, nil
}

func createSchema(ctx context.Context, database *sql.DB) error {
	conn, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	empty, err := schemaEmpty(ctx, conn)
	if err != nil {
		return err
	}
	if !empty {
		state, err := inspectCompatibility(ctx, conn)
		if err != nil {
			return err
		}
		return requireCompatible(state, false)
	}
	preamble, body, err := schemaParts()
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, preamble); err != nil {
		return fmt.Errorf("apply schema preamble: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	empty, err = schemaEmpty(ctx, tx)
	if err != nil {
		return err
	}
	if !empty {
		return errors.New("database changed during schema creation; retry sync")
	}
	if err := initializeSchema(ctx, tx, body, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func schemaEmpty(ctx context.Context, reader Reader) (bool, error) {
	var version, objects int
	if err := reader.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return false, err
	}
	if err := reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*'").Scan(&objects); err != nil {
		return false, err
	}
	return version == 0 && objects == 0, nil
}

// replaceSchema is the shared in-place reset. Automatic callers already hold
// the writer lock; ResetAll owns it. Never unlink a database or its WAL/SHM.
func replaceSchema(ctx context.Context, database *sql.DB, sourceKey string) error {
	conn, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if sourceKey != "" {
		state, err := inspectCompatibility(ctx, conn)
		if err != nil {
			return err
		}
		if err := requireRecoverySource(state, sourceKey); err != nil {
			return err
		}
		if !state.ResetRequired {
			return nil
		}
	}
	preamble, body, err := schemaParts()
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, preamble); err != nil {
		return fmt.Errorf("apply reset preamble: %w", err)
	}
	// Dropping all tables makes their old foreign-key graph irrelevant. Disable
	// enforcement only on this pinned connection, outside the transaction.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if sourceKey != "" {
		state, err := inspectCompatibility(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecoverySource(state, sourceKey); err != nil {
			return err
		}
		if !state.ResetRequired {
			return nil
		}
	}
	if err := replaceSchemaTx(ctx, tx, body, sourceKey); err != nil {
		return err
	}
	return tx.Commit()
}

// Kept separate so rollback tests can interrupt after the actual DDL/state
// replacement and before commit, without production fault-injection hooks.
func replaceSchemaTx(ctx context.Context, tx *sql.Tx, body string, sourceKey string) error {
	rows, err := tx.QueryContext(ctx, "SELECT type, name FROM sqlite_schema WHERE type IN ('view', 'trigger', 'table') AND name NOT GLOB 'sqlite_*' ORDER BY CASE type WHEN 'view' THEN 0 WHEN 'trigger' THEN 1 ELSE 2 END")
	if err != nil {
		return err
	}
	var statements []string
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			_ = rows.Close()
			return err
		}
		statements = append(statements, "DROP "+strings.ToUpper(kind)+" IF EXISTS \""+strings.ReplaceAll(name, "\"", "\"\"")+"\"")
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("reset database schema: %w", err)
		}
	}
	return initializeSchema(ctx, tx, body, sourceKey)
}

func initializeSchema(ctx context.Context, tx *sql.Tx, body string, sourceKey string) error {
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO database_lifecycle (id, data_generation, rebuild_pending, rebuild_source_key, updated_at_ms) VALUES (1, ?, ?, NULLIF(?, ''), ?)", CurrentDataGeneration, sourceKey != "", sourceKey, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("initialize database lifecycle: %w", err)
	}
	return nil
}
