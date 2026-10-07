package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

func (s *Store) importDefaultLegacy(ctx context.Context, target string) error {
	if filepath.Base(target) != "server.duckdb" {
		return nil
	}
	path := filepath.Join(filepath.Dir(target), "server.sqlite")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return s.ImportLegacy(ctx, path)
}

// ImportLegacy reads SQLite without mutation. Import must precede raw ingestion.
// Baselines stay queryable until an identical contribution ID has proven coverage.
func (s *Store) ImportLegacy(ctx context.Context, path string) error {
	if s.kind != KindPersonal {
		return errors.New("hosted_legacy_import_unsupported")
	}
	legacy, err := serverstore.OpenReadOnly(path)
	if err != nil {
		return err
	}
	defer func() { _ = legacy.Close() }()
	read, err := legacy.BeginRead(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = read.Rollback() }()
	metadata, err := serverstore.ReadMetadata(ctx, read)
	if err != nil {
		return err
	}
	s.writer.Lock()
	defer s.writer.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var existing int64
	if err := tx.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM analytics.legacy WHERE dataset_id=?)+(SELECT COUNT(*) FROM raw.evidence WHERE dataset_id=?)+(SELECT COUNT(*) FROM ingestion.legacy_receipts WHERE dataset_id=?)", s.datasetID, s.datasetID, s.datasetID).Scan(&existing); err != nil {
		return err
	}
	if existing != 0 {
		return errors.New("legacy_import_requires_empty_data_store")
	}
	rows, err := read.QueryContext(ctx, `SELECT u.semantic_key,u.harness,s.semantic_key,s.session_id,s.first_seen_at_ms,s.last_seen_at_ms,m.semantic_key,m.harness_message_id,m.occurred_at_ms,u.recorded_at_ms,u.provider,u.provider_source,u.model,u.usage_scope,u.quality,u.is_countable,u.input_tokens,u.output_tokens,u.reasoning_tokens,u.cache_read_tokens,u.cache_write_tokens,u.total_tokens,COALESCE(l.semantic_key,''),COALESCE(l.directory_key,''),COALESCE(l.directory_name,''),COALESCE(l.repository_key,''),COALESCE(l.repository_name,''),COALESCE(l.repository_source,''),u.revision_rule,u.revision_value FROM canonical_token_usage u JOIN canonical_sessions s ON s.id=u.session_id LEFT JOIN canonical_messages m ON m.id=u.message_id LEFT JOIN usage_locations l ON l.id=u.location_id ORDER BY u.semantic_key`)
	if err != nil {
		return err
	}
	var count int64
	var totals [6]int64
	for rows.Next() {
		var fact publication.Fact
		var messageID, native sql.NullString
		var messageTime sql.NullInt64
		location := publication.Location{}
		var revisionRule string
		var revisionValue int64
		if err := rows.Scan(&fact.ID, &fact.Harness, &fact.Session.ID, &fact.Session.NativeID, &fact.Session.FirstOccurredAtMs, &fact.Session.LastOccurredAtMs, &messageID, &native, &messageTime, &fact.OccurredAtMs, &fact.Provider, &fact.ProviderSource, &fact.Model, &fact.UsageScope, &fact.Quality, &fact.Countable, &fact.InputTokens, &fact.OutputTokens, &fact.ReasoningTokens, &fact.CacheReadTokens, &fact.CacheWriteTokens, &fact.TotalTokens, &location.ID, &location.DirectoryKey, &location.DirectoryName, &location.RepositoryKey, &location.RepositoryName, &location.RepositorySource, &revisionRule, &revisionValue); err != nil {
			_ = rows.Close()
			return err
		}
		fact.Session.Harness = fact.Harness
		if revisionRule != "" {
			fact.Revision = &publication.SourceRevision{Rule: revisionRule, Value: revisionValue}
		}
		if messageID.Valid {
			fact.Message = &publication.Message{ID: messageID.String, NativeID: native.String, OccurredAtMs: messageTime.Int64}
		}
		if location.ID != "" {
			fact.Location = &location
		}
		if err := insertFact(ctx, tx, s.datasetID, "analytics.legacy", fact, 0, metadata.Revision, "", ""); err != nil {
			_ = rows.Close()
			return err
		}
		values := [6]int64{fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens, fact.TotalTokens}
		for i, value := range values {
			if value < 0 || totals[i] > publication.SafeInteger-value {
				_ = rows.Close()
				return errors.New("legacy_aggregate_limit")
			}
			totals[i] += value
		}
		count++
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	var copied int64
	var actual [6]int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*),CAST(COALESCE(SUM(input_tokens),0) AS BIGINT),CAST(COALESCE(SUM(output_tokens),0) AS BIGINT),CAST(COALESCE(SUM(reasoning_tokens),0) AS BIGINT),CAST(COALESCE(SUM(cache_read_tokens),0) AS BIGINT),CAST(COALESCE(SUM(cache_write_tokens),0) AS BIGINT),CAST(COALESCE(SUM(total_tokens),0) AS BIGINT) FROM analytics.legacy WHERE dataset_id=?", s.datasetID).Scan(&copied, &actual[0], &actual[1], &actual[2], &actual[3], &actual[4], &actual[5]); err != nil {
		return err
	}
	if copied != count || actual != totals {
		return errors.New("legacy_verification_failed")
	}
	rows, err = read.QueryContext(ctx, "SELECT stream_id,batch_id,request_hash,receipt_json FROM ingestion_receipts")
	if err != nil {
		return err
	}
	for rows.Next() {
		var stream, batch, hash, body string
		if err := rows.Scan(&stream, &batch, &hash, &body); err != nil {
			_ = rows.Close()
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.legacy_receipts VALUES(?,?,?,?,?)", s.datasetID, stream, batch, hash, body); err != nil {
			_ = rows.Close()
			return err
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET database_id=?,revision=?,last_ingestion_at_ms=?,created_at_ms=? WHERE dataset_id=?", metadata.DatabaseID, metadata.Revision, metadata.LastIngestionAtMs, metadata.CreatedAtMs, s.datasetID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ingestion.instance SET database_id=?,created_at_ms=? WHERE id=1", metadata.DatabaseID, metadata.CreatedAtMs); err != nil {
		return err
	}
	return tx.Commit()
}

func proveLegacyCoverage(ctx context.Context, tx *sql.Tx, datasetID string, fact publication.Fact, generation int64) error {
	var body string
	err := tx.QueryRowContext(ctx, "SELECT payload_json FROM analytics.legacy WHERE dataset_id=? AND fact_id=?", datasetID, fact.ID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var legacy publication.Fact
	if err := json.Unmarshal([]byte(body), &legacy); err != nil {
		return err
	}
	proven := legacyComponentsEqual(legacy, fact)
	if legacy.Revision != nil && fact.Revision != nil && legacy.Revision.Rule == fact.Revision.Rule && fact.Revision.Value >= legacy.Revision.Value {
		proven = true
	}
	if !proven {
		return nil
	}
	encoded, err := json.Marshal(fact)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO analytics.legacy_coverage VALUES(?,?,?,?) ON CONFLICT(dataset_id,generation,fact_id) DO UPDATE SET payload_hash=excluded.payload_hash", datasetID, generation, fact.ID, evidence.Hash(encoded))
	return err
}
