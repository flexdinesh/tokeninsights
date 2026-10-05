package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrRecoveryRequired = errors.New("database recovery required: run `tokeninsights sync --all`")
var ErrRebuildPending = errors.New("database rebuild pending: repeat `tokeninsights sync --all` with the original source options and --collector-db-path to resume")

type Compatibility struct {
	Exists           bool
	ResetRequired    bool
	RebuildPending   bool
	RebuildSourceKey string
}

// InspectCompatibility never creates a database or changes its contents.
func InspectCompatibility(ctx context.Context, path string) (Compatibility, error) {
	return inspectPath(ctx, path, true)
}

func inspectPath(ctx context.Context, path string, checkIntegrity bool) (Compatibility, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Compatibility{}, err
	}
	info, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return Compatibility{}, ctx.Err()
	}
	if err != nil {
		return Compatibility{}, err
	}
	if info.IsDir() {
		return Compatibility{}, fmt.Errorf("db path is a directory: %s", abs)
	}
	database, err := openSQLiteMode(ctx, abs, "ro")
	if err != nil {
		return Compatibility{Exists: true}, err
	}
	defer func() { _ = database.Close() }()
	return inspectDatabase(ctx, database, checkIntegrity)
}

func inspectDatabase(ctx context.Context, database *sql.DB, checkIntegrity bool) (Compatibility, error) {
	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Compatibility{Exists: true}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := inspectCompatibility(ctx, tx)
	if err != nil || !checkIntegrity || !state.ResetRequired {
		return state, err
	}
	// Scan before destructive recovery. Compatible refreshes and analytics opens
	// must not scan all stored facts merely to check their compatibility marker.
	var integrity string
	if err := tx.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity); err != nil {
		return state, fmt.Errorf("database integrity check: %w", err)
	}
	if integrity != "ok" {
		return state, errors.New("database integrity check failed; automatic recovery refused")
	}
	return state, nil
}

// BeginAnalyticsRead validates lifecycle state in the same snapshot used by the
// caller's queries. Callers must commit or roll back the returned transaction.
func BeginAnalyticsRead(ctx context.Context, database *sql.DB) (*sql.Tx, error) {
	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	state, err := inspectCompatibility(ctx, tx)
	if err == nil {
		err = requireCompatible(state, true)
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func inspectCompatibility(ctx context.Context, reader Reader) (Compatibility, error) {
	result := Compatibility{Exists: true}
	if err := requireCollectorRole(ctx, reader); err != nil {
		return result, err
	}
	var version int
	if err := reader.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return result, fmt.Errorf("schema version check failed: %w", err)
	}
	if version > SupportedSchemaVersion {
		return result, fmt.Errorf("database schema %d is newer than supported schema %d; upgrade TokenInsights", version, SupportedSchemaVersion)
	}
	if version != SupportedSchemaVersion {
		return result, unrecognizedDatabase(version)
	}
	if err := recognizeSchema(ctx, reader, version); err != nil {
		return result, err
	}
	if err := inspectPublicationState(ctx, reader); err != nil {
		return result, err
	}
	var id, generation, pending int
	var updated int64
	var sourceKey sql.NullString
	var count int
	if err := reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM database_lifecycle").Scan(&count); err != nil {
		return result, err
	}
	if count != 1 {
		return result, errors.New("invalid database lifecycle singleton; automatic recovery refused")
	}
	if err := reader.QueryRowContext(ctx, "SELECT id, data_generation, rebuild_pending, rebuild_source_key, updated_at_ms FROM database_lifecycle").Scan(&id, &generation, &pending, &sourceKey, &updated); err != nil {
		return result, fmt.Errorf("invalid database lifecycle: %w", err)
	}
	if id != 1 || generation < 0 || (pending != 0 && pending != 1) || updated < 0 {
		return result, errors.New("invalid database lifecycle state; automatic recovery refused")
	}
	if (pending == 0 && sourceKey.Valid) || (pending == 1 && (!sourceKey.Valid || sourceKey.String == "")) {
		return result, errors.New("invalid database rebuild source key; automatic recovery refused")
	}
	if generation > CurrentDataGeneration {
		return result, fmt.Errorf("database data generation %d is newer than supported generation %d; upgrade TokenInsights", generation, CurrentDataGeneration)
	}
	result.ResetRequired = generation < CurrentDataGeneration
	result.RebuildPending = pending == 1
	result.RebuildSourceKey = sourceKey.String
	return result, nil
}

