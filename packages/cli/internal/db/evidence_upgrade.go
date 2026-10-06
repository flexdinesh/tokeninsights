package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// UpgradeEvidence adds only schema-17 outbox state to a verified schema 16.
// The caller must hold the Collector writer lock. No reset/recovery is performed.
func UpgradeEvidence(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	inspection, err := openSQLiteMode(ctx, abs, "ro")
	if err != nil {
		return err
	}
	var version int
	err = inspection.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	if err == nil && version == 16 {
		_, err = inspectCompatibilityVersion(ctx, inspection, 16)
	}
	_ = inspection.Close()
	if err != nil {
		return err
	}
	if version != 16 {
		return nil
	}
	database, err := openSQLiteMode(ctx, abs, "rw")
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 16 {
		return errors.New("collector_changed_during_upgrade")
	}
	if _, err := inspectCompatibilityVersion(ctx, tx, 16); err != nil {
		return err
	}
	_, body, err := schemaParts()
	if err != nil {
		return err
	}
	_, additions, ok := strings.Cut(body, "-- Sanitized raw outbox.")
	if !ok {
		return errors.New("missing_evidence_schema")
	}
	// Preserve the comment's remainder while applying only additive definitions.
	additions = "-- Sanitized raw outbox." + additions
	if _, err := tx.ExecContext(ctx, additions); err != nil {
		return err
	}
	return tx.Commit()
}

// OpenEvidence validates old canonical lifecycle without rebuilding it. Raw
// extraction has its own version/cursor and cannot reset retained publication.
func OpenEvidence(ctx context.Context, path string) (*sql.DB, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		database, _, err := CreateIfMissing(path)
		return database, err
	} else if err != nil {
		return nil, err
	}
	if _, err := InspectCompatibility(ctx, path); err != nil {
		return nil, err
	}
	return openSQLiteMode(ctx, path, "rw")
}

// EvidencePending protects outbox data from Collector reset/recovery.
func EvidencePending(ctx context.Context, reader Reader) (bool, error) {
	var exists bool
	if err := reader.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name='evidence_outbox')").Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	var pending bool
	err := reader.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM evidence_outbox o WHERE NOT EXISTS(SELECT 1 FROM evidence_destinations d) OR EXISTS(SELECT 1 FROM evidence_destinations d WHERE d.acknowledged_sequence < o.sequence))`).Scan(&pending)
	return pending, err
}
