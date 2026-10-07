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
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const SchemaVersion = 2
const KindPersonal = "personal"
const KindHosted = "hosted"
const DatasetID = "default"

//go:embed schema/data.sql
var Schema string

type Store struct {
	database    *sql.DB
	writer      *sync.Mutex
	worker      *dataengine.Worker
	admissions  chan struct{}
	path        string
	fileInfo    os.FileInfo
	datasetID   string
	kind        string
	root        bool
	nextDataset *string
}
type Metadata struct {
	DatabaseID, DatasetID, Kind                                         string
	Generation, InputRevision, Revision, LastIngestionAtMs, CreatedAtMs int64
	TargetGeneration                                                    int64
}

func (s *Store) SQL() *sql.DB { return s.database }
func (s *Store) Close() error {
	if !s.root {
		return nil
	}
	return s.database.Close()
}
func (s *Store) BeginRead(ctx context.Context) (*sql.Tx, error) {
	if err := s.checkFile(); err != nil {
		return nil, err
	}
	// DuckDB supplies snapshot isolation; its Go driver rejects ReadOnly options.
	return s.database.BeginTx(ctx, nil)
}

type metadataReader interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

// ReadMetadata retains the personal/default read contract.
func ReadMetadata(ctx context.Context, reader metadataReader) (Metadata, error) {
	return ReadMetadataForDataset(ctx, reader, DatasetID)
}
func ReadMetadataForDataset(ctx context.Context, reader metadataReader, datasetID string) (Metadata, error) {
	var m Metadata
	var role string
	var version int
	err := reader.QueryRowContext(ctx, "SELECT role,schema_version,database_id,dataset_id,server_kind,generation,target_generation,input_revision,revision,last_ingestion_at_ms,created_at_ms FROM ingestion.metadata WHERE dataset_id=?", datasetID).Scan(&role, &version, &m.DatabaseID, &m.DatasetID, &m.Kind, &m.Generation, &m.TargetGeneration, &m.InputRevision, &m.Revision, &m.LastIngestionAtMs, &m.CreatedAtMs)
	if err == nil && (role != "server-data" || version != SchemaVersion || m.DatasetID == "" || (m.Kind != KindPersonal && m.Kind != KindHosted) || m.DatabaseID == "" || m.Generation < 1 || m.TargetGeneration < m.Generation || m.TargetGeneration > publication.SafeInteger || m.InputRevision < 0 || m.InputRevision > publication.SafeInteger || m.Revision < 0 || m.Revision > publication.SafeInteger) {
		err = errors.New("incompatible_server_data")
	}
	if err == nil {
		var activeState, targetState string
		var activeCount int64
		err = reader.QueryRowContext(ctx, "SELECT active.state,target.state,(SELECT COUNT(*) FROM analytics.generations WHERE dataset_id=? AND state='active') FROM analytics.generations active JOIN analytics.generations target ON target.dataset_id=active.dataset_id WHERE active.dataset_id=? AND active.generation=? AND target.generation=?", datasetID, datasetID, m.Generation, m.TargetGeneration).Scan(&activeState, &targetState, &activeCount)
		if err == nil && !validGenerationStates(activeState, targetState, activeCount, m.Generation, m.TargetGeneration) {
			err = errors.New("incompatible_server_data")
		}
	}
	if err == nil {
		var processor int
		err = reader.QueryRowContext(ctx, "SELECT processor_version FROM analytics.generations WHERE dataset_id=? AND generation=?", datasetID, m.TargetGeneration).Scan(&processor)
		if err == nil && processor > evidence.ProcessorVersion {
			err = errors.New("newer_processor_version")
		}
		if err == nil {
			var newest int
			err = reader.QueryRowContext(ctx, "SELECT MAX(processor_version) FROM analytics.generations WHERE dataset_id=?", datasetID).Scan(&newest)
			if err == nil && newest > evidence.ProcessorVersion {
				err = errors.New("newer_processor_version")
			}
		}
	}
	return m, err
}
func validGenerationStates(activeState, targetState string, activeCount, generation, target int64) bool {
	expectedTarget := "active"
	if target > generation {
		expectedTarget = "building"
	}
	return activeCount == 1 && activeState == "active" && targetState == expectedTarget
}
func (s *Store) DatasetID() string { return s.datasetID }
func (s *Store) Kind() string      { return s.kind }

