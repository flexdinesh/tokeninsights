// Package ingestion applies normalized, self-contained publications atomically.
package ingestion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

const MaxConcurrentRequests = 4

type Core struct {
	store     *serverstore.Store
	admission chan struct{}
}
type Error struct {
	Status int
	Code   string
	Stage  string
}

func (e *Error) Error() string { return e.Stage + ": " + e.Code }
func failure(status int, code, stage string) error {
	return &Error{Status: status, Code: code, Stage: stage}
}
func NewCore(store *serverstore.Store) *Core {
	return &Core{store: store, admission: make(chan struct{}, MaxConcurrentRequests)}
}

func (c *Core) Ingest(ctx context.Context, body []byte) (publication.Receipt, error) {
	select {
	case c.admission <- struct{}{}:
		defer func() { <-c.admission }()
	default:
		return publication.Receipt{}, failure(http.StatusServiceUnavailable, "busy", "admission")
	}
	return c.ingest(ctx, body)
}

func (c *Core) ingest(ctx context.Context, body []byte) (publication.Receipt, error) {
	var receipt publication.Receipt
	if len(body) > publication.MaxBodyBytes {
		return receipt, failure(http.StatusRequestEntityTooLarge, "body_limit", "validation")
	}
	batch, err := publication.DecodeBatch(body)
	if err != nil {
		status := http.StatusBadRequest
		code := "invalid_request"
		var validation *publication.ValidationError
		if errors.As(err, &validation) {
			code = validation.Code
			switch validation.Code {
			case "incompatible":
				status = http.StatusUnprocessableEntity
			case "too_large":
				status = http.StatusRequestEntityTooLarge
			}
		}
		return receipt, failure(status, code, "validation")
	}
	tx, err := c.store.SQL().BeginTx(ctx, nil)
	if err != nil {
		return receipt, databaseError(err)
	}
	defer func() { _ = tx.Rollback() }()
	metadata, err := serverstore.ReadMetadata(ctx, tx)
	if err != nil {
		return receipt, databaseError(err)
	}
	if batch.DatabaseID != metadata.DatabaseID {
		return receipt, failure(http.StatusConflict, "database_mismatch", "identity")
	}
	requestHash := publication.RequestHash(body)
	var oldHash, oldReceipt string
	err = tx.QueryRowContext(ctx, `SELECT request_hash, receipt_json FROM ingestion_receipts WHERE stream_id=? AND batch_id=?`, batch.StreamID, batch.BatchID).Scan(&oldHash, &oldReceipt)
	if err == nil {
		if oldHash != requestHash {
			return receipt, failure(http.StatusConflict, "batch_conflict", "receipt")
		}
		if err := json.Unmarshal([]byte(oldReceipt), &receipt); err != nil {
			return receipt, databaseError(err)
		}
		return receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return receipt, databaseError(err)
	}
	receipt = publication.Receipt{DatabaseID: metadata.DatabaseID, StreamID: batch.StreamID, BatchID: batch.BatchID, FromSequence: batch.FromSequence, ToSequence: batch.ToSequence, RequestHash: requestHash, CommittedAtMs: time.Now().UnixMilli(), Revision: metadata.Revision}
	changed := false
	for _, entry := range batch.Entries {
		result, refsChanged, err := applyFact(ctx, tx, entry.Fact)
		if err != nil {
			return publication.Receipt{}, err
		}
		changed = changed || refsChanged || result != "noop"
		switch result {
		case "inserted":
			receipt.Inserted++
		case "updated":
			receipt.Updated++
		default:
			receipt.Noop++
		}
	}
	if err := validateAggregate(ctx, tx); err != nil {
		return publication.Receipt{}, err
	}
	if changed {
		if receipt.Revision >= publication.SafeInteger {
			return publication.Receipt{}, failure(http.StatusBadRequest, "revision_limit", "validation")
		}
		receipt.Revision++
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return publication.Receipt{}, databaseError(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ingestion_receipts(stream_id,batch_id,request_hash,first_sequence,last_sequence,receipt_json,committed_at_ms,inserted_count,updated_count,noop_count,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, batch.StreamID, batch.BatchID, requestHash, batch.FromSequence, batch.ToSequence, string(encoded), receipt.CommittedAtMs, receipt.Inserted, receipt.Updated, receipt.Noop, receipt.Revision)
	if err != nil {
		return publication.Receipt{}, databaseError(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ingestion_producers(stream_id,hostname,last_ingestion_at_ms) VALUES(?,?,?) ON CONFLICT(stream_id) DO UPDATE SET hostname=excluded.hostname,last_ingestion_at_ms=excluded.last_ingestion_at_ms`, batch.StreamID, batch.Hostname, receipt.CommittedAtMs)
	if err != nil {
		return publication.Receipt{}, databaseError(err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE server_metadata SET revision=?,last_ingestion_at_ms=? WHERE id=1`, receipt.Revision, receipt.CommittedAtMs)
	if err != nil {
		return publication.Receipt{}, databaseError(err)
	}
	if err := tx.Commit(); err != nil {
		return publication.Receipt{}, databaseError(err)
	}
	return receipt, nil
}

func databaseError(err error) error {
	if strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked") {
		return failure(http.StatusServiceUnavailable, "busy", "database")
	}
	return failure(http.StatusInternalServerError, "transaction_failed", "database")
}

func applyFact(ctx context.Context, tx *sql.Tx, f publication.Fact) (string, bool, error) {
	hash := publication.PayloadHash(f)
	var oldHash, rule string
	var value int64
	err := tx.QueryRowContext(ctx, `SELECT payload_hash,revision_rule,revision_value FROM canonical_token_usage WHERE semantic_key=?`, f.ID).Scan(&oldHash, &rule, &value)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, databaseError(err)
	}
	result := "inserted"
	stale := false
	if exists {
		result = "noop"
		if oldHash != hash {
			if f.Revision == nil || rule != f.Revision.Rule {
				return "", false, failure(http.StatusConflict, "fact_conflict", "facts")
			}
			if f.Revision.Value == value {
				return "", false, failure(http.StatusConflict, "revision_conflict", "facts")
			}
			if f.Revision.Value > value {
				result = "updated"
			} else {
				stale = true
			}
		}
	}
	references := f
	if stale {
		references.Location = nil
	}
	sessionID, messageID, locationID, refsChanged, err := mergeReferences(ctx, tx, references)
	if err != nil {
		return "", false, err
	}
	if result == "noop" {
		return result, refsChanged, nil
	}
	rule = ""
	value = 0
	if f.Revision != nil {
		rule = f.Revision.Rule
		value = f.Revision.Value
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO canonical_token_usage(semantic_key,recorded_at_ms,harness,session_id,message_id,provider,provider_source,model,usage_scope,quality,is_countable,input_tokens,output_tokens,reasoning_tokens,cache_read_tokens,cache_write_tokens,total_tokens,location_id,payload_hash,revision_rule,revision_value) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(semantic_key) DO UPDATE SET recorded_at_ms=excluded.recorded_at_ms,provider=excluded.provider,provider_source=excluded.provider_source,model=excluded.model,quality=excluded.quality,is_countable=excluded.is_countable,input_tokens=excluded.input_tokens,output_tokens=excluded.output_tokens,reasoning_tokens=excluded.reasoning_tokens,cache_read_tokens=excluded.cache_read_tokens,cache_write_tokens=excluded.cache_write_tokens,total_tokens=excluded.total_tokens,location_id=excluded.location_id,payload_hash=excluded.payload_hash,revision_rule=excluded.revision_rule,revision_value=excluded.revision_value`, f.ID, f.OccurredAtMs, f.Harness, sessionID, messageID, f.Provider, f.ProviderSource, f.Model, f.UsageScope, f.Quality, f.Countable, f.InputTokens, f.OutputTokens, f.ReasoningTokens, f.CacheReadTokens, f.CacheWriteTokens, f.TotalTokens, locationID, hash, rule, value)
	if err != nil {
		return "", false, databaseError(err)
	}
	return result, refsChanged, nil
}

func mergeReferences(ctx context.Context, tx *sql.Tx, f publication.Fact) (int64, interface{}, interface{}, bool, error) {
	var sessionID, first, last int64
	var harness, native string
	err := tx.QueryRowContext(ctx, `SELECT id,harness,session_id,first_seen_at_ms,last_seen_at_ms FROM canonical_sessions WHERE semantic_key=?`, f.Session.ID).Scan(&sessionID, &harness, &native, &first, &last)
	changed := false
	if errors.Is(err, sql.ErrNoRows) {
		result, e := tx.ExecContext(ctx, `INSERT INTO canonical_sessions(semantic_key,harness,session_id,first_seen_at_ms,last_seen_at_ms) VALUES(?,?,?,?,?)`, f.Session.ID, f.Harness, f.Session.NativeID, f.Session.FirstOccurredAtMs, f.Session.LastOccurredAtMs)
		if e != nil {
			return 0, nil, nil, false, databaseError(e)
		}
		sessionID, e = result.LastInsertId()
		if e != nil {
			return 0, nil, nil, false, databaseError(e)
		}
		changed = true
	} else if err != nil {
		return 0, nil, nil, false, databaseError(err)
	} else {
		if harness != f.Harness || native != f.Session.NativeID {
			return 0, nil, nil, false, failure(http.StatusConflict, "reference_conflict", "references")
		}
		newFirst := min(first, f.Session.FirstOccurredAtMs)
		newLast := max(last, f.Session.LastOccurredAtMs)
		if newFirst != first || newLast != last {
			if _, e := tx.ExecContext(ctx, `UPDATE canonical_sessions SET first_seen_at_ms=?,last_seen_at_ms=? WHERE id=?`, newFirst, newLast, sessionID); e != nil {
				return 0, nil, nil, false, databaseError(e)
			}
			changed = true
		}
	}
	var messageID interface{}
	if f.Message != nil {
		var id, owner, occurred int64
		err = tx.QueryRowContext(ctx, `SELECT id,session_id,harness,harness_message_id,occurred_at_ms FROM canonical_messages WHERE semantic_key=?`, f.Message.ID).Scan(&id, &owner, &harness, &native, &occurred)
		if errors.Is(err, sql.ErrNoRows) {
			r, e := tx.ExecContext(ctx, `INSERT INTO canonical_messages(semantic_key,session_id,harness,harness_message_id,occurred_at_ms) VALUES(?,?,?,?,?)`, f.Message.ID, sessionID, f.Harness, f.Message.NativeID, f.Message.OccurredAtMs)
			if e != nil {
				return 0, nil, nil, false, databaseError(e)
			}
			id, e = r.LastInsertId()
			if e != nil {
				return 0, nil, nil, false, databaseError(e)
			}
			changed = true
		} else if err != nil {
			return 0, nil, nil, false, databaseError(err)
		} else {
			if owner != sessionID || harness != f.Harness || native != f.Message.NativeID {
				return 0, nil, nil, false, failure(http.StatusConflict, "reference_conflict", "references")
			}
			if f.Message.OccurredAtMs < occurred {
				if _, e := tx.ExecContext(ctx, `UPDATE canonical_messages SET occurred_at_ms=? WHERE id=?`, f.Message.OccurredAtMs, id); e != nil {
					return 0, nil, nil, false, databaseError(e)
				}
				changed = true
			}
		}
		messageID = id
	}
	var locationID interface{}
	if f.Location != nil {
		l := f.Location
		var id int64
		var dk, dn, rk, rn, rs string
		err = tx.QueryRowContext(ctx, `SELECT id,COALESCE(directory_key,''),COALESCE(directory_name,''),COALESCE(repository_key,''),COALESCE(repository_name,''),COALESCE(repository_source,'') FROM usage_locations WHERE semantic_key=?`, l.ID).Scan(&id, &dk, &dn, &rk, &rn, &rs)
		if errors.Is(err, sql.ErrNoRows) {
			r, e := tx.ExecContext(ctx, `INSERT INTO usage_locations(semantic_key,directory_key,directory_name,repository_key,repository_name,repository_source) VALUES(?,?,?,?,?,?)`, l.ID, optionalLocationString(l.DirectoryKey), optionalLocationString(l.DirectoryName), optionalLocationString(l.RepositoryKey), optionalLocationString(l.RepositoryName), optionalLocationString(l.RepositorySource))
			if e != nil {
				return 0, nil, nil, false, databaseError(e)
			}
			id, e = r.LastInsertId()
			if e != nil {
				return 0, nil, nil, false, databaseError(e)
			}
			changed = true
		} else if err != nil {
			return 0, nil, nil, false, databaseError(err)
		} else if dk != l.DirectoryKey || dn != l.DirectoryName || rk != l.RepositoryKey || rn != l.RepositoryName {
			return 0, nil, nil, false, failure(http.StatusConflict, "reference_conflict", "references")
		} else if publication.RepositorySourceRank(l.RepositorySource) > publication.RepositorySourceRank(rs) {
			if _, e := tx.ExecContext(ctx, `UPDATE usage_locations SET repository_source=? WHERE id=?`, l.RepositorySource, id); e != nil {
				return 0, nil, nil, false, databaseError(e)
			}
			changed = true
		}
		locationID = id
	}
	return sessionID, messageID, locationID, changed, nil
}

// Missing location metadata stays NULL for canonical unknown grouping/filtering.
func optionalLocationString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

// Scan rows instead of SQLite SUM: SQLite can overflow before the safe-domain check.
func validateAggregate(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT input_tokens,output_tokens,reasoning_tokens,cache_read_tokens,cache_write_tokens,total_tokens FROM canonical_token_usage WHERE is_countable=1`)
	if err != nil {
		return databaseError(err)
	}
	defer func() { _ = rows.Close() }()
	var totals [6]int64
	for rows.Next() {
		var counts [6]int64
		if err := rows.Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4], &counts[5]); err != nil {
			return databaseError(err)
		}
		for i, count := range counts {
			if count < 0 || count > publication.SafeInteger-totals[i] {
				return failure(http.StatusBadRequest, "aggregate_limit", "validation")
			}
			totals[i] += count
		}
	}
	if err := rows.Err(); err != nil {
		return databaseError(err)
	}
	return nil
}