func unrecognizedDatabase(version int) error {
	return fmt.Errorf("unsupported or invalid collector schema version %d; use a fresh database with --collector-db-path; existing contents preserved", version)
}

// Require the current collector table family and identifying columns rather
// than trusting user_version. Old architecture schemas are never migrated.
// Unknown tables/views are rejected; user/failure-injection triggers are allowed.
func recognizeSchema(ctx context.Context, reader Reader, version int) error {
	required := map[string]string{
		TableIngestRuns:               "id run_id harness collector parser source_id source_kind status started_at_ms raw_fact_count observation_count canonical_count diagnostic_count hostname",
		TableRawTokenUsage:            "id raw_fact_key harness source_id source_kind collector parser observed_at_ms session_id message_id provider model usage_scope quality input_tokens output_tokens reasoning_tokens cache_read_tokens cache_write_tokens total_tokens location_id location_conflicts",
		TableRawObservations:          "id ingest_run_id raw_fact_id observed_at_ms observation_key",
		TableCanonicalSessions:        "id semantic_key harness session_id first_seen_at_ms last_seen_at_ms primary_raw_fact_id",
		TableCanonicalMessages:        "id semantic_key session_id harness harness_message_id primary_raw_fact_id",
		TableCanonicalTokenUsage:      "id semantic_key recorded_at_ms harness session_id message_id provider provider_source model usage_scope quality is_countable input_tokens output_tokens reasoning_tokens cache_read_tokens cache_write_tokens total_tokens primary_raw_fact_id ingest_run_id location_id",
		TableNormalizationDiagnostics: "id diagnostic_key recorded_at_ms harness raw_fact_id ingest_run_id severity code message",
		TablePublicationState:         "id stream_id identity_version semantics_version source_namespace created_at_ms",
		TablePublicationEntities:      "fact_id payload_hash sequence source_revision_rule source_revision_value",
		TablePublicationJournal:       "sequence fact_id payload_hash payload_json identity_version semantics_version source_revision_rule source_revision_value created_at_ms",
		TablePublicationDestinations:  "destination_id endpoint database_id acknowledged_sequence last_receipt_json acknowledged_at_ms",
		TablePublicationBatches:       "batch_id destination_id stream_id database_id first_sequence last_sequence request_hash request_bytes receipt_bytes acknowledged_at_ms",
		TableNormalizationWorkQueue:   "id raw_fact_id domain enqueued_at_ms",
		TableSourceRefreshState:       "id harness source_kind source_state_key collector parser last_successful_refresh_at_ms source_mtime_ms source_size_bytes updated_at_ms",
		TableSourceCursorState:        "id harness source_kind source_state_key collector parser cursor_kind byte_offset source_mtime_ms source_size_bytes prefix_hash boundary_hash location_fingerprint updated_at_ms",
		TableNormalizationRuleState:   "harness rule_signature updated_at_ms",
		TableSyncState:                "id revision last_successful_sync_at_ms",
		TableSyncJobs:                 "id scope_key status phase started_at_ms completed_at_ms updated_at_ms normalize all_harnesses error_code",
		TableSyncHarnesses:            "job_id harness status discovered total_sources checked_sources failed_sources checked_at_ms",
		TableSyncSources:              "job_id harness source_id source_kind status error_code min_occurred_at_ms max_occurred_at_ms updated_at_ms",
		TableDatabaseLifecycle:        "id data_generation rebuild_pending rebuild_source_key updated_at_ms",
		TableUsageLocations:           "id semantic_key directory_key directory_name repository_key repository_name repository_source",
	}
	rows, err := reader.QueryContext(ctx, "SELECT type, name FROM sqlite_schema WHERE type IN ('table', 'view', 'trigger') AND name NOT GLOB 'sqlite_*'")
	if err != nil {
		return err
	}
	tables := map[string]bool{}
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			_ = rows.Close()
			return err
		}
		if kind == "trigger" {
			continue
		}
		if kind != "table" || required[name] == "" {
			_ = rows.Close()
			return unrecognizedDatabase(version)
		}
		tables[name] = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for table := range required {
		if !tables[table] {
			return unrecognizedDatabase(version)
		}
	}
	for table := range tables {
		columns := required[table]
		// Names and column lists are internal constants, never database content.
		rows, err := reader.QueryContext(ctx, "SELECT "+strings.Join(strings.Fields(columns), ", ")+" FROM "+table+" LIMIT 0")
		if err != nil {
			return unrecognizedDatabase(version)
		}
		_ = rows.Close()
	}
	return nil
}

