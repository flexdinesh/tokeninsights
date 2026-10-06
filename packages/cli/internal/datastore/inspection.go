package datastore

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

func (s *Store) checkFile() error {
	if s.path == "" {
		return nil
	}
	file, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if s.fileInfo != nil && !os.SameFile(s.fileInfo, info) {
		return errors.New("server_data_file_replaced")
	}
	var magic [4]byte
	if _, err := file.ReadAt(magic[:], 8); err != nil {
		return err
	}
	if string(magic[:]) != "DUCK" {
		return errors.New("invalid_server_data_file")
	}
	return nil
}

// Inspect verifies storage read-only, including automatic default import input.
func Inspect(ctx context.Context, path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if filepath.Base(path) != "server.duckdb" {
			return nil
		}
		legacyPath := filepath.Join(filepath.Dir(path), "server.sqlite")
		if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		legacy, err := serverstore.OpenReadOnly(legacyPath)
		if err != nil {
			return err
		}
		return legacy.Close()
	} else if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	var header [16]byte
	_, readErr := file.ReadAt(header[:], 0)
	_ = file.Close()
	if readErr == nil && string(header[:]) == "SQLite format 3\x00" {
		return errors.New("server SQLite requires import into a new DuckDB: service import --server-db-path NEW.duckdb --legacy-server-db-path OLD.sqlite")
	}
	database, err := connect(path, true)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	_, err = ReadMetadata(ctx, database)
	if err != nil {
		return err
	}
	// Validate table contracts without reading source/analytics rows.
	rows, err := database.QueryContext(ctx, `SELECT e.evidence_id,e.scope,e.harness,e.record_json,e.first_received_at_ms,b.stream_id,b.batch_id,b.request_hash,b.request_bytes,b.receipt_json,i.sequence,i.evidence_id,bi.sequence,bi.evidence_id,s.scope,s.revision,s.processed_revision,s.generation,s.error_code,s.attempts,s.retry_at_ms,d.child,d.parent,o.evidence_id,o.disposition,o.code,o.fact_id,o.generation,o.input_revision,g.generation,g.processor_version,g.state,f.fact_id,f.scope,f.harness,f.session_id,f.session_native_id,f.message_native_id,f.native_request_id,f.occurred_at_ms,f.provider,f.provider_source,f.model,f.usage_scope,f.quality,f.countable,f.input_tokens,f.output_tokens,f.reasoning_tokens,f.cache_read_tokens,f.cache_write_tokens,f.total_tokens,f.directory_key,f.directory_name,f.repository_key,f.repository_name,f.repository_source,f.payload_json,f.generation,f.input_revision,c.generation,c.fact_id,c.payload_hash,p.generation,p.fact_id,p.evidence_id,l.receipt_json FROM raw.evidence e,ingestion.batches b,ingestion.items i,ingestion.batch_items bi,processing.scopes s,processing.dependencies d,processing.outcomes o,analytics.generations g,analytics.facts f,analytics.legacy_coverage c,analytics.provenance p,ingestion.legacy_receipts l LIMIT 0`)
	if err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, table := range []string{"analytics.confirmed", "analytics.estimated", "analytics.legacy"} {
		rows, err := database.QueryContext(ctx, "SELECT fact_id,total_tokens,payload_json FROM "+table+" LIMIT 0")
		if err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}
