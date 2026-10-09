// Package rawcollectorstore owns immutable raw outbox delivery state in SQLite.
package rawcollectorstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

type Store struct{ DB *sql.DB }
type SavedBatch struct {
	Batch   evidence.Batch
	Request []byte
}

func Open(ctx context.Context, path string) (*Store, error) {
	database, err := db.OpenEvidence(ctx, path)
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
	if !evidence.ValidDatasetID(datasetID) {
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

// ResolveDestination binds endpoint, database and dataset; tokens never identify cursors.
// Remote replacement rejects, while local replacement deliberately replays history.
func (s *Store) ResolveDestination(ctx context.Context, endpoint, databaseID, datasetID string, local bool) (string, error) {
	if !local {
		var conflicting bool
		if err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM evidence_destinations WHERE endpoint=? AND dataset_id=? AND database_id<>?)", endpoint, datasetID, databaseID).Scan(&conflicting); err != nil {
			return "", err
		}
		if conflicting {
			return "", errors.New("server_database_changed")
		}
	}
	query := "SELECT destination_id,database_id FROM evidence_destinations WHERE endpoint=? AND dataset_id=?"
	args := []interface{}{endpoint, datasetID}
	if local {
		query += " AND database_id=?"
		args = append(args, databaseID)
	}
	var id, previous string
	err := s.DB.QueryRowContext(ctx, query, args...).Scan(&id, &previous)
	if err == nil {
		if previous != databaseID {
			return "", errors.New("server_database_changed")
		}
		return id, s.BindDataset(ctx, id, endpoint, databaseID, datasetID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	identity := []interface{}{endpoint, datasetID}
	if local {
		identity = append(identity, databaseID)
	}
	id = evidence.Tuple(identity...)
	return id, s.BindDataset(ctx, id, endpoint, databaseID, datasetID)
}

func (s *Store) Pending(ctx context.Context, id string) (int64, error) {
	var result int64
	err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM evidence_outbox WHERE sequence > (SELECT acknowledged_sequence FROM evidence_destinations WHERE destination_id=?)", id).Scan(&result)
	return result, err
}

func (s *Store) Prepare(ctx context.Context, id string) (*SavedBatch, error) {
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
	batchID, err := evidence.RandomID()
	if err != nil {
		return nil, err
	}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, BatchID: batchID}
	var cursor int64
	if err := tx.QueryRowContext(ctx, "SELECT acknowledged_sequence,database_id,dataset_id FROM evidence_destinations WHERE destination_id=?", id).Scan(&cursor, &batch.DatabaseID, &batch.DatasetID); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT stream_id FROM evidence_state WHERE id=1").Scan(&batch.StreamID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT sequence,record_json FROM evidence_outbox WHERE sequence>? ORDER BY sequence LIMIT ?", cursor, evidence.MaxEntries)
	if err != nil {
		return nil, err
	}
	var entries []evidence.Entry
	for rows.Next() {
		var entry evidence.Entry
		var body string
		if err := rows.Scan(&entry.Sequence, &body); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(body), &entry.Record); err != nil {
			_ = rows.Close()
			return nil, err
		}
		entries = append(entries, entry)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	batch.FromSequence = cursor + 1
	batch, body, err := encodeBatchPrefix(batch, entries)
	if err != nil {
		return nil, err
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
	saved.Batch, err = evidence.DecodeBatch(saved.Request)
	if err == nil && (saved.Batch.ProtocolVersion != protocol || saved.Batch.DatasetID != datasetID) {
		return nil, errors.New("saved_request_binding_corrupt")
	}
	return saved, err
}

func (s *Store) Ack(ctx context.Context, id string, body []byte) error {
	var response evidence.Response
	if err := evidence.StrictDecode(body, &response); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := pending(ctx, tx, id)
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