func requireCompatible(state Compatibility, analytics bool) error {
	if state.ResetRequired {
		return ErrRecoveryRequired
	}
	if analytics && state.RebuildPending {
		return ErrRebuildPending
	}
	return nil
}

// ResetForRecovery requires the caller's writer lock. A compatible or already
// pending database is preserved, so retries cannot discard partial imports.
// sourceKey is a nonempty, metadata-safe fingerprint supplied by the coordinator.
func ResetForRecovery(ctx context.Context, path string, sourceKey string) error {
	if sourceKey == "" {
		return errors.New("database recovery requires a nonempty source key")
	}
	state, err := InspectCompatibility(ctx, path)
	if err != nil {
		return err
	}
	if !state.Exists {
		return errors.New("cannot recover a missing database")
	}
	if err := requireRecoverySource(state, sourceKey); err != nil {
		return err
	}
	if !state.ResetRequired {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	database, err := openSQLiteMode(ctx, abs, "rw")
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	return replaceSchema(ctx, database, sourceKey)
}

func requireRecoverySource(state Compatibility, sourceKey string) error {
	if state.RebuildPending && state.RebuildSourceKey != sourceKey {
		return fmt.Errorf("recovery source scope differs; repeat the original source flags: %w", ErrRebuildPending)
	}
	return nil
}

// CompleteRecovery only publishes analytics once all normalization work is
// finished. Validation and the state change share a transaction.
func CompleteRecovery(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := inspectCompatibility(ctx, tx)
	if err != nil {
		return err
	}
	if err := requireCompatible(state, false); err != nil {
		return err
	}
	var pending bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM normalization_work_queue)").Scan(&pending); err != nil {
		return err
	}
	if pending {
		return fmt.Errorf("normalization work remains: %w", ErrRebuildPending)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE database_lifecycle SET rebuild_pending = 0, rebuild_source_key = NULL, updated_at_ms = ? WHERE id = 1 AND data_generation = ?", time.Now().UnixMilli(), CurrentDataGeneration); err != nil {
		return err
	}
	return tx.Commit()
}

// Role is checked before schema recognition or destructive recovery.
func requireCollectorRole(ctx context.Context, reader Reader) error {
	var role int
	if err := reader.QueryRowContext(ctx, "PRAGMA application_id").Scan(&role); err != nil {
		return err
	}
	if role != CollectorApplicationID {
		return errors.New("not a collector database; use fresh collector.sqlite and server.sqlite files; legacy tokeninsights.sqlite remains unchanged")
	}
	return nil
}

func inspectPublicationState(ctx context.Context, reader Reader) error {
	var count, id, identity, semantics int
	var stream, namespace string
	var created int64
	if err := reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM publication_state").Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return errors.New("invalid publication state singleton; recovery refused")
	}
	if err := reader.QueryRowContext(ctx, "SELECT id,stream_id,identity_version,semantics_version,source_namespace,created_at_ms FROM publication_state").Scan(&id, &stream, &identity, &semantics, &namespace, &created); err != nil {
		return err
	}
	if id != 1 || len(stream) == 0 || len(stream) > 256 || identity != 1 || semantics != 1 || len(namespace) == 0 || len(namespace) > 256 || created < 0 {
		return errors.New("invalid publication state; recovery refused")
	}
	return nil
}
