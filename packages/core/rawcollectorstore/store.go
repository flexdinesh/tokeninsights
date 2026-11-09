// Package rawcollectorstore owns immutable raw outbox delivery state in SQLite.
package rawcollectorstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/collectordb"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
)

type Store struct{ DB *sql.DB }
type SavedBatch struct {
	Batch   evidence.Batch
	Request []byte
}

func Open(ctx context.Context, path string) (*Store, error) {
	database, err := collectordb.OpenEvidence(ctx, path)
	if err != nil {
		return nil, err
	}
	id, err := evidence.RandomID()
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	if _, err := database.ExecContext(ctx, "INSERT OR IGNORE INTO evidence_state(id,stream_id,extractor_version,created_at_ms) VALUES(1,?,?,?)", id, evidence.ExtractorVersion, time.Now().UnixMilli()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return &Store{DB: database}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

const (
	observationExistsSQL = "SELECT EXISTS(SELECT 1 FROM evidence_outbox WHERE observation_key=?)"
	insertObservationSQL = "INSERT INTO evidence_outbox(observation_key,harness,record_json,created_at_ms) VALUES(?,?,?,?)"
)

type recordStatements struct{ exists, insert *sql.Stmt }

func Record(ctx context.Context, tx *sql.Tx, record evidence.Record, now int64) (bool, error) {
	return recordObservation(ctx, tx, nil, record, now)
}

func recordObservation(ctx context.Context, tx *sql.Tx, statements *recordStatements, record evidence.Record, now int64) (bool, error) {
	if err := evidence.ValidateRecord(record); err != nil {
		return false, err
	}
	// Observation identity and stored bytes use the same encoding. Keep the
	// existence check before insertion: ignored AUTOINCREMENT conflicts leave
	// gaps, but delivery requires contiguous outbox sequences.
	body, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	key := evidence.Hash(body)
	var found bool
	var row *sql.Row
	if statements != nil {
		row = statements.exists.QueryRowContext(ctx, key)
	} else {
		row = tx.QueryRowContext(ctx, observationExistsSQL, key)
	}
	if err := row.Scan(&found); err != nil {
		return false, err
	}
	if found {
		return false, nil
	}
	if statements != nil {
		_, err = statements.insert.ExecContext(ctx, key, record.Harness, string(body), now)
	} else {
		_, err = tx.ExecContext(ctx, insertObservationSQL, key, record.Harness, string(body), now)
	}
	return err == nil, err
}

func (s *Store) Bind(ctx context.Context, id, endpoint, databaseID string) error {
	return s.BindDataset(ctx, id, endpoint, databaseID, "default")
}

func (s *Store) BindDataset(ctx context.Context, id, endpoint, databaseID, datasetID string) error {
	if datasetID != evidence.PersonalDatasetID {
		return errors.New("invalid_dataset")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("invalid_destination")
	}
	var oldEndpoint, oldID, oldDataset string
	err = s.DB.QueryRowContext(ctx, "SELECT endpoint,database_id,dataset_id FROM evidence_destinations WHERE destination_id=?", id).Scan(&oldEndpoint, &oldID, &oldDataset)
	if err == nil {
		if oldEndpoint != endpoint || oldID != databaseID || oldDataset != datasetID {
			return errors.New("server_database_changed")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO evidence_destinations(destination_id,endpoint,database_id,dataset_id) VALUES(?,?,?,?)", id, endpoint, databaseID, datasetID)
	return err
}

// ResolveDestination binds the personal outbox to its local token database.
// Historical endpoint strings remain immutable journal identities, never transports.
// A replacement local database deliberately replays retained evidence.
func (s *Store) ResolveDestination(ctx context.Context, endpoint, databaseID, datasetID string) (string, error) {
	if datasetID != evidence.PersonalDatasetID {
		return "", errors.New("invalid_dataset")
	}
	var id string
	err := s.DB.QueryRowContext(ctx, "SELECT destination_id FROM evidence_destinations WHERE endpoint=? AND dataset_id=? AND database_id=?", endpoint, datasetID, databaseID).Scan(&id)
	if err == nil {
		return id, s.BindDataset(ctx, id, endpoint, databaseID, datasetID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = evidence.Tuple(endpoint, datasetID, databaseID)
	return id, s.BindDataset(ctx, id, endpoint, databaseID, datasetID)
}

// Pending counts unacknowledged outbox evidence without scanning the outbox.
// Sequences are contiguous from 1: AUTOINCREMENT inserts follow duplicate
// checks, rows are immutable, and reset clears sqlite_sequence with them, so
// the maximum sequence equals the row count. Preparation rejects gaps.
func (s *Store) Pending(ctx context.Context, id string) (int64, error) {
	var result int64
	err := s.DB.QueryRowContext(ctx, "SELECT MAX(0, COALESCE((SELECT MAX(sequence) FROM evidence_outbox) - (SELECT acknowledged_sequence FROM evidence_destinations WHERE destination_id=?), 0))", id).Scan(&result)
	return result, err
}

// Draft is a validated next batch encoded outside the publication lock. It is
// never persisted directly: PrepareDraft uses it only when a fresh preparation
// would read exactly the same outbox entries for the same destination binding.
type Draft struct {
	batch    evidence.Batch
	request  []byte
	cursor   int64
	lastRead int64
	// complete means more outbox rows cannot change the encoded prefix.
	complete bool
}

// Draft encodes the batch following cursor without writing. It returns nil
// when no outbox evidence follows cursor.
func (s *Store) Draft(ctx context.Context, id string, cursor int64) (*Draft, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	header, _, err := batchHeader(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	entries, err := outboxEntries(ctx, tx, cursor)
	if err != nil {
		return nil, err
	}
	// Release the single collector connection before CPU-bound encoding.
	if err := tx.Rollback(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	batch, body, err := encodeNext(header, cursor, entries)
	if err != nil {
		return nil, err
	}
	lastRead := entries[len(entries)-1].Sequence
	return &Draft{batch: batch, request: body, cursor: cursor, lastRead: lastRead, complete: len(entries) == evidence.MaxEntries || batch.ToSequence < lastRead}, nil
}

// matches reports whether a fresh preparation at cursor would encode the same
// entries. Encoding is a greedy prefix, so it changes only when an incomplete
// draft missed rows appended after it read the outbox.
func (d *Draft) matches(ctx context.Context, tx *sql.Tx, header evidence.Batch, cursor int64) (bool, error) {
	if d == nil || d.cursor != cursor || d.batch.StreamID != header.StreamID || d.batch.DatabaseID != header.DatabaseID || d.batch.DatasetID != header.DatasetID {
		return false, nil
	}
	if d.complete {
		return true, nil
	}
	var last int64
	err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence),0) FROM evidence_outbox").Scan(&last)
	return err == nil && last == d.lastRead, err
}

func batchHeader(ctx context.Context, tx *sql.Tx, id string) (evidence.Batch, int64, error) {
	batchID, err := evidence.RandomID()
	if err != nil {
		return evidence.Batch{}, 0, err
	}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, BatchID: batchID}
	var cursor int64
	if err := tx.QueryRowContext(ctx, "SELECT acknowledged_sequence,database_id,dataset_id FROM evidence_destinations WHERE destination_id=?", id).Scan(&cursor, &batch.DatabaseID, &batch.DatasetID); err != nil {
		return batch, 0, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT stream_id FROM evidence_state WHERE id=1").Scan(&batch.StreamID); err != nil {
		return batch, 0, err
	}
	return batch, cursor, nil
}

func outboxEntries(ctx context.Context, tx *sql.Tx, cursor int64) ([]evidence.Entry, error) {
	rows, err := tx.QueryContext(ctx, "SELECT sequence,record_json FROM evidence_outbox WHERE sequence>? ORDER BY sequence LIMIT ?", cursor, evidence.MaxEntries)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var entries []evidence.Entry
	for rows.Next() {
		var entry evidence.Entry
		var body string
		if err := rows.Scan(&entry.Sequence, &body); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(body), &entry.Record); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func encodeNext(header evidence.Batch, cursor int64, entries []evidence.Entry) (evidence.Batch, []byte, error) {
	header.FromSequence = cursor + 1
	return encodeBatchPrefix(header, entries)
}

func (s *Store) Prepare(ctx context.Context, id string) (*SavedBatch, error) {
	return s.PrepareDraft(ctx, id, nil)
}

// PrepareDraft returns the unacknowledged batch, or persists the next one.
// A matching draft replaces only the in-transaction read and encoding.
func (s *Store) PrepareDraft(ctx context.Context, id string, draft *Draft) (*SavedBatch, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := pending(ctx, tx, id)
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	batch, cursor, err := batchHeader(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var body []byte
	reuse, err := draft.matches(ctx, tx, batch, cursor)
	if err != nil {
		return nil, err
	}
	if reuse {
		batch, body = draft.batch, draft.request
	} else {
		entries, err := outboxEntries(ctx, tx, cursor)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			return nil, nil
		}
		batch, body, err = encodeNext(batch, cursor, entries)
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,dataset_id,protocol_version,first_sequence,last_sequence,request_hash,request_bytes) VALUES(?,?,?,?,?,?,?,?,?,?)", batch.BatchID, id, batch.StreamID, batch.DatabaseID, batch.DatasetID, batch.ProtocolVersion, batch.FromSequence, batch.ToSequence, evidence.Hash(body), body)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &SavedBatch{Batch: batch, Request: body}, nil
}

func pending(ctx context.Context, tx *sql.Tx, id string) (*SavedBatch, error) {
	return pendingPrepared(ctx, tx, id, nil)
}

// pendingPrepared reuses a batch validated in this process only when the
// stored request bytes are identical to it; anything else is decoded in full.
func pendingPrepared(ctx context.Context, tx *sql.Tx, id string, prepared *SavedBatch) (*SavedBatch, error) {
	saved := &SavedBatch{}
	var hash, datasetID string
	var protocol int
	err := tx.QueryRowContext(ctx, "SELECT request_bytes,request_hash,dataset_id,protocol_version FROM evidence_batches WHERE destination_id=? AND receipt_bytes IS NULL", id).Scan(&saved.Request, &hash, &datasetID, &protocol)
	if err != nil {
		return nil, err
	}
	if hash != evidence.Hash(saved.Request) {
		return nil, errors.New("saved_request_corrupt")
	}
	if prepared != nil && bytes.Equal(saved.Request, prepared.Request) {
		saved.Batch = prepared.Batch
	} else {
		saved.Batch, err = evidence.DecodeBatch(saved.Request)
	}
	if err == nil && (saved.Batch.ProtocolVersion != protocol || saved.Batch.DatasetID != datasetID) {
		return nil, errors.New("saved_request_binding_corrupt")
	}
	return saved, err
}

func (s *Store) Ack(ctx context.Context, id string, body []byte) error {
	return s.AckPrepared(ctx, id, nil, body)
}

// AckPrepared acknowledges the batch PrepareDraft returned in this process.
// The stored request must equal prepared bytes to skip decoding it again.
func (s *Store) AckPrepared(ctx context.Context, id string, prepared *SavedBatch, body []byte) error {
	var response evidence.Response
	if err := evidence.StrictDecode(body, &response); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := pendingPrepared(ctx, tx, id, prepared)
	if errors.Is(err, sql.ErrNoRows) {
		var old []byte
		if err := tx.QueryRowContext(ctx, "SELECT receipt_bytes FROM evidence_batches WHERE destination_id=? AND batch_id=?", id, response.Receipt.BatchID).Scan(&old); err != nil {
			return err
		}
		var previous evidence.Response
		if json.Unmarshal(old, &previous) != nil || previous.Receipt != response.Receipt {
			return errors.New("receipt_mismatch")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := evidence.ValidateReceipt(saved.Batch, saved.Request, response); err != nil {
		return err
	}
	var cursor int64
	if err := tx.QueryRowContext(ctx, "SELECT acknowledged_sequence FROM evidence_destinations WHERE destination_id=?", id).Scan(&cursor); err != nil {
		return err
	}
	if cursor+1 != saved.Batch.FromSequence {
		return errors.New("acknowledgement_gap")
	}
	accepted, _ := json.Marshal(evidence.Response{Receipt: response.Receipt})
	_, err = tx.ExecContext(ctx, "UPDATE evidence_batches SET receipt_bytes=?,acknowledged_at_ms=? WHERE batch_id=?", accepted, time.Now().UnixMilli(), saved.Batch.BatchID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE evidence_destinations SET acknowledged_sequence=?,acknowledged_at_ms=? WHERE destination_id=?", saved.Batch.ToSequence, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
