// Package collectorstore persists normalized publication and delivery progress.
package collectorstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

type SQLRunner interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

type Store struct{ DB *sql.DB }
type SavedBatch struct {
	Batch       publication.Batch
	Request     []byte
	RequestHash string
}

// Record must run in the canonical mutation's transaction. A no-op does not
// allocate a journal sequence. The caller rolls back on every returned error.
func Record(ctx context.Context, runner SQLRunner, fact publication.Fact, nowMs int64) (bool, error) {
	if err := publication.ValidateFact(fact); err != nil {
		return false, err
	}
	if nowMs < 0 {
		return false, errors.New("invalid publication time")
	}
	hash := publication.PayloadHash(fact)
	body, err := json.Marshal(fact)
	if err != nil {
		return false, err
	}
	var oldHash, oldBody string
	err = runner.QueryRowContext(ctx, "SELECT e.payload_hash,j.payload_json FROM publication_entities e JOIN publication_journal j ON j.sequence=e.sequence WHERE e.fact_id = ?", fact.ID).Scan(&oldHash, &oldBody)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	// Envelope changes still need delivery so the server can merge source ranges.
	// They do not change the immutable contribution hash or create another fact.
	if err == nil && oldHash == hash && bytes.Equal([]byte(oldBody), body) {
		return false, nil
	}
	var rule interface{}
	var revision interface{}
	if fact.Revision != nil {
		rule = fact.Revision.Rule
		revision = fact.Revision.Value
	}
	result, err := runner.ExecContext(ctx, `INSERT INTO publication_journal (fact_id,payload_hash,payload_json,identity_version,semantics_version,source_revision_rule,source_revision_value,created_at_ms) VALUES (?,?,?,?,?,?,?,?)`, fact.ID, hash, string(body), publication.IdentityVersion, publication.SemanticsVersion, rule, revision, nowMs)
	if err != nil {
		return false, err
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return false, err
	}
	_, err = runner.ExecContext(ctx, `INSERT INTO publication_entities (fact_id,payload_hash,sequence,source_revision_rule,source_revision_value) VALUES (?,?,?,?,?) ON CONFLICT(fact_id) DO UPDATE SET payload_hash=excluded.payload_hash,sequence=excluded.sequence,source_revision_rule=excluded.source_revision_rule,source_revision_value=excluded.source_revision_value`, fact.ID, hash, sequence, rule, revision)
	return err == nil, err
}

