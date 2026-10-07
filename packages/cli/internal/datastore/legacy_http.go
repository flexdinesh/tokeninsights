package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func (s *Store) mountLegacy(mux *http.ServeMux) {
	mux.HandleFunc("GET "+LegacyIngestionPrefix+"capabilities", func(w http.ResponseWriter, r *http.Request) {
		m, err := s.Metadata(r.Context())
		if err != nil {
			respond(w, 503, publication.ErrorResponse{Code: "unavailable", Stage: "database"})
			return
		}
		respond(w, 200, publication.Capabilities{ProtocolVersion: 1, IdentityVersion: 1, SemanticsVersion: 1, DatabaseID: m.DatabaseID, MaxBodyBytes: publication.MaxBodyBytes, MaxEntries: publication.MaxEntries, MaxStringBytes: publication.MaxStringBytes, MaxInteger: publication.SafeInteger})
	})
	mux.HandleFunc("POST "+LegacyIngestionPrefix+"batches", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, publication.MaxBodyBytes+1))
		if err != nil {
			respond(w, 400, publication.ErrorResponse{Code: "invalid_request", Stage: "validation"})
			return
		}
		if len(body) > publication.MaxBodyBytes {
			respond(w, 413, publication.ErrorResponse{Code: "body_limit", Stage: "validation"})
			return
		}
		receipt, err := s.AcceptLegacy(r.Context(), body)
		if err != nil {
			var admission *AdmissionError
			code := "transaction_failed"
			status := 503
			if errors.As(err, &admission) {
				code = admission.Code
				status = 409
				if code == "invalid_request" {
					status = 400
				}
			}
			respond(w, status, publication.ErrorResponse{Code: code, Stage: "admission"})
			return
		}
		respond(w, 200, receipt)
	})
}

func (s *Store) AcceptLegacy(ctx context.Context, body []byte) (publication.Receipt, error) {
	if err := s.checkFile(); err != nil {
		return publication.Receipt{}, err
	}
	batch, err := publication.DecodeBatch(body)
	if err != nil {
		return publication.Receipt{}, reject("invalid_request")
	}
	s.writer.Lock()
	defer s.writer.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return publication.Receipt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err := ReadMetadata(ctx, tx)
	if err != nil {
		return publication.Receipt{}, err
	}
	if batch.DatabaseID != m.DatabaseID {
		return publication.Receipt{}, reject("database_mismatch")
	}
	var hash, old string
	err = tx.QueryRowContext(ctx, "SELECT request_hash,receipt_json FROM ingestion.legacy_receipts WHERE stream_id=? AND batch_id=?", batch.StreamID, batch.BatchID).Scan(&hash, &old)
	if err == nil {
		if hash != publication.RequestHash(body) {
			return publication.Receipt{}, reject("batch_conflict")
		}
		var receipt publication.Receipt
		err = json.Unmarshal([]byte(old), &receipt)
		return receipt, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return publication.Receipt{}, err
	}
	receipt := publication.Receipt{DatabaseID: m.DatabaseID, StreamID: batch.StreamID, BatchID: batch.BatchID, FromSequence: batch.FromSequence, ToSequence: batch.ToSequence, RequestHash: publication.RequestHash(body), CommittedAtMs: time.Now().UnixMilli(), Revision: m.Revision}
	for _, entry := range batch.Entries {
		fact := entry.Fact
		var previousJSON string
		err := tx.QueryRowContext(ctx, "SELECT payload_json FROM analytics.legacy WHERE fact_id=?", fact.ID).Scan(&previousJSON)
		if err == nil {
			var previous publication.Fact
			if err := json.Unmarshal([]byte(previousJSON), &previous); err != nil {
				return receipt, err
			}
			if legacyComponentsEqual(previous, fact) && previous.OccurredAtMs == fact.OccurredAtMs {
				receipt.Noop++
				continue
			}
			if previous.Revision == nil || fact.Revision == nil || previous.Revision.Rule != fact.Revision.Rule {
				return receipt, reject("fact_conflict")
			}
			if fact.Revision.Value < previous.Revision.Value {
				receipt.Noop++
				continue
			}
			if fact.Revision.Value == previous.Revision.Value {
				return receipt, reject("revision_conflict")
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM analytics.legacy WHERE fact_id=?", fact.ID); err != nil {
				return receipt, err
			}
			receipt.Updated++
		} else if errors.Is(err, sql.ErrNoRows) {
			receipt.Inserted++
		} else {
			return receipt, err
		}
		if err := insertFact(ctx, tx, "analytics.legacy", fact, 0, m.Revision, "", ""); err != nil {
			return receipt, err
		}
	}
	if receipt.Inserted+receipt.Updated > 0 {
		if m.Revision >= publication.SafeInteger {
			return receipt, reject("revision_limit")
		}
		receipt.Revision++
	}
	encoded, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, "INSERT INTO ingestion.legacy_receipts VALUES(?,?,?,?)", batch.StreamID, batch.BatchID, receipt.RequestHash, string(encoded)); err != nil {
		return receipt, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ingestion.metadata SET revision=?,last_ingestion_at_ms=? WHERE id=1", receipt.Revision, receipt.CommittedAtMs); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit()
}
func legacyComponentsEqual(left, right publication.Fact) bool {
	return left.Harness == right.Harness && left.Session.ID == right.Session.ID && left.InputTokens == right.InputTokens && left.OutputTokens == right.OutputTokens && left.ReasoningTokens == right.ReasoningTokens && left.CacheReadTokens == right.CacheReadTokens && left.CacheWriteTokens == right.CacheWriteTokens && left.TotalTokens == right.TotalTokens && left.Countable == right.Countable && left.Provider == right.Provider && left.ProviderSource == right.ProviderSource && left.Model == right.Model
}
