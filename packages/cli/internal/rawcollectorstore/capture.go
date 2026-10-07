package rawcollectorstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// Capture commits sanitized observations and source continuity together. The
// transaction never escapes into source parsing or native snapshot readers.
type Capture struct{ tx *sql.Tx }

type Checkpoint struct {
	SourceKey   string
	SourceID    string
	Lineage     string
	Format      string
	Offset      int64
	Ordinal     int64
	Prefix      string
	Context     json.RawMessage
	UpdatedAtMs int64
}

func (s *Store) BeginCapture(ctx context.Context) (*Capture, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Capture{tx: tx}, nil
}

func (c *Capture) Checkpoint(ctx context.Context, key, format string) (Checkpoint, bool, error) {
	state := Checkpoint{SourceKey: key, Format: format}
	var body string
	err := c.tx.QueryRowContext(ctx, "SELECT source_id,lineage,byte_offset,ordinal,prefix_hash,context_json,updated_at_ms FROM evidence_sources WHERE source_key=? AND format=? AND extractor_version=?", key, format, evidence.ExtractorVersion).Scan(&state.SourceID, &state.Lineage, &state.Offset, &state.Ordinal, &state.Prefix, &body, &state.UpdatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	state.Context = json.RawMessage(body)
	return state, true, nil
}

func (c *Capture) SaveCheckpoint(ctx context.Context, state Checkpoint) error {
	if !json.Valid(state.Context) {
		return errors.New("invalid_checkpoint_context")
	}
	_, err := c.tx.ExecContext(ctx, `INSERT INTO evidence_sources(source_key,source_id,lineage,format,extractor_version,byte_offset,ordinal,prefix_hash,context_json,updated_at_ms) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(source_key) DO UPDATE SET source_id=excluded.source_id,lineage=excluded.lineage,format=excluded.format,extractor_version=excluded.extractor_version,byte_offset=excluded.byte_offset,ordinal=excluded.ordinal,prefix_hash=excluded.prefix_hash,context_json=excluded.context_json,updated_at_ms=excluded.updated_at_ms`, state.SourceKey, state.SourceID, state.Lineage, state.Format, evidence.ExtractorVersion, state.Offset, state.Ordinal, state.Prefix, string(state.Context), state.UpdatedAtMs)
	return err
}

func (c *Capture) Record(ctx context.Context, record evidence.Record, now int64) (bool, error) {
	return Record(ctx, c.tx, record, now)
}
func (c *Capture) Commit() error   { return c.tx.Commit() }
func (c *Capture) Rollback() error { return c.tx.Rollback() }
