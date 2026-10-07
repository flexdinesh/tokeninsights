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
	if err := db.UpgradeEvidence(ctx, path); err != nil {
		return nil, err
	}
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

func Record(ctx context.Context, tx *sql.Tx, record evidence.Record, now int64) (bool, error) {
	if err := evidence.ValidateRecord(record); err != nil {
		return false, err
	}
	key := evidence.ObservationKey(record)
	var found bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM evidence_outbox WHERE observation_key=?)", key).Scan(&found); err != nil {
		return false, err
	}
	if found {
		return false, nil
	}
	body, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO evidence_outbox(observation_key,harness,record_json,created_at_ms) VALUES(?,?,?,?)", key, record.Harness, string(body), now)
	return err == nil, err
}

func (s *Store) Bind(ctx context.Context, id, endpoint, databaseID string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("invalid_destination")
	}
	var oldEndpoint, oldID string
	err = s.DB.QueryRowContext(ctx, "SELECT endpoint,database_id FROM evidence_destinations WHERE destination_id=?", id).Scan(&oldEndpoint, &oldID)
	if err == nil {
		if oldEndpoint != endpoint || oldID != databaseID {
			return errors.New("server_database_changed")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO evidence_destinations(destination_id,endpoint,database_id) VALUES(?,?,?)", id, endpoint, databaseID)
	return err
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
	if err := tx.QueryRowContext(ctx, "SELECT acknowledged_sequence,database_id FROM evidence_destinations WHERE destination_id=?", id).Scan(&cursor, &batch.DatabaseID); err != nil {
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
	var body []byte
	for _, entry := range entries {
		if entry.Sequence != batch.FromSequence+int64(len(batch.Entries)) {
			return nil, errors.New("outbox_gap")
		}
		batch.Entries = append(batch.Entries, entry)
		batch.ToSequence = entry.Sequence
		candidate, err := json.Marshal(batch)
		if err != nil {
			return nil, err
		}
		if len(candidate) > evidence.MaxBodyBytes {
			if len(batch.Entries) == 1 {
				return nil, errors.New("record_exceeds_batch_limit")
			}
			batch.Entries = batch.Entries[:len(batch.Entries)-1]
			batch.ToSequence = batch.Entries[len(batch.Entries)-1].Sequence
			break
		}
		body = candidate
	}
	if _, err := evidence.DecodeBatch(body); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,first_sequence,last_sequence,request_hash,request_bytes) VALUES(?,?,?,?,?,?,?,?)", batch.BatchID, id, batch.StreamID, batch.DatabaseID, batch.FromSequence, batch.ToSequence, evidence.Hash(body), body)
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
	var hash string
	err := tx.QueryRowContext(ctx, "SELECT request_bytes,request_hash FROM evidence_batches WHERE destination_id=? AND receipt_bytes IS NULL", id).Scan(&saved.Request, &hash)
	if err != nil {
		return nil, err
	}
	if hash != evidence.Hash(saved.Request) {
		return nil, errors.New("saved_request_corrupt")
	}
	saved.Batch, err = evidence.DecodeBatch(saved.Request)
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
