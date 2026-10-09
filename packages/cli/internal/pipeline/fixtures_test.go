package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
	"os"
	"path/filepath"
	"testing"
)

func extractJSONL(ctx context.Context, source Source, options SyncOptions, store *rawcollectorstore.Store) (int, error) {
	return extractSingleRaw(ctx, source, options, store)
}

func extractSQLite(ctx context.Context, source Source, options SyncOptions, store *rawcollectorstore.Store) (int, error) {
	return extractSingleRaw(ctx, source, options, store)
}

type openCodeSQLiteMessage struct {
	ID          string
	SessionID   string
	TimeCreated int64
	TimeUpdated int64
	Data        string
}

func createOpenCodeSQLiteMessages(t *testing.T, dbPath string, messages ...openCodeSQLiteMessage) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec(`
		CREATE TABLE message (
			id text PRIMARY KEY,
			session_id text NOT NULL,
			time_created integer NOT NULL,
			time_updated integer NOT NULL,
			data text NOT NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if _, err := database.Exec(`
			INSERT INTO message (id, session_id, time_created, time_updated, data)
			VALUES (?, ?, ?, ?, ?)
		`, message.ID, message.SessionID, message.TimeCreated, message.TimeUpdated, message.Data); err != nil {
			t.Fatal(err)
		}
	}
}

func extractSingleRaw(ctx context.Context, source Source, options SyncOptions, store *rawcollectorstore.Store) (int, error) {
	directory, err := os.MkdirTemp("", "tokeninsights-capture-*")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	if options.locationResolver == nil {
		options.locationResolver = &locationResolver{}
	}
	p, err := prepareRawSource(ctx, source, options, store, directory)
	defer p.close()
	if err != nil {
		if p.quarantine != nil {
			err = errors.Join(err, store.SaveQuarantine(ctx, *p.quarantine))
		}
		return 0, err
	}
	return commitPreparedRaw(ctx, source, p, store)
}

func withSyncStats(ctx context.Context, stats *syncStats) context.Context {
	if stats == nil {
		return ctx
	}
	return context.WithValue(ctx, syncStatsKey{}, stats)
}
