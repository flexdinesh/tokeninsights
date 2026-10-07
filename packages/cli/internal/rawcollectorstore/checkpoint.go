package rawcollectorstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// Checkpoint reads continuity without holding a writer transaction while sources
// parse. The capture writer must revalidate it before committing prepared work.
func (s *Store) Checkpoint(ctx context.Context, key, format string) (Checkpoint, bool, error) {
	state := Checkpoint{SourceKey: key, Format: format}
	var body string
	err := s.DB.QueryRowContext(ctx, "SELECT source_id,lineage,byte_offset,ordinal,prefix_hash,context_json,updated_at_ms FROM evidence_sources WHERE source_key=? AND format=? AND extractor_version=?", key, format, evidence.ExtractorVersion).Scan(&state.SourceID, &state.Lineage, &state.Offset, &state.Ordinal, &state.Prefix, &body, &state.UpdatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	state.Context = json.RawMessage(body)
	return state, true, nil
}
