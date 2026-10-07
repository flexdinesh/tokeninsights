package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// UpgradeEvidence additively upgrades verified schemas 16/17 to schema 18.
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
	if err == nil && (version == 16 || version == 17) {
		_, err = inspectCompatibilityVersion(ctx, inspection, version)
	}
	_ = inspection.Close()
	if err != nil {
		return err
	}
	if version != 16 && version != 17 {
		return nil
	}
	previous := version
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
	if version != previous {
		return errors.New("collector_changed_during_upgrade")
	}
	if _, err := inspectCompatibilityVersion(ctx, tx, previous); err != nil {
		return err
	}
	if previous == 16 {
		_, body, err := schemaParts()
		if err != nil {
			return err
		}
		_, additions, ok := strings.Cut(body, "-- Sanitized raw outbox.")
		if !ok {
			return errors.New("missing_evidence_schema")
		}
		if _, err := tx.ExecContext(ctx, "-- Sanitized raw outbox."+additions); err != nil {
			return err
		}
	} else {
		// SQLite supplies defaults for old rows without rewriting immutable request bytes.
		for _, statement := range []string{
			"ALTER TABLE evidence_destinations ADD COLUMN dataset_id TEXT NOT NULL DEFAULT 'default' CHECK (length(dataset_id) BETWEEN 1 AND 256)",
			"ALTER TABLE evidence_batches ADD COLUMN dataset_id TEXT NOT NULL DEFAULT 'default' CHECK (length(dataset_id) BETWEEN 1 AND 256)",
			"ALTER TABLE evidence_batches ADD COLUMN protocol_version INTEGER NOT NULL DEFAULT 2 CHECK (protocol_version IN (2, 3))",
			"DROP TRIGGER evidence_batches_immutable",
			"CREATE TRIGGER evidence_batches_immutable BEFORE UPDATE OF batch_id,destination_id,stream_id,database_id,dataset_id,protocol_version,first_sequence,last_sequence,request_hash,request_bytes ON evidence_batches BEGIN SELECT RAISE(ABORT, 'evidence request is immutable'); END",
			"PRAGMA user_version = 18",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
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

// InspectEvidenceCompatibility recognizes additive predecessors without writing.
// Maintenance previews must never upgrade user storage merely to inspect it.
func InspectEvidenceCompatibility(ctx context.Context, path string) (Compatibility, error) {
	return inspectPathWith(ctx, path, true, inspectEvidenceCompatibility)
}
func inspectEvidenceCompatibility(ctx context.Context, reader Reader) (Compatibility, error) {
	var version int
	if err := reader.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return Compatibility{Exists: true}, err
	}
	if version == 16 || version == 17 {
		return inspectCompatibilityVersion(ctx, reader, version)
	}
	return inspectCompatibility(ctx, reader)
}

// OpenEvidencePreview reads verified compatible schemas, keeping the old file
// and immutable outbox untouched. Ordinary production writers remain strict.
func OpenEvidencePreview(ctx context.Context, path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	database, err := openSQLiteMode(ctx, abs, "ro")
	if err != nil {
		return nil, err
	}
	state, err := inspectDatabaseWith(ctx, database, true, inspectEvidenceCompatibility)
	if err == nil {
		err = requireCompatible(state, true)
	}
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}
