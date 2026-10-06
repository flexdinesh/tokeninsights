// Package datastore owns durable raw acceptance and DuckDB analytics projection.
package datastore

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const SchemaVersion = 1
const DatasetID = "default"

//go:embed schema/data.sql
var Schema string

type Store struct {
	database   *sql.DB
	writer     sync.Mutex
	processor  sync.Mutex
	wake       chan struct{}
	admissions chan struct{}
	path       string
	fileInfo   os.FileInfo
}
type Metadata struct {
	DatabaseID, DatasetID                                               string
	Generation, InputRevision, Revision, LastIngestionAtMs, CreatedAtMs int64
	TargetGeneration                                                    int64
}

func (s *Store) SQL() *sql.DB { return s.database }
func (s *Store) Close() error { return s.database.Close() }
func (s *Store) BeginRead(ctx context.Context) (*sql.Tx, error) {
	if err := s.checkFile(); err != nil {
		return nil, err
	}
	// DuckDB supplies snapshot isolation; its Go driver rejects ReadOnly options.
	return s.database.BeginTx(ctx, nil)
}
func ReadMetadata(ctx context.Context, reader interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}) (Metadata, error) {
	var m Metadata
	var role string
	var version int
	err := reader.QueryRowContext(ctx, "SELECT role,schema_version,database_id,dataset_id,generation,target_generation,input_revision,revision,last_ingestion_at_ms,created_at_ms FROM ingestion.metadata WHERE id=1").Scan(&role, &version, &m.DatabaseID, &m.DatasetID, &m.Generation, &m.TargetGeneration, &m.InputRevision, &m.Revision, &m.LastIngestionAtMs, &m.CreatedAtMs)
	if err == nil && (role != "server-data" || version != SchemaVersion || m.DatasetID != DatasetID || m.DatabaseID == "" || m.Generation < 1 || m.TargetGeneration < m.Generation || m.InputRevision < 0 || m.InputRevision > publication.SafeInteger || m.Revision < 0 || m.Revision > publication.SafeInteger) {
		err = errors.New("incompatible_server_data")
	}
	if err == nil {
		var processor int
		err = reader.QueryRowContext(ctx, "SELECT processor_version FROM analytics.generations WHERE generation=?", m.TargetGeneration).Scan(&processor)
		if err == nil && processor > evidence.ProcessorVersion {
			err = errors.New("newer_processor_version")
		}
	}
	return m, err
}
func (s *Store) Metadata(ctx context.Context) (Metadata, error) {
	if err := s.checkFile(); err != nil {
		return Metadata{}, err
	}
	return ReadMetadata(ctx, s.database)
}
func connect(path string, readOnly bool) (*sql.DB, error) {
	u := url.URL{Path: path}
	q := url.Values{}
	q.Set("threads", "2")
	q.Set("memory_limit", "256MB")
	if readOnly {
		q.Set("access_mode", "read_only")
	}
	u.RawQuery = q.Encode()
	database, err := sql.Open("duckdb", u.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(8)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

// Open rejects unrecognized files through a read-only inspection. Initialization
// is staged, checkpointed and atomically published under the server lifetime lock.
func Open(ctx context.Context, path string) (*Store, error) {
	return OpenWithLegacy(ctx, path, "")
}

// OpenWithLegacy imports a verified SQLite baseline into a staged new DuckDB.
// Existing targets reject explicit imports; neither source nor target is reset.
func OpenWithLegacy(ctx context.Context, path, legacyPath string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err == nil {
		if legacyPath != "" {
			return nil, errors.New("legacy_import_requires_new_target")
		}
		if err := Inspect(ctx, abs); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else {
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(filepath.Dir(abs), ".tokeninsights-data-*.duckdb")
		if err != nil {
			return nil, err
		}
		temporary := file.Name()
		_ = file.Close()
		_ = os.Remove(temporary)
		defer func() { _ = os.Remove(temporary); _ = os.Remove(temporary + ".wal") }()
		database, err := connect(temporary, false)
		if err != nil {
			return nil, err
		}
		store := &Store{database: database, wake: make(chan struct{}, 1)}
		err = store.initialize(ctx)
		if err == nil {
			if legacyPath != "" {
				err = store.ImportLegacy(ctx, legacyPath)
			} else {
				err = store.importDefaultLegacy(ctx, abs)
			}
		}
		if err == nil {
			_, err = database.ExecContext(ctx, "CHECKPOINT")
		}
		closeErr := database.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if err := publishDatabase(temporary, abs); err != nil {
			return nil, err
		}
		// Persist the new directory entry before this store can acknowledge data.
		directory, err := os.Open(filepath.Dir(abs))
		if err != nil {
			return nil, err
		}
		syncErr := directory.Sync()
		closeErr = directory.Close()
		if syncErr != nil {
			return nil, syncErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	database, err := connect(abs, false)
	if err != nil {
		return nil, err
	}
	store := &Store{database: database, wake: make(chan struct{}, 1), admissions: make(chan struct{}, 4), path: abs}
	store.fileInfo, err = os.Stat(abs)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.prepareGeneration(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}
func (s *Store) initialize(ctx context.Context) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, Schema); err != nil {
		return err
	}
	id, err := evidence.RandomID()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx, "INSERT INTO analytics.generations VALUES(1,?,'active',?,?)", evidence.ProcessorVersion, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.metadata(id,role,schema_version,database_id,dataset_id,generation,target_generation,created_at_ms) VALUES(1,'server-data',1,?,'default',1,1,?)", id, now); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) prepareGeneration(ctx context.Context) error {
	m, err := s.Metadata(ctx)
	if err != nil {
		return err
	}
	var processor int
	if err := s.database.QueryRowContext(ctx, "SELECT processor_version FROM analytics.generations WHERE generation=?", m.TargetGeneration).Scan(&processor); err != nil {
		return err
	}
	if processor > evidence.ProcessorVersion {
		return errors.New("newer_processor_version")
	}
	if processor == evidence.ProcessorVersion {
		return nil
	}
	_, err = s.Reprocess(ctx)
	return err
}

// Reprocess stages a new generation. Existing queryable history stays active
// until every scope is rebuilt against the current accepted input revisions.
func (s *Store) Reprocess(ctx context.Context) (int64, error) {
	if err := s.checkFile(); err != nil {
		return 0, err
	}
	s.writer.Lock()
	defer s.writer.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := ReadMetadata(ctx, tx)
	if err != nil {
		return 0, err
	}
	if m.TargetGeneration > m.Generation {
		return m.TargetGeneration, nil
	}
	if m.TargetGeneration >= publication.SafeInteger {
		return 0, reject("generation_limit")
	}
	next := m.TargetGeneration + 1
	if _, err := tx.ExecContext(ctx, "INSERT INTO analytics.generations(generation,processor_version,state,created_at_ms) VALUES(?,?,'building',?)", next, evidence.ProcessorVersion, time.Now().UnixMilli()); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET target_generation=? WHERE id=1", next); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE processing.scopes SET error_code='',attempts=0,retry_at_ms=0"); err != nil {
		return 0, err
	}
	var scopes int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM processing.scopes").Scan(&scopes); err != nil {
		return 0, err
	}
	if scopes == 0 {
		if err := activateGeneration(ctx, tx, next); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.signal()
	return next, nil
}

func activateGeneration(ctx context.Context, tx *sql.Tx, generation int64) error {
	if _, err := tx.ExecContext(ctx, "UPDATE analytics.generations SET state='retained' WHERE state='active'"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE analytics.generations SET state='active',activated_at_ms=? WHERE generation=?", time.Now().UnixMilli(), generation); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET generation=?,revision=revision+1 WHERE id=1", generation)
	return err
}

type AdmissionError struct{ Code string }

func (e *AdmissionError) Error() string { return e.Code }
func reject(code string) error          { return &AdmissionError{Code: code} }

func (s *Store) Accept(ctx context.Context, body []byte) (evidence.Response, error) {
	batch, err := evidence.DecodeBatch(body)
	if err != nil {
		if err.Error() == "incompatible" {
			return evidence.Response{}, reject("incompatible")
		}
		return evidence.Response{}, reject("invalid_request")
	}
	if err := s.checkFile(); err != nil {
		return evidence.Response{}, err
	}
	s.writer.Lock()
	defer s.writer.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return evidence.Response{}, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := ReadMetadata(ctx, tx)
	if err != nil {
		return evidence.Response{}, err
	}
	if batch.DatabaseID != m.DatabaseID {
		return evidence.Response{}, reject("database_mismatch")
	}
	var hash, receiptBody string
	err = tx.QueryRowContext(ctx, "SELECT request_hash,receipt_json FROM ingestion.batches WHERE stream_id=? AND batch_id=?", batch.StreamID, batch.BatchID).Scan(&hash, &receiptBody)
	if err == nil {
		if hash != evidence.Hash(body) {
			return evidence.Response{}, reject("batch_conflict")
		}
		var receipt evidence.Receipt
		if err := json.Unmarshal([]byte(receiptBody), &receipt); err != nil {
			return evidence.Response{}, err
		}
		return responseFor(ctx, tx, receipt)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return evidence.Response{}, err
	}
	if m.InputRevision >= publication.SafeInteger {
		return evidence.Response{}, reject("revision_limit")
	}
	revision := m.InputRevision + 1
	now := time.Now().UnixMilli()
	changed := false
	for _, entry := range batch.Entries {
		id := evidence.EvidenceID(entry.Record)
		scope := evidence.Scope(entry.Record)
		var previous string
		err := tx.QueryRowContext(ctx, "SELECT evidence_id FROM ingestion.items WHERE stream_id=? AND sequence=?", batch.StreamID, entry.Sequence).Scan(&previous)
		if err == nil && previous != id {
			return evidence.Response{}, reject("sequence_conflict")
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return evidence.Response{}, err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM raw.evidence WHERE evidence_id=?)", id).Scan(&exists); err != nil {
			return evidence.Response{}, err
		}
		if !exists {
			encoded, err := json.Marshal(entry.Record)
			if err != nil {
				return evidence.Response{}, err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO raw.evidence VALUES(?,?,?,?,?)", id, scope, entry.Record.Harness, string(encoded), now); err != nil {
				return evidence.Response{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO processing.scopes(scope,revision) VALUES(?,?) ON CONFLICT(scope) DO UPDATE SET revision=excluded.revision,error_code='',attempts=0,retry_at_ms=0`, scope, revision); err != nil {
				return evidence.Response{}, err
			}
			for _, parent := range parentScopes(entry.Record) {
				if _, err := tx.ExecContext(ctx, "INSERT INTO processing.dependencies VALUES(?,?) ON CONFLICT DO NOTHING", scope, parent); err != nil {
					return evidence.Response{}, err
				}
			}
			changed = true
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.items VALUES(?,?,?) ON CONFLICT DO NOTHING", batch.StreamID, entry.Sequence, id); err != nil {
			return evidence.Response{}, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.batch_items VALUES(?,?,?,?)", batch.StreamID, batch.BatchID, entry.Sequence, id); err != nil {
			return evidence.Response{}, err
		}
	}
	if !changed {
		revision = m.InputRevision
	}
	// Late ancestor evidence invalidates descendants, including already terminal
	// ambiguous outcomes. Their receipts remain immutable.
	if changed {
		if _, err := tx.ExecContext(ctx, `WITH RECURSIVE affected(scope) AS (SELECT scope FROM processing.scopes WHERE revision=? UNION SELECT d.child FROM processing.dependencies d JOIN affected a ON d.parent=a.scope) UPDATE processing.scopes SET revision=?,error_code='',attempts=0,retry_at_ms=0 WHERE scope IN(SELECT scope FROM affected)`, revision, revision); err != nil {
			return evidence.Response{}, err
		}
	}
	receipt := evidence.Receipt{DatabaseID: m.DatabaseID, DatasetID: m.DatasetID, StreamID: batch.StreamID, BatchID: batch.BatchID, RequestHash: evidence.Hash(body), FromSequence: batch.FromSequence, ToSequence: batch.ToSequence, Accepted: int64(len(batch.Entries)), AcceptedAtMs: now, InputRevision: revision}
	encoded, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.batches VALUES(?,?,?,?,?)", batch.StreamID, batch.BatchID, receipt.RequestHash, body, string(encoded)); err != nil {
		return evidence.Response{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET input_revision=?,last_ingestion_at_ms=? WHERE id=1", revision, now); err != nil {
		return evidence.Response{}, err
	}
	response, err := responseFor(ctx, tx, receipt)
	if err != nil {
		return evidence.Response{}, err
	}
	if err := tx.Commit(); err != nil {
		return evidence.Response{}, err
	}
	s.signal()
	return response, nil
}

func parentScopes(record evidence.Record) []string {
	if record.Harness != "codex" {
		return nil
	}
	contexts := append([]evidence.Context{{Data: record.Data}}, record.Context...)
	set := map[string]bool{}
	for _, entry := range contexts {
		if evidence.String(entry.Data, "type") != "session_meta" {
			continue
		}
		for _, path := range []string{"payload.forked_from_id", "payload.parent_thread_id", "payload.source.subagent.thread_spawn.parent_thread_id"} {
			if id := evidence.String(entry.Data, path); id != "" {
				set[evidence.SessionScope("codex", id)] = true
			}
		}
	}
	var result []string
	for scope := range set {
		result = append(result, scope)
	}
	return result
}
func responseFor(ctx context.Context, tx *sql.Tx, receipt evidence.Receipt) (evidence.Response, error) {
	m, err := ReadMetadata(ctx, tx)
	if err != nil {
		return evidence.Response{}, err
	}
	response := evidence.Response{Receipt: receipt, Processing: evidence.Status{Generation: m.TargetGeneration, InputRevision: m.InputRevision, Items: []evidence.Outcome{}}}
	rows, err := tx.QueryContext(ctx, `SELECT i.evidence_id,COALESCE(o.disposition,'pending'),COALESCE(o.code,''),COALESCE(o.fact_id,''),COALESCE(o.generation,0),COALESCE(o.input_revision,0),s.revision,s.processed_revision,s.generation FROM ingestion.batch_items i JOIN raw.evidence e ON e.evidence_id=i.evidence_id JOIN processing.scopes s ON s.scope=e.scope LEFT JOIN processing.outcomes o ON o.evidence_id=i.evidence_id AND o.generation=? WHERE i.stream_id=? AND i.batch_id=? ORDER BY i.sequence`, m.TargetGeneration, receipt.StreamID, receipt.BatchID)
	if err != nil {
		return response, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var outcome evidence.Outcome
		var revision, processed, generation int64
		if err := rows.Scan(&outcome.EvidenceID, &outcome.Disposition, &outcome.Code, &outcome.FactID, &outcome.Generation, &outcome.InputRevision, &revision, &processed, &generation); err != nil {
			return response, err
		}
		if revision != processed || generation != m.TargetGeneration || outcome.Generation != m.TargetGeneration {
			outcome.Disposition = "pending"
			outcome.Code = ""
			outcome.FactID = ""
			response.Processing.Pending++
		}
		response.Processing.Items = append(response.Processing.Items, outcome)
	}
	return response, rows.Err()
}
func (s *Store) Receipt(ctx context.Context, stream, batch string) (evidence.Response, error) {
	tx, err := s.BeginRead(ctx)
	if err != nil {
		return evidence.Response{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var body string
	if err := tx.QueryRowContext(ctx, "SELECT receipt_json FROM ingestion.batches WHERE stream_id=? AND batch_id=?", stream, batch).Scan(&body); err != nil {
		return evidence.Response{}, err
	}
	var receipt evidence.Receipt
	if err := json.Unmarshal([]byte(body), &receipt); err != nil {
		return evidence.Response{}, err
	}
	return responseFor(ctx, tx, receipt)
}

func (s *Store) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Store) Run(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for {
			worked, err := s.ProcessNext(ctx)
			if err != nil {
				if ctx.Err() == nil && report != nil {
					report(err)
				}
				break
			}
			if !worked {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

type work struct {
	revision, generation int64
	root                 string
	scopes               map[string]int64
	records              []evidence.Stored
}

func (s *Store) loadWork(ctx context.Context) (work, error) {
	result := work{scopes: map[string]int64{}}
	tx, err := s.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := ReadMetadata(ctx, tx)
	if err != nil {
		return result, err
	}
	result.revision = m.InputRevision
	result.generation = m.TargetGeneration
	var scope string
	if err := tx.QueryRowContext(ctx, "SELECT scope FROM processing.scopes WHERE (processed_revision<>revision OR generation<>?) AND retry_at_ms<=? ORDER BY revision,scope LIMIT 1", m.TargetGeneration, time.Now().UnixMilli()).Scan(&scope); err != nil {
		return result, err
	}
	result.root = scope
	rows, err := tx.QueryContext(ctx, `WITH RECURSIVE connected(scope) AS (SELECT CAST(? AS VARCHAR) UNION SELECT CASE WHEN d.child=c.scope THEN d.parent ELSE d.child END FROM processing.dependencies d JOIN connected c ON d.child=c.scope OR d.parent=c.scope) SELECT s.scope,s.revision FROM processing.scopes s WHERE s.scope IN(SELECT scope FROM connected)`, scope)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var scope string
		var revision int64
		if err := rows.Scan(&scope, &revision); err != nil {
			_ = rows.Close()
			return result, err
		}
		result.scopes[scope] = revision
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return result, err
	}
	for scope := range result.scopes {
		rows, err := tx.QueryContext(ctx, "SELECT evidence_id,record_json FROM raw.evidence WHERE scope=? ORDER BY evidence_id", scope)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var stored evidence.Stored
			var body string
			stored.Scope = scope
			if err := rows.Scan(&stored.ID, &body); err != nil {
				_ = rows.Close()
				return result, err
			}
			if err := json.Unmarshal([]byte(body), &stored.Record); err != nil {
				_ = rows.Close()
				return result, err
			}
			result.records = append(result.records, stored)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Store) ProcessNext(ctx context.Context) (worked bool, failure error) {
	if !s.processor.TryLock() {
		return false, nil
	}
	defer s.processor.Unlock()
	work, err := s.loadWork(ctx)
	defer func() {
		if failure != nil && ctx.Err() == nil && work.root != "" {
			s.recordFailure(ctx, work.root)
		}
	}()

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	projection, err := pipeline.ProcessEvidence(ctx, work.records)
	if err != nil {
		return false, err
	}
	s.writer.Lock()
	defer s.writer.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := ReadMetadata(ctx, tx)
	if err != nil {
		return false, err
	}
	// Fence the entire dependency component, including newly connected scopes.
	// Unrelated ingestion cannot starve this scope's asynchronous processing.
	if m.TargetGeneration != work.generation {
		return true, nil
	}
	current, err := tx.QueryContext(ctx, `WITH RECURSIVE connected(scope) AS (SELECT CAST(? AS VARCHAR) UNION SELECT CASE WHEN d.child=c.scope THEN d.parent ELSE d.child END FROM processing.dependencies d JOIN connected c ON d.child=c.scope OR d.parent=c.scope) SELECT s.scope,s.revision FROM processing.scopes s WHERE s.scope IN(SELECT scope FROM connected)`, work.root)
	if err != nil {
		return false, err
	}
	matched := true
	count := 0
	for current.Next() {
		var scope string
		var revision int64
		if err := current.Scan(&scope, &revision); err != nil {
			_ = current.Close()
			return false, err
		}
		if expected, exists := work.scopes[scope]; !exists || expected != revision {
			matched = false
		}
		count++
	}
	err = current.Err()
	_ = current.Close()
	if err != nil {
		return false, err
	}
	if !matched || count != len(work.scopes) {
		return true, nil
	}
	for scope := range work.scopes {
		if _, err := tx.ExecContext(ctx, "DELETE FROM analytics.provenance WHERE generation=? AND fact_id IN(SELECT fact_id FROM analytics.facts WHERE scope=? AND generation=?)", work.generation, scope, work.generation); err != nil {
			return false, err
		}
		for _, table := range []string{"analytics.facts", "analytics.estimates"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE scope=? AND generation=?", scope, work.generation); err != nil {
				return false, err
			}
		}
	}
	for _, contribution := range projection.Contributions {
		if err := insertFact(ctx, tx, "analytics.facts", contribution.Fact, work.generation, work.revision, "", ""); err != nil {
			return false, err
		}
		if err := proveLegacyCoverage(ctx, tx, contribution.Fact, work.generation); err != nil {
			return false, err
		}
		for _, id := range contribution.EvidenceIDs {
			if _, err := tx.ExecContext(ctx, "INSERT INTO analytics.provenance VALUES(?,?,?) ON CONFLICT DO NOTHING", work.generation, contribution.Fact.ID, id); err != nil {
				return false, err
			}
		}
	}
	for _, estimate := range projection.Estimates {
		if err := insertFact(ctx, tx, "analytics.estimates", estimate.Fact, work.generation, work.revision, estimate.EvidenceID, estimate.Code); err != nil {
			return false, err
		}
	}
	for _, outcome := range projection.Outcomes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO processing.outcomes VALUES(?,?,?,?,?,?) ON CONFLICT(generation,evidence_id) DO UPDATE SET disposition=excluded.disposition,code=excluded.code,fact_id=excluded.fact_id,input_revision=excluded.input_revision`, outcome.EvidenceID, outcome.Disposition, outcome.Code, outcome.FactID, work.generation, work.revision); err != nil {
			return false, err
		}
	}
	for scope, revision := range work.scopes {
		if _, err := tx.ExecContext(ctx, "UPDATE processing.scopes SET processed_revision=?,generation=?,error_code='',attempts=0,retry_at_ms=0 WHERE scope=?", revision, work.generation, scope); err != nil {
			return false, err
		}
	}
	if m.Revision >= publication.SafeInteger {
		return false, reject("revision_limit")
	}
	if m.Generation == work.generation {
		if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET revision=revision+1 WHERE id=1"); err != nil {
			return false, err
		}
	} else {
		var pending int64
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM processing.scopes WHERE processed_revision<>revision OR generation<>?", work.generation).Scan(&pending); err != nil {
			return false, err
		}
		if pending == 0 {
			if err := activateGeneration(ctx, tx, work.generation); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func insertFact(ctx context.Context, tx *sql.Tx, table string, fact publication.Fact, generation, revision int64, evidenceID, reason string) error {
	if table != "analytics.legacy" {
		if err := publication.ValidateFact(fact); err != nil {
			return err
		}
	}
	body, err := json.Marshal(fact)
	if err != nil {
		return err
	}
	message := ""
	if fact.Message != nil {
		message = fact.Message.NativeID
	}
	location := publication.Location{}
	if fact.Location != nil {
		location = *fact.Location
	}
	args := []interface{}{fact.ID, evidence.SessionScope(fact.Harness, fact.Session.NativeID), fact.Harness, fact.Session.ID, fact.Session.NativeID, message, fact.NativeRequestID, fact.OccurredAtMs, fact.Provider, fact.ProviderSource, fact.Model, fact.UsageScope, fact.Quality, fact.Countable, fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens, fact.TotalTokens, location.DirectoryKey, location.DirectoryName, location.RepositoryKey, location.RepositoryName, location.RepositorySource, string(body), generation, revision}
	if table == "analytics.estimates" {
		args = append(args, evidenceID, reason)
	}
	placeholders := "?"
	for range args[1:] {
		placeholders += ",?"
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO "+table+" VALUES("+placeholders+")", args...)
	if err != nil {
		return fmt.Errorf("insert projected contribution: %w", err)
	}
	return nil
}

// Failures remain pending and retryable. Backoff lets independent scopes progress.
func (s *Store) recordFailure(ctx context.Context, scope string) {
	s.writer.Lock()
	defer s.writer.Unlock()
	var attempts int64
	if err := s.database.QueryRowContext(ctx, "SELECT attempts FROM processing.scopes WHERE scope=?", scope).Scan(&attempts); err != nil {
		return
	}
	const maxBackoffExponent = 6
	exponent := min(attempts, maxBackoffExponent)
	delay := time.Second * time.Duration(int64(1)<<exponent)
	_, _ = s.database.ExecContext(ctx, "UPDATE processing.scopes SET attempts=attempts+1,retry_at_ms=?,error_code='processing_failed' WHERE scope=?", time.Now().Add(delay).UnixMilli(), scope)
}
