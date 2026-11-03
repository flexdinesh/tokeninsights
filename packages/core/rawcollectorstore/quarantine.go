package rawcollectorstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// Quarantine remembers deterministic capture failures, independently of a
// successful checkpoint. Signature contains only a hash of local file metadata.
type Quarantine struct {
	SourceKey     string
	Format        string
	Signature     string
	Code          string
	ParserVersion int
	Offset        int64
	RecordedAtMs  int64
}

func (s *Store) GetQuarantine(ctx context.Context, sourceKey string) (Quarantine, bool, error) {
	state := Quarantine{SourceKey: sourceKey}
	err := s.DB.QueryRowContext(ctx, "SELECT format,signature,parser_version,code,byte_offset,recorded_at_ms FROM evidence_quarantine WHERE source_key=?", sourceKey).Scan(&state.Format, &state.Signature, &state.ParserVersion, &state.Code, &state.Offset, &state.RecordedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	if err := validateQuarantine(state); err != nil {
		return state, false, err
	}
	return state, true, nil
}

func (s *Store) SaveQuarantine(ctx context.Context, state Quarantine) error {
	if err := validateQuarantine(state); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO evidence_quarantine(source_key,format,signature,parser_version,code,byte_offset,recorded_at_ms) VALUES(?,?,?,?,?,?,?) ON CONFLICT(source_key) DO UPDATE SET format=excluded.format,signature=excluded.signature,parser_version=excluded.parser_version,code=excluded.code,byte_offset=excluded.byte_offset,recorded_at_ms=excluded.recorded_at_ms`, state.SourceKey, state.Format, state.Signature, state.ParserVersion, state.Code, state.Offset, state.RecordedAtMs)
	return err
}

func (s *Store) ClearQuarantine(ctx context.Context, sourceKey string) error {
	_, err := s.DB.ExecContext(ctx, "DELETE FROM evidence_quarantine WHERE source_key=?", sourceKey)
	return err
}

func validateQuarantine(state Quarantine) error {
	if !quarantineHash(state.SourceKey) || state.ParserVersion <= 0 || state.Offset < 0 || state.RecordedAtMs < 0 {
		return errors.New("invalid_quarantine")
	}
	switch state.Format {
	case "codex-jsonl", "pi-jsonl", "claude-code-jsonl", "opencode-sqlite":
	default:
		return errors.New("invalid_quarantine")
	}
	switch state.Code {
	case "source_record_limit", "invalid_source_record":
	default:
		return errors.New("invalid_quarantine")
	}
	if !quarantineHash(state.Signature) {
		return errors.New("invalid_quarantine")
	}
	return nil
}

func quarantineHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