// ForDataset shares the connection, write coordinator and worker with its owner.
// Authorization belongs to the caller; this handle cannot select another dataset.
func (s *Store) ForDataset(datasetID string) *Store {
	scoped := *s
	scoped.datasetID = datasetID
	scoped.root = false
	return &scoped
}
func (s *Store) Metadata(ctx context.Context) (Metadata, error) {
	tx, err := s.BeginRead(ctx)
	if err != nil {
		return Metadata{}, err
	}
	defer func() { _ = tx.Rollback() }()
	return ReadMetadataForDataset(ctx, tx, s.datasetID)
}
func (s *Store) WriteTransaction(ctx context.Context, operation func(*sql.Tx) error) error {
	if err := s.checkFile(); err != nil {
		return err
	}
	s.writer.Lock()
	defer s.writer.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CreateDataset(ctx context.Context, datasetID string) error {
	return s.WriteTransaction(ctx, func(tx *sql.Tx) error { return s.CreateDatasetInTx(ctx, tx, datasetID) })
}
func (s *Store) CreateDatasetInTx(ctx context.Context, tx *sql.Tx, datasetID string) error {
	if datasetID == "" || (s.kind == KindPersonal && datasetID != DatasetID) {
		return errors.New("invalid_dataset")
	}
	var databaseID string
	if err := tx.QueryRowContext(ctx, "SELECT database_id FROM ingestion.instance WHERE id=1").Scan(&databaseID); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx, "INSERT INTO analytics.generations VALUES(?,1,?,'active',?,?)", datasetID, evidence.ProcessorVersion, now, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO ingestion.metadata(dataset_id,role,schema_version,database_id,server_kind,generation,target_generation,created_at_ms) VALUES(?,'server-data',2,?,?,1,1,?)", datasetID, databaseID, s.kind, now)
	return err
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
	return OpenWithOptions(ctx, path, Options{Kind: KindPersonal})
}

// OpenWithLegacy imports a verified SQLite baseline into a staged new DuckDB.
// Existing targets reject explicit imports; neither source nor target is reset.
func OpenWithLegacy(ctx context.Context, path, legacyPath string) (*Store, error) {
	return OpenWithOptions(ctx, path, Options{Kind: KindPersonal, LegacyPath: legacyPath})
}

type Options struct {
	Kind       string
	LegacyPath string
}

func OpenKind(ctx context.Context, path, kind string) (*Store, error) {
	return OpenWithOptions(ctx, path, Options{Kind: kind})
}
func OpenWithOptions(ctx context.Context, path string, options Options) (*Store, error) {
	kind := options.Kind
	if kind == "" {
		kind = KindPersonal
	}
	if kind != KindPersonal && kind != KindHosted {
		return nil, errors.New("invalid_server_kind")
	}
	legacyPath := options.LegacyPath
	if kind == KindHosted && legacyPath != "" {
		return nil, errors.New("hosted_legacy_import_unsupported")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err == nil {
		if legacyPath != "" {
			return nil, errors.New("legacy_import_requires_new_target")
		}
		if err := inspectAndUpgrade(ctx, abs, kind); err != nil {
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
		store := &Store{database: database, writer: &sync.Mutex{}, worker: dataengine.NewWorker(), datasetID: DatasetID, kind: kind, root: true, nextDataset: new(string)}
		err = store.initialize(ctx)
		if err == nil {
			if legacyPath != "" {
				err = store.ImportLegacy(ctx, legacyPath)
			} else if kind == KindPersonal {
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
	store := &Store{database: database, writer: &sync.Mutex{}, worker: dataengine.NewWorker(), admissions: make(chan struct{}, 4), path: abs, datasetID: DatasetID, kind: kind, root: true, nextDataset: new(string)}
	store.fileInfo, err = os.Stat(abs)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.prepareAllGenerations(ctx); err != nil {
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
	if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.instance VALUES(1,'server-data',2,?,?,?)", id, s.kind, now); err != nil {
		return err
	}
	if s.kind == KindPersonal {
		if err := s.CreateDatasetInTx(ctx, tx, DatasetID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) prepareAllGenerations(ctx context.Context) error {
	rows, err := s.database.QueryContext(ctx, "SELECT dataset_id FROM ingestion.metadata ORDER BY dataset_id")
	if err != nil {
		return err
	}
	var datasets []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		datasets = append(datasets, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, id := range datasets {
		if err := s.ForDataset(id).prepareGeneration(ctx); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) prepareGeneration(ctx context.Context) error {
	m, err := s.Metadata(ctx)
	if err != nil {
		return err
	}
	var processor int
	if err := s.database.QueryRowContext(ctx, "SELECT processor_version FROM analytics.generations WHERE dataset_id=? AND generation=?", s.datasetID, m.TargetGeneration).Scan(&processor); err != nil {
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
	m, err := ReadMetadataForDataset(ctx, tx, s.datasetID)
	if err != nil {
		return 0, err
	}
	if m.TargetGeneration > m.Generation {
		var processor int
		if err := tx.QueryRowContext(ctx, "SELECT processor_version FROM analytics.generations WHERE dataset_id=? AND generation=?", s.datasetID, m.TargetGeneration).Scan(&processor); err != nil {
			return 0, err
		}
		if processor == evidence.ProcessorVersion {
			return m.TargetGeneration, nil
		}
		// Never finish an interrupted older generation under newer rules while
		// retaining its old version label. Keep its evidence/projection for review.
		if _, err := tx.ExecContext(ctx, "UPDATE analytics.generations SET state='retained' WHERE dataset_id=? AND generation=?", s.datasetID, m.TargetGeneration); err != nil {
			return 0, err
		}
	}
	if m.TargetGeneration >= publication.SafeInteger {
		return 0, reject("generation_limit")
	}
	next := m.TargetGeneration + 1
	if _, err := tx.ExecContext(ctx, "INSERT INTO analytics.generations(dataset_id,generation,processor_version,state,created_at_ms) VALUES(?,?,?,'building',?)", s.datasetID, next, evidence.ProcessorVersion, time.Now().UnixMilli()); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET target_generation=? WHERE dataset_id=?", next, s.datasetID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE processing.scopes SET error_code='',attempts=0,retry_at_ms=0 WHERE dataset_id=?", s.datasetID); err != nil {
		return 0, err
	}
	var scopes int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM processing.scopes WHERE dataset_id=?", s.datasetID).Scan(&scopes); err != nil {
		return 0, err
	}
	if scopes == 0 {
		if err := activateGeneration(ctx, tx, s.datasetID, next); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.signal()
	return next, nil
}

func activateGeneration(ctx context.Context, tx *sql.Tx, datasetID string, generation int64) error {
	if _, err := tx.ExecContext(ctx, "UPDATE analytics.generations SET state='retained' WHERE state='active' AND dataset_id=?", datasetID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE analytics.generations SET state='active',activated_at_ms=? WHERE dataset_id=? AND generation=?", time.Now().UnixMilli(), datasetID, generation); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET generation=?,revision=revision+1 WHERE dataset_id=?", generation, datasetID)
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
	m, err := ReadMetadataForDataset(ctx, tx, s.datasetID)
	if err != nil {
		return evidence.Response{}, err
	}
	if batch.EffectiveDatasetID() != m.DatasetID {
		return evidence.Response{}, reject("dataset_mismatch")
	}
	if s.kind == KindHosted && batch.ProtocolVersion != evidence.ProtocolVersion {
		return evidence.Response{}, reject("incompatible")
	}
	if batch.DatabaseID != m.DatabaseID {
		return evidence.Response{}, reject("database_mismatch")
	}
	var hash, receiptBody string
	err = tx.QueryRowContext(ctx, "SELECT request_hash,receipt_json FROM ingestion.batches WHERE dataset_id=? AND stream_id=? AND batch_id=?", s.datasetID, batch.StreamID, batch.BatchID).Scan(&hash, &receiptBody)
	if err == nil {
		if hash != evidence.Hash(body) {
			return evidence.Response{}, reject("batch_conflict")
		}
		var receipt evidence.Receipt
		if err := json.Unmarshal([]byte(receiptBody), &receipt); err != nil {
			return evidence.Response{}, err
		}
		return responseFor(ctx, tx, s.datasetID, receipt)
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
		err := tx.QueryRowContext(ctx, "SELECT evidence_id FROM ingestion.items WHERE dataset_id=? AND stream_id=? AND sequence=?", s.datasetID, batch.StreamID, entry.Sequence).Scan(&previous)
		if err == nil && previous != id {
			return evidence.Response{}, reject("sequence_conflict")
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return evidence.Response{}, err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM raw.evidence WHERE dataset_id=? AND evidence_id=?)", s.datasetID, id).Scan(&exists); err != nil {
			return evidence.Response{}, err
		}
		if !exists {
			encoded, err := json.Marshal(entry.Record)
			if err != nil {
				return evidence.Response{}, err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO raw.evidence VALUES(?,?,?,?,?,?)", s.datasetID, id, scope, entry.Record.Harness, string(encoded), now); err != nil {
				return evidence.Response{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO processing.scopes(dataset_id,scope,revision) VALUES(?,?,?) ON CONFLICT(dataset_id,scope) DO UPDATE SET revision=excluded.revision,error_code='',attempts=0,retry_at_ms=0`, s.datasetID, scope, revision); err != nil {
				return evidence.Response{}, err
			}
			for _, parent := range parentScopes(entry.Record) {
				if _, err := tx.ExecContext(ctx, "INSERT INTO processing.dependencies VALUES(?,?,?) ON CONFLICT DO NOTHING", s.datasetID, scope, parent); err != nil {
					return evidence.Response{}, err
				}
			}
			changed = true
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.items VALUES(?,?,?,?) ON CONFLICT DO NOTHING", s.datasetID, batch.StreamID, entry.Sequence, id); err != nil {
			return evidence.Response{}, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.batch_items VALUES(?,?,?,?,?)", s.datasetID, batch.StreamID, batch.BatchID, entry.Sequence, id); err != nil {
			return evidence.Response{}, err
		}
	}
	if !changed {
		revision = m.InputRevision
	}
	// Late ancestor evidence invalidates descendants, including already terminal
	// ambiguous outcomes. Their receipts remain immutable.
	if changed {
		if _, err := tx.ExecContext(ctx, `WITH RECURSIVE affected(scope) AS (SELECT scope FROM processing.scopes WHERE dataset_id=? AND revision=? UNION SELECT d.child FROM processing.dependencies d JOIN affected a ON d.parent=a.scope WHERE d.dataset_id=?) UPDATE processing.scopes SET revision=?,error_code='',attempts=0,retry_at_ms=0 WHERE dataset_id=? AND scope IN(SELECT scope FROM affected)`, s.datasetID, revision, s.datasetID, revision, s.datasetID); err != nil {
			return evidence.Response{}, err
		}
	}
	receipt := evidence.Receipt{DatabaseID: m.DatabaseID, DatasetID: m.DatasetID, StreamID: batch.StreamID, BatchID: batch.BatchID, RequestHash: evidence.Hash(body), FromSequence: batch.FromSequence, ToSequence: batch.ToSequence, Accepted: int64(len(batch.Entries)), AcceptedAtMs: now, InputRevision: revision}
	encoded, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.batches VALUES(?,?,?,?,?,?)", s.datasetID, batch.StreamID, batch.BatchID, receipt.RequestHash, body, string(encoded)); err != nil {
		return evidence.Response{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET input_revision=?,last_ingestion_at_ms=? WHERE dataset_id=?", revision, now, s.datasetID); err != nil {
		return evidence.Response{}, err
	}
	response, err := responseFor(ctx, tx, s.datasetID, receipt)
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
func responseFor(ctx context.Context, tx *sql.Tx, datasetID string, receipt evidence.Receipt) (evidence.Response, error) {
	m, err := ReadMetadataForDataset(ctx, tx, datasetID)
	if err != nil {
		return evidence.Response{}, err
	}
	if receipt.DatasetID != datasetID || receipt.DatabaseID != m.DatabaseID {
		return evidence.Response{}, errors.New("stored_receipt_binding_mismatch")
	}
	response := evidence.Response{Receipt: receipt, Processing: evidence.Status{Generation: m.TargetGeneration, InputRevision: m.InputRevision, Items: []evidence.Outcome{}}}
	rows, err := tx.QueryContext(ctx, `SELECT i.evidence_id,COALESCE(o.disposition,'pending'),COALESCE(o.code,''),COALESCE(o.fact_id,''),COALESCE(o.generation,0),COALESCE(o.input_revision,0),s.revision,s.processed_revision,s.generation FROM ingestion.batch_items i JOIN raw.evidence e ON e.dataset_id=i.dataset_id AND e.evidence_id=i.evidence_id JOIN processing.scopes s ON s.dataset_id=e.dataset_id AND s.scope=e.scope LEFT JOIN processing.outcomes o ON o.dataset_id=i.dataset_id AND o.evidence_id=i.evidence_id AND o.generation=? WHERE i.dataset_id=? AND i.stream_id=? AND i.batch_id=? ORDER BY i.sequence`, m.TargetGeneration, datasetID, receipt.StreamID, receipt.BatchID)
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
	if err := tx.QueryRowContext(ctx, "SELECT receipt_json FROM ingestion.batches WHERE dataset_id=? AND stream_id=? AND batch_id=?", s.datasetID, stream, batch).Scan(&body); err != nil {
		return evidence.Response{}, err
	}
	var receipt evidence.Receipt
	if err := json.Unmarshal([]byte(body), &receipt); err != nil {
		return evidence.Response{}, err
	}
	return responseFor(ctx, tx, s.datasetID, receipt)
}

func (s *Store) signal() { s.worker.Wake() }

func (s *Store) Run(ctx context.Context, report func(error)) {
	s.worker.Run(ctx, s, report)
}

func (s *Store) ProcessNext(ctx context.Context) (bool, error) {
	return s.worker.ProcessNext(ctx, s)
}

// LoadWork selects eligible datasets fairly, then reads one consistent connected
// component. Calls are serialized by the shared dataengine worker.
func (s *Store) LoadWork(ctx context.Context) (dataengine.Work, bool, error) {
	if s.root {
		var datasetID string
		err := s.database.QueryRowContext(ctx, `SELECT DISTINCT m.dataset_id FROM ingestion.metadata m JOIN processing.scopes p ON p.dataset_id=m.dataset_id WHERE (p.processed_revision<>p.revision OR p.generation<>m.target_generation) AND p.retry_at_ms<=? ORDER BY CASE WHEN m.dataset_id>? THEN 0 ELSE 1 END,m.dataset_id LIMIT 1`, time.Now().UnixMilli(), *s.nextDataset).Scan(&datasetID)
		if errors.Is(err, sql.ErrNoRows) {
			return dataengine.Work{}, false, nil
		}
		if err != nil {
			return dataengine.Work{}, false, err
		}
		*s.nextDataset = datasetID
		s = s.ForDataset(datasetID)
	}
	work, err := s.loadWork(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return work, false, nil
	}
	return work, err == nil, err
}

func (s *Store) loadWork(ctx context.Context) (dataengine.Work, error) {
	result := dataengine.Work{DatasetID: s.datasetID, Scopes: map[string]int64{}}
	tx, err := s.BeginRead(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := ReadMetadataForDataset(ctx, tx, s.datasetID)
	if err != nil {
		return result, err
	}
	result.Revision = m.InputRevision
	result.Generation = m.TargetGeneration
	var scope string
	if err := tx.QueryRowContext(ctx, "SELECT scope FROM processing.scopes WHERE dataset_id=? AND (processed_revision<>revision OR generation<>?) AND retry_at_ms<=? ORDER BY revision,scope LIMIT 1", s.datasetID, m.TargetGeneration, time.Now().UnixMilli()).Scan(&scope); err != nil {
		return result, err
	}
	result.Root = scope
	rows, err := tx.QueryContext(ctx, `WITH RECURSIVE connected(scope) AS (SELECT CAST(? AS VARCHAR) UNION SELECT CASE WHEN d.child=c.scope THEN d.parent ELSE d.child END FROM processing.dependencies d JOIN connected c ON d.child=c.scope OR d.parent=c.scope WHERE d.dataset_id=?) SELECT s.scope,s.revision FROM processing.scopes s WHERE s.dataset_id=? AND s.scope IN(SELECT scope FROM connected)`, scope, s.datasetID, s.datasetID)
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
		result.Scopes[scope] = revision
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return result, err
	}
	for scope := range result.Scopes {
		rows, err := tx.QueryContext(ctx, "SELECT evidence_id,record_json FROM raw.evidence WHERE dataset_id=? AND scope=? ORDER BY evidence_id", s.datasetID, scope)
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
			result.Records = append(result.Records, stored)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// PublishProjection preserves the complete component fence and all projection,
// provenance, outcome and generation changes in a single write transaction.
func (s *Store) PublishProjection(ctx context.Context, work dataengine.Work, projection evidence.Projection) (bool, error) {
	if !s.root && work.DatasetID != s.datasetID {
		return false, reject("dataset_mismatch")
	}
	s = s.ForDataset(work.DatasetID)
	s.writer.Lock()
	defer s.writer.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := ReadMetadataForDataset(ctx, tx, s.datasetID)
	if err != nil {
		return false, err
	}
	// Fence the entire dependency component, including newly connected scopes.
	// Unrelated ingestion cannot starve this scope's asynchronous processing.
	if m.TargetGeneration != work.Generation {
		return false, nil
	}
	current, err := tx.QueryContext(ctx, `WITH RECURSIVE connected(scope) AS (SELECT CAST(? AS VARCHAR) UNION SELECT CASE WHEN d.child=c.scope THEN d.parent ELSE d.child END FROM processing.dependencies d JOIN connected c ON d.child=c.scope OR d.parent=c.scope WHERE d.dataset_id=?) SELECT s.scope,s.revision FROM processing.scopes s WHERE s.dataset_id=? AND s.scope IN(SELECT scope FROM connected)`, work.Root, s.datasetID, s.datasetID)
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
		if expected, exists := work.Scopes[scope]; !exists || expected != revision {
			matched = false
		}
		count++
	}
	err = current.Err()
	_ = current.Close()
	if err != nil {
		return false, err
	}
	if !matched || count != len(work.Scopes) {
		return false, nil
	}
	for scope := range work.Scopes {
		if _, err := tx.ExecContext(ctx, "DELETE FROM analytics.provenance WHERE dataset_id=? AND generation=? AND fact_id IN(SELECT fact_id FROM analytics.facts WHERE dataset_id=? AND scope=? AND generation=?)", s.datasetID, work.Generation, s.datasetID, scope, work.Generation); err != nil {
			return false, err
		}
		for _, table := range []string{"analytics.facts", "analytics.estimates"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE dataset_id=? AND scope=? AND generation=?", s.datasetID, scope, work.Generation); err != nil {
				return false, err
			}
		}
	}
	for _, contribution := range projection.Contributions {
		if err := insertFact(ctx, tx, s.datasetID, "analytics.facts", contribution.Fact, work.Generation, work.Revision, "", ""); err != nil {
			return false, err
		}
		if err := proveLegacyCoverage(ctx, tx, s.datasetID, contribution.Fact, work.Generation); err != nil {
			return false, err
		}
		for _, id := range contribution.EvidenceIDs {
			if _, err := tx.ExecContext(ctx, "INSERT INTO analytics.provenance VALUES(?,?,?,?) ON CONFLICT DO NOTHING", s.datasetID, work.Generation, contribution.Fact.ID, id); err != nil {
				return false, err
			}
		}
	}
	for _, estimate := range projection.Estimates {
		if err := insertFact(ctx, tx, s.datasetID, "analytics.estimates", estimate.Fact, work.Generation, work.Revision, estimate.EvidenceID, estimate.Code); err != nil {
			return false, err
		}
	}
	for _, outcome := range projection.Outcomes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO processing.outcomes VALUES(?,?,?,?,?,?,?) ON CONFLICT(dataset_id,generation,evidence_id) DO UPDATE SET disposition=excluded.disposition,code=excluded.code,fact_id=excluded.fact_id,input_revision=excluded.input_revision`, s.datasetID, outcome.EvidenceID, outcome.Disposition, outcome.Code, outcome.FactID, work.Generation, work.Revision); err != nil {
			return false, err
		}
	}
	for scope, revision := range work.Scopes {
		if _, err := tx.ExecContext(ctx, "UPDATE processing.scopes SET processed_revision=?,generation=?,error_code='',attempts=0,retry_at_ms=0 WHERE dataset_id=? AND scope=?", revision, work.Generation, s.datasetID, scope); err != nil {
			return false, err
		}
	}
	if m.Revision >= publication.SafeInteger {
		return false, reject("revision_limit")
	}
	if m.Generation == work.Generation {
		if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET revision=revision+1 WHERE dataset_id=?", s.datasetID); err != nil {
			return false, err
		}
	} else {
		var pending int64
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM processing.scopes WHERE dataset_id=? AND (processed_revision<>revision OR generation<>?)", s.datasetID, work.Generation).Scan(&pending); err != nil {
			return false, err
		}
		if pending == 0 {
			if err := activateGeneration(ctx, tx, s.datasetID, work.Generation); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func insertFact(ctx context.Context, tx *sql.Tx, datasetID, table string, fact publication.Fact, generation, revision int64, evidenceID, reason string) error {
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
	args := []interface{}{datasetID, fact.ID, evidence.SessionScope(fact.Harness, fact.Session.NativeID), fact.Harness, fact.Session.ID, fact.Session.NativeID, message, fact.NativeRequestID, fact.OccurredAtMs, fact.Provider, fact.ProviderSource, fact.Model, fact.UsageScope, fact.Quality, fact.Countable, fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens, fact.TotalTokens, location.DirectoryKey, location.DirectoryName, location.RepositoryKey, location.RepositoryName, location.RepositorySource, string(body), generation, revision}
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
	if err := s.database.QueryRowContext(ctx, "SELECT attempts FROM processing.scopes WHERE dataset_id=? AND scope=?", s.datasetID, scope).Scan(&attempts); err != nil {
		return
	}
	const maxBackoffExponent = 6
	exponent := min(attempts, maxBackoffExponent)
	delay := time.Second * time.Duration(int64(1)<<exponent)
	_, _ = s.database.ExecContext(ctx, "UPDATE processing.scopes SET attempts=attempts+1,retry_at_ms=?,error_code='processing_failed' WHERE dataset_id=? AND scope=?", time.Now().Add(delay).UnixMilli(), s.datasetID, scope)
}

// RecordFailure reuses the work binding; failures in one dataset cannot alter
// retry status in another. Cancellation is filtered by the engine.
func (s *Store) RecordFailure(ctx context.Context, work dataengine.Work) {
	if !s.root && work.DatasetID != s.datasetID {
		return
	}
	s.ForDataset(work.DatasetID).recordFailure(ctx, work.Root)
}
