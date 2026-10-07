package datastore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed schema/data_v1.sql
var schemaV1 string

var datasetTables = []string{
	"raw.evidence", "ingestion.batches", "ingestion.items", "ingestion.batch_items", "ingestion.legacy_receipts",
	"processing.scopes", "processing.dependencies", "processing.outcomes", "analytics.generations", "analytics.facts",
	"analytics.estimates", "analytics.provenance", "analytics.legacy", "analytics.legacy_coverage",
}

func inspectCurrent(ctx context.Context, database *sql.DB, expectedKind string) error {
	var role, id, kind string
	var version int
	if err := database.QueryRowContext(ctx, "SELECT role,schema_version,database_id,server_kind FROM ingestion.instance WHERE id=1").Scan(&role, &version, &id, &kind); err != nil {
		return err
	}
	if role != "server-data" || version != SchemaVersion || id == "" || (kind != KindPersonal && kind != KindHosted) {
		return errors.New("incompatible_server_data")
	}
	reference, err := connect("", false)
	if err != nil {
		return err
	}
	defer func() { _ = reference.Close() }()
	if _, err := reference.ExecContext(ctx, Schema); err != nil {
		return err
	}
	tables := append([]string{"ingestion.instance", "ingestion.metadata", "accounts.users", "accounts.tokens", "accounts.sessions"}, datasetTables...)
	for _, table := range tables {
		actual, err := columnContract(ctx, database, table)
		if err != nil {
			return err
		}
		expected, err := columnContract(ctx, reference, table)
		if err != nil {
			return err
		}
		if actual != expected {
			return errors.New("incompatible_server_data")
		}
	}
	if expectedKind != "" && kind != expectedKind {
		return errors.New("server_kind_mismatch")
	}
	rows, err := database.QueryContext(ctx, "SELECT dataset_id FROM ingestion.metadata")
	if err != nil {
		return err
	}
	var datasets []string
	for rows.Next() {
		var dataset string
		if err := rows.Scan(&dataset); err != nil {
			_ = rows.Close()
			return err
		}
		datasets = append(datasets, dataset)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if kind == KindPersonal && (len(datasets) != 1 || datasets[0] != DatasetID) {
		return errors.New("incompatible_server_data")
	}
	for _, dataset := range datasets {
		m, err := ReadMetadataForDataset(ctx, database, dataset)
		if err != nil {
			return err
		}
		if m.DatabaseID != id || m.Kind != kind {
			return errors.New("incompatible_server_data")
		}
	}
	return nil
}

func inspectAndUpgrade(ctx context.Context, path, kind string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	var header [16]byte
	_, readErr := file.ReadAt(header[:], 0)
	_ = file.Close()
	if readErr == nil && string(header[:]) == "SQLite format 3\x00" {
		return Inspect(ctx, path)
	}
	database, err := connect(path, true)
	if err != nil {
		return err
	}
	var version int
	// The metadata table exists in both supported generations, including empty hosted stores.
	err = database.QueryRowContext(ctx, "SELECT schema_version FROM ingestion.instance WHERE id=1").Scan(&version)
	if err != nil {
		err = database.QueryRowContext(ctx, "SELECT schema_version FROM ingestion.metadata WHERE id=1").Scan(&version)
	}
	if err != nil {
		_ = database.Close()
		return err
	}
	if version == SchemaVersion {
		err = inspectCurrent(ctx, database, kind)
		_ = database.Close()
		if err != nil {
			return err
		}
		return Inspect(ctx, path)
	}
	if version != 1 {
		_ = database.Close()
		return errors.New("incompatible_server_data")
	}
	if kind != KindPersonal {
		_ = database.Close()
		return errors.New("server_kind_mismatch")
	}
	err = verifyV1(ctx, database)
	_ = database.Close()
	if err != nil {
		return err
	}
	return upgradeV1(ctx, path)
}

func verifyV1(ctx context.Context, database *sql.DB) error {
	var role, id, dataset string
	var version int
	var generation, target, input, revision int64
	if err := database.QueryRowContext(ctx, "SELECT role,schema_version,database_id,dataset_id,generation,target_generation,input_revision,revision FROM ingestion.metadata WHERE id=1").Scan(&role, &version, &id, &dataset, &generation, &target, &input, &revision); err != nil {
		return err
	}
	if role != "server-data" || version != 1 || id == "" || dataset != DatasetID || generation < 1 || target < generation || target > publication.SafeInteger || input < 0 || input > publication.SafeInteger || revision < 0 || revision > publication.SafeInteger {
		return errors.New("incompatible_server_data")
	}
	var activeState, targetState string
	var activeCount int64
	if err := database.QueryRowContext(ctx, "SELECT active.state,target.state,(SELECT COUNT(*) FROM analytics.generations WHERE state='active') FROM analytics.generations active,analytics.generations target WHERE active.generation=? AND target.generation=?", generation, target).Scan(&activeState, &targetState, &activeCount); err != nil {
		return err
	}
	if !validGenerationStates(activeState, targetState, activeCount, generation, target) {
		return errors.New("incompatible_server_data")
	}
	var processor int
	if err := database.QueryRowContext(ctx, "SELECT MAX(processor_version) FROM analytics.generations").Scan(&processor); err != nil {
		return err
	}
	if processor > evidence.ProcessorVersion {
		return errors.New("newer_processor_version")
	}
	// Compare the complete table contract to a canonical fresh schema, rejecting additions too.
	reference, err := connect("", false)
	if err != nil {
		return err
	}
	defer func() { _ = reference.Close() }()
	if _, err := reference.ExecContext(ctx, schemaV1); err != nil {
		return err
	}
	for _, table := range append([]string{"ingestion.metadata"}, datasetTables...) {
		actual, err := columnContract(ctx, database, table)
		if err != nil {
			return err
		}
		expected, err := columnContract(ctx, reference, table)
		if err != nil {
			return err
		}
		if actual != expected {
			return errors.New("incompatible_server_data")
		}
	}
	return nil
}

func columnContract(ctx context.Context, database *sql.DB, table string) (string, error) {
	parts := strings.Split(table, ".")
	rows, err := database.QueryContext(ctx, "SELECT column_name,data_type,is_nullable,column_default FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position", parts[0], parts[1])
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var result strings.Builder
	for rows.Next() {
		var name, kind, nullable string
		var defaultValue sql.NullString
		if err := rows.Scan(&name, &kind, &nullable, &defaultValue); err != nil {
			return "", err
		}
		fmt.Fprintf(&result, "%s:%s:%s:%v\n", name, kind, nullable, defaultValue)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	constraints, err := database.QueryContext(ctx, "SELECT constraint_type,constraint_text,CAST(constraint_column_names AS VARCHAR) FROM duckdb_constraints() WHERE schema_name=? AND table_name=? ORDER BY constraint_type,constraint_text", parts[0], parts[1])
	if err != nil {
		return "", err
	}
	defer func() { _ = constraints.Close() }()
	for constraints.Next() {
		var kind, text, columns string
		if err := constraints.Scan(&kind, &text, &columns); err != nil {
			return "", err
		}
		fmt.Fprintf(&result, "constraint:%s:%s:%s\n", kind, text, columns)
	}
	if err := constraints.Err(); err != nil {
		return "", err
	}
	if err := constraints.Close(); err != nil {
		return "", err
	}
	indexes, err := database.QueryContext(ctx, "SELECT index_name,is_unique,expressions FROM duckdb_indexes() WHERE schema_name=? AND table_name=? ORDER BY index_name", parts[0], parts[1])
	if err != nil {
		return "", err
	}
	defer func() { _ = indexes.Close() }()
	for indexes.Next() {
		var name, expressions string
		var unique bool
		if err := indexes.Scan(&name, &unique, &expressions); err != nil {
			return "", err
		}
		fmt.Fprintf(&result, "index:%s:%v:%s\n", name, unique, expressions)
	}
	return result.String(), indexes.Err()
}

func upgradeV1(ctx context.Context, path string) error {
	// A stopped owner checkpoints on close. Refuse a live/uncheckpointed source rather than
	// publishing a new database beside a WAL whose committed contents may change.
	if info, err := os.Stat(path + ".wal"); err == nil && info.Size() != 0 {
		return errors.New("schema_upgrade_requires_checkpoint")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sourceReader, err := connect(path, true)
	if err != nil {
		return err
	}
	defer func() { _ = sourceReader.Close() }()
	// Keep a read-only DuckDB lock on the original throughout verification and
	// publication, so another DuckDB writer cannot alter the retained source.
	before, err := fileDigest(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".tokeninsights-upgrade-*.duckdb")
	if err != nil {
		return err
	}
	temporary := file.Name()
	_ = file.Close()
	_ = os.Remove(temporary)
	defer func() { _ = os.Remove(temporary); _ = os.Remove(temporary + ".wal") }()
	database, err := connect(temporary, false)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	// Path is a trusted filesystem path, still quote SQL string literals explicitly.
	source := "'" + strings.ReplaceAll(path, "'", "''") + "'"
	if _, err := database.ExecContext(ctx, "ATTACH "+source+" AS previous (READ_ONLY)"); err != nil {
		return err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, Schema); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.instance SELECT id,role,2,database_id,'personal',created_at_ms FROM previous.ingestion.metadata"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.metadata SELECT dataset_id,id,role,2,database_id,'personal',generation,target_generation,input_revision,revision,last_ingestion_at_ms,created_at_ms FROM previous.ingestion.metadata"); err != nil {
		return err
	}
	for _, table := range datasetTables {
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" SELECT 'default',* FROM previous."+table); err != nil {
			return err
		}
		// EXCEPT ALL compares every value, including immutable BLOB/receipt bytes and all components.
		var differences int64
		query := "SELECT COUNT(*) FROM ((SELECT * EXCLUDE(dataset_id) FROM " + table + " EXCEPT ALL SELECT * FROM previous." + table + ") UNION ALL (SELECT * FROM previous." + table + " EXCEPT ALL SELECT * EXCLUDE(dataset_id) FROM " + table + "))"
		if err := tx.QueryRowContext(ctx, query).Scan(&differences); err != nil {
			return err
		}
		if differences != 0 {
			return errors.New("schema_upgrade_verification_failed")
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, "DETACH previous; CHECKPOINT"); err != nil {
		return err
	}
	if err := database.Close(); err != nil {
		return err
	}
	if err := Inspect(ctx, temporary); err != nil {
		return err
	}
	after, err := fileDigest(path)
	if err != nil {
		return err
	}
	if before != after {
		return errors.New("schema_upgrade_source_changed")
	}
	// Retain the verified original before atomic replacement. A crash leaves a complete
	// schema1 target or a complete schema2 target; neither requires guessing a partial file.
	backup := path + ".schema1-backup"
	if err := os.Link(path, backup); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		digest, readErr := fileDigest(backup)
		if readErr != nil {
			return readErr
		}
		if digest != before {
			return errors.New("schema_upgrade_backup_conflict")
		}
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func fileDigest(path string) ([32]byte, error) {
	var result [32]byte
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return result, err
	}
	copy(result[:], digest.Sum(nil))
	return result, nil
}
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}
