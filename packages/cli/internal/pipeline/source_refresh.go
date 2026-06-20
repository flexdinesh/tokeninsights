package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"
)

const recentSourceRefreshWindow = 48 * time.Hour

type sourceRefreshMetadata struct {
	stateKey  string
	mtimeMs   int64
	sizeBytes int64
}

type sourceRefreshState struct {
	collector                 string
	parser                    string
	lastSuccessfulRefreshAtMs int64
	sourceMtimeMs             int64
	sourceSizeBytes           int64
}

func sourceRefreshMetadataFor(source Source) (sourceRefreshMetadata, bool) {
	if !sourceRefreshEnabled(source) {
		return sourceRefreshMetadata{}, false
	}
	info, err := os.Stat(source.Path)
	if err != nil || info.IsDir() {
		return sourceRefreshMetadata{}, false
	}
	mtimeMs := info.ModTime().UnixMilli()
	if mtimeMs < 0 || info.Size() < 0 {
		return sourceRefreshMetadata{}, false
	}
	return sourceRefreshMetadata{
		stateKey:  source.ID,
		mtimeMs:   mtimeMs,
		sizeBytes: info.Size(),
	}, true
}

func sourceRefreshEnabled(source Source) bool {
	if source.ID == "" || source.Path == "" {
		return false
	}
	switch source.Harness {
	case HarnessOpenCode:
		return source.Kind == opencodeSQLiteSourceKind
	case HarnessPi:
		return source.Kind == piSessionJSONLSourceKind
	case HarnessCodex:
		return source.Kind == codexSessionJSONLSourceKind
	case HarnessClaudeCode:
		return source.Kind == claudeCodeJSONLSourceKind
	default:
		return false
	}
}

func shouldSkipSourceRefresh(ctx context.Context, runner sqlRunner, source Source, options SyncOptions, metadata sourceRefreshMetadata, ok bool) (bool, error) {
	if options.FullRefresh {
		return false, nil
	}
	if !ok {
		return false, nil
	}
	state, found, err := loadSourceRefreshState(ctx, runner, source, metadata.stateKey)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if state.collector != options.Collector || state.parser != options.Parser {
		return false, nil
	}
	if state.sourceMtimeMs != metadata.mtimeMs || state.sourceSizeBytes != metadata.sizeBytes {
		return false, nil
	}
	freshnessCutoffMs := state.lastSuccessfulRefreshAtMs - recentSourceRefreshWindow.Milliseconds()
	return metadata.mtimeMs < freshnessCutoffMs, nil
}

func loadSourceRefreshState(ctx context.Context, runner sqlRunner, source Source, stateKey string) (sourceRefreshState, bool, error) {
	var state sourceRefreshState
	err := runner.QueryRowContext(ctx, `
		SELECT collector, parser, last_successful_refresh_at_ms, source_mtime_ms, source_size_bytes
		FROM source_refresh_state
		WHERE harness = ? AND source_kind = ? AND source_state_key = ?
	`, source.Harness, source.Kind, stateKey).Scan(
		&state.collector,
		&state.parser,
		&state.lastSuccessfulRefreshAtMs,
		&state.sourceMtimeMs,
		&state.sourceSizeBytes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceRefreshState{}, false, nil
	}
	if err != nil {
		return sourceRefreshState{}, false, err
	}
	return state, true, nil
}

func upsertSourceRefreshState(ctx context.Context, runner sqlRunner, source Source, options SyncOptions, metadata sourceRefreshMetadata, ok bool) error {
	if !ok {
		return nil
	}
	nowMs := syncNowMs(options.Now)
	_, err := runner.ExecContext(ctx, `
		INSERT INTO source_refresh_state (
			harness, source_kind, source_state_key, collector, parser,
			last_successful_refresh_at_ms, source_mtime_ms, source_size_bytes, updated_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (harness, source_kind, source_state_key) DO UPDATE SET
			collector = excluded.collector,
			parser = excluded.parser,
			last_successful_refresh_at_ms = excluded.last_successful_refresh_at_ms,
			source_mtime_ms = excluded.source_mtime_ms,
			source_size_bytes = excluded.source_size_bytes,
			updated_at_ms = excluded.updated_at_ms
	`, source.Harness, source.Kind, metadata.stateKey, options.Collector, options.Parser, nowMs, metadata.mtimeMs, metadata.sizeBytes, nowMs)
	return err
}