// BindDestination refuses rebinding an existing cursor. A replacement server or
// changed endpoint needs a distinct delivery binding, whose cursor starts at zero.
func (s Store) BindDestination(ctx context.Context, id, endpoint, databaseID string) error {
	if id == "" || len(id) > publication.MaxStringBytes || databaseID == "" || len(databaseID) > publication.MaxStringBytes {
		return errors.New("invalid destination identity")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("invalid destination endpoint")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var oldEndpoint, oldDatabase string
	err = tx.QueryRowContext(ctx, "SELECT endpoint,database_id FROM publication_destinations WHERE destination_id=?", id).Scan(&oldEndpoint, &oldDatabase)
	if err == nil {
		if oldEndpoint != endpoint || oldDatabase != databaseID {
			return errors.New("destination binding conflict: configure a new binding")
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO publication_destinations (destination_id,endpoint,database_id) VALUES (?,?,?)", id, endpoint, databaseID); err != nil {
		return err
	}
	return tx.Commit()
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// PrepareBatch returns stored bytes on retry, even if newer journal entries or
// labels exist. It never advances destination progress.
func (s Store) PrepareBatch(ctx context.Context, destinationID, hostname string, nowMs int64) (*SavedBatch, error) {
	if nowMs < 0 {
		return nil, errors.New("invalid publication time")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := pendingBatch(ctx, tx, destinationID)
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var cursor int64
	var databaseID, streamID string
	if err := tx.QueryRowContext(ctx, "SELECT acknowledged_sequence,database_id FROM publication_destinations WHERE destination_id=?", destinationID).Scan(&cursor, &databaseID); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT stream_id FROM publication_state WHERE id=1").Scan(&streamID); err != nil {
		return nil, err
	}
	batchID, err := randomID()
	if err != nil {
		return nil, err
	}
	batch := publication.Batch{ProtocolVersion: publication.ProtocolVersion, IdentityVersion: publication.IdentityVersion, SemanticsVersion: publication.SemanticsVersion, DatabaseID: databaseID, StreamID: streamID, BatchID: batchID, Hostname: hostname}
	rows, err := tx.QueryContext(ctx, "SELECT sequence,payload_json FROM publication_journal WHERE sequence>? ORDER BY sequence LIMIT ?", cursor, publication.MaxEntries)
	if err != nil {
		return nil, err
	}
	var entries []publication.Entry
	for rows.Next() {
		var entry publication.Entry
		var body string
		if err := rows.Scan(&entry.Sequence, &body); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(body), &entry.Fact); err != nil {
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
	if entries[0].Sequence != cursor+1 {
		return nil, errors.New("publication journal gap")
	}
	batch.FromSequence = entries[0].Sequence
	var body []byte
	for _, entry := range entries {
		if entry.Sequence != batch.FromSequence+int64(len(batch.Entries)) {
			return nil, errors.New("publication journal gap")
		}
		batch.Entries = append(batch.Entries, entry)
		batch.ToSequence = entry.Sequence
		candidate, err := publication.EncodeBatch(batch)
		if err != nil {
			if len(batch.Entries) == 1 {
				return nil, fmt.Errorf("publication entry cannot fit batch: %w", err)
			}
			batch.Entries = batch.Entries[:len(batch.Entries)-1]
			batch.ToSequence = batch.Entries[len(batch.Entries)-1].Sequence
			break
		}
		body = candidate
	}
	if err := publication.ValidateBatch(batch); err != nil {
		return nil, err
	}
	hash := publication.RequestHash(body)
	if _, err := tx.ExecContext(ctx, `INSERT INTO publication_batches (batch_id,destination_id,stream_id,database_id,first_sequence,last_sequence,request_hash,request_bytes) VALUES (?,?,?,?,?,?,?,?)`, batch.BatchID, destinationID, streamID, databaseID, batch.FromSequence, batch.ToSequence, hash, body); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &SavedBatch{Batch: batch, Request: body, RequestHash: hash}, nil
}

func pendingBatch(ctx context.Context, runner SQLRunner, destinationID string) (*SavedBatch, error) {
	var saved SavedBatch
	if err := runner.QueryRowContext(ctx, "SELECT request_bytes,request_hash FROM publication_batches WHERE destination_id=? AND receipt_bytes IS NULL", destinationID).Scan(&saved.Request, &saved.RequestHash); err != nil {
		return nil, err
	}
	if publication.RequestHash(saved.Request) != saved.RequestHash {
		return nil, errors.New("saved publication request hash mismatch")
	}
	if err := json.Unmarshal(saved.Request, &saved.Batch); err != nil {
		return nil, err
	}
	if err := publication.ValidateBatch(saved.Batch); err != nil {
		return nil, err
	}
	return &saved, nil
}

// Ack validates the exact saved request and advances the cursor atomically with
// the receipt. Unknown network outcomes keep the pending request untouched.
func (s Store) Ack(ctx context.Context, destinationID string, receiptBytes []byte, nowMs int64) error {
	if nowMs < 0 {
		return errors.New("invalid acknowledgement time")
	}
	receipt, err := publication.DecodeReceipt(receiptBytes)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := pendingBatch(ctx, tx, destinationID)
	if errors.Is(err, sql.ErrNoRows) {
		var prior []byte
		if lookupErr := tx.QueryRowContext(ctx, "SELECT receipt_bytes FROM publication_batches WHERE destination_id=? AND batch_id=? AND receipt_bytes IS NOT NULL", destinationID, receipt.BatchID).Scan(&prior); lookupErr != nil {
			return lookupErr
		}
		if !bytes.Equal(prior, receiptBytes) {
			return errors.New("acknowledged receipt differs")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := publication.ValidateReceipt(receipt, saved.Batch, saved.Request); err != nil {
		return err
	}
	var cursor int64
	var databaseID string
	if err := tx.QueryRowContext(ctx, "SELECT acknowledged_sequence,database_id FROM publication_destinations WHERE destination_id=?", destinationID).Scan(&cursor, &databaseID); err != nil {
		return err
	}
	if cursor+1 != saved.Batch.FromSequence || databaseID != saved.Batch.DatabaseID {
		return errors.New("acknowledgement cursor binding mismatch")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE publication_batches SET receipt_bytes=?,acknowledged_at_ms=? WHERE batch_id=? AND receipt_bytes IS NULL", receiptBytes, nowMs, saved.Batch.BatchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE publication_destinations SET acknowledged_sequence=?,last_receipt_json=?,acknowledged_at_ms=? WHERE destination_id=?", saved.Batch.ToSequence, string(receiptBytes), nowMs, destinationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s Store) Pending(ctx context.Context, destinationID string) (int64, error) {
	var count int64
	err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM publication_journal WHERE sequence>(SELECT acknowledged_sequence FROM publication_destinations WHERE destination_id=?)", destinationID).Scan(&count)
	return count, err
}
