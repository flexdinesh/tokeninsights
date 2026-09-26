package pipeline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
)

const sourceCursorHashWindow = 4096
const piByteCursorKind = "pi-jsonl-byte-v1"

type byteCursorAdapter interface {
	ParseFrom(context.Context, Source, SyncOptions, int64) ([]RawTokenFact, []Diagnostic, bool, error)
}

type sourceCursorState struct {
	collector, parser, kind    string
	offset, mtimeMs, sizeBytes int64
	prefixHash, boundaryHash   string
	locationFingerprint        string
}

func planPiCursor(ctx context.Context, runner sqlRunner, source Source, options SyncOptions, metadata sourceRefreshMetadata, ok bool) (int64, bool, error) {
	if !ok || options.FullRefresh || source.AlwaysRefresh || source.Harness != HarnessPi {
		return 0, false, nil
	}
	var state sourceCursorState
	err := runner.QueryRowContext(ctx, `
		SELECT collector, parser, cursor_kind, byte_offset, source_mtime_ms, source_size_bytes, prefix_hash, boundary_hash, location_fingerprint
		FROM source_cursor_state WHERE harness = ? AND source_kind = ? AND source_state_key = ?
	`, source.Harness, source.Kind, metadata.stateKey).Scan(&state.collector, &state.parser, &state.kind, &state.offset, &state.mtimeMs, &state.sizeBytes, &state.prefixHash, &state.boundaryHash, &state.locationFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if state.collector != options.Collector || state.parser != options.Parser || state.kind != piByteCursorKind || state.offset == 0 || state.offset != state.sizeBytes || metadata.sizeBytes < state.offset {
		return 0, false, nil
	}
	refresh, found, err := loadSourceRefreshState(ctx, runner, source, metadata.stateKey)
	if err != nil {
		return 0, false, err
	}
	if !found || refresh.collector != state.collector || refresh.parser != state.parser || refresh.sourceMtimeMs != state.mtimeMs || refresh.sourceSizeBytes != state.sizeBytes {
		return 0, false, nil
	}
	prefix, boundary, terminated, err := sourceCursorHashes(source.Path, state.offset)
	if err != nil || !terminated || prefix != state.prefixHash || boundary != state.boundaryHash {
		return 0, false, nil
	}
	locationFingerprint, validHeader := piCursorLocationFingerprint(ctx, source, options)
	if !validHeader || locationFingerprint != state.locationFingerprint {
		return 0, false, nil
	}
	if metadata.sizeBytes == state.offset {
		if metadata.mtimeMs == state.mtimeMs {
			return state.offset, true, nil
		}
		return 0, false, nil
	}
	return state.offset, false, nil
}

func storePiCursor(ctx context.Context, runner sqlRunner, source Source, options SyncOptions, metadata sourceRefreshMetadata, ok, eligible bool) error {
	if !ok || source.Harness != HarnessPi {
		return nil
	}
	if !eligible {
		_, err := runner.ExecContext(ctx, "DELETE FROM source_cursor_state WHERE harness = ? AND source_kind = ? AND source_state_key = ?", source.Harness, source.Kind, metadata.stateKey)
		return err
	}
	current, currentOK := sourceRefreshMetadataFor(source)
	if !currentOK || current.mtimeMs != metadata.mtimeMs || current.sizeBytes != metadata.sizeBytes {
		return nil
	}
	prefix, boundary, terminated, err := sourceCursorHashes(source.Path, metadata.sizeBytes)
	if err != nil || !terminated {
		return nil
	}
	locationFingerprint, validHeader := piCursorLocationFingerprint(ctx, source, options)
	if !validHeader {
		return nil
	}
	_, err = runner.ExecContext(ctx, `
		INSERT INTO source_cursor_state (
			harness, source_kind, source_state_key, collector, parser, cursor_kind,
			byte_offset, source_mtime_ms, source_size_bytes, prefix_hash, boundary_hash, location_fingerprint, updated_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (harness, source_kind, source_state_key) DO UPDATE SET
			collector = excluded.collector, parser = excluded.parser, cursor_kind = excluded.cursor_kind,
			byte_offset = excluded.byte_offset, source_mtime_ms = excluded.source_mtime_ms,
			source_size_bytes = excluded.source_size_bytes, prefix_hash = excluded.prefix_hash,
			boundary_hash = excluded.boundary_hash, location_fingerprint = excluded.location_fingerprint,
			updated_at_ms = excluded.updated_at_ms
	`, source.Harness, source.Kind, metadata.stateKey, options.Collector, options.Parser, piByteCursorKind,
		metadata.sizeBytes, metadata.mtimeMs, metadata.sizeBytes, prefix, boundary, locationFingerprint, syncNowMs(options.Now))
	return err
}

func piCursorLocationFingerprint(ctx context.Context, source Source, options SyncOptions) (string, bool) {
	file, err := os.Open(source.Path)
	if err != nil {
		return "", false
	}
	defer func() { _ = file.Close() }()
	header, ok, err := piCursorHeader(ctx, file, piSessionIDFromFilename(source.Path))
	if err != nil || !ok {
		return "", false
	}
	location, _ := resolveFactLocation(ctx, options, header.cwd, "", "")
	if location == nil {
		return "", true
	}
	return locationSemanticKey(*location), true
}

func sourceCursorHashes(path string, offset int64) (string, string, bool, error) {
	if offset <= 0 {
		return "", "", false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = file.Close() }()
	length := min(int64(sourceCursorHashWindow), offset)
	boundary := make([]byte, length)
	prefixHasher := sha256.New()
	if _, err := io.CopyN(prefixHasher, file, offset); err != nil {
		return "", "", false, fmt.Errorf("read source cursor prefix: %w", err)
	}
	if _, err := file.ReadAt(boundary, offset-length); err != nil {
		return "", "", false, fmt.Errorf("read source cursor boundary: %w", err)
	}
	boundarySum := sha256.Sum256(boundary)
	return fmt.Sprintf("%x", prefixHasher.Sum(nil)), fmt.Sprintf("%x", boundarySum), boundary[len(boundary)-1] == '\n', nil
}
