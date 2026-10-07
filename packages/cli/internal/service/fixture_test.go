package service

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestPrepareFixtureRejectsHostedBeforeResettingCollector(t *testing.T) {
	collectorPath, serverPath := fixturePaths(t)
	ctx := t.Context()
	if err := PrepareFixture(ctx, collectorPath, serverPath); err != nil {
		t.Fatal(err)
	}
	collector, err := db.OpenWritable(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	var beforeStream string
	if err := collector.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&beforeStream); err != nil {
		t.Fatal(err)
	}
	_ = collector.Close()
	if err := os.Remove(serverPath); err != nil {
		t.Fatal(err)
	}
	hosted, err := datastore.OpenWithOptions(ctx, serverPath, datastore.Options{Kind: datastore.KindHosted})
	if err != nil {
		t.Fatal(err)
	}
	if err := hosted.Close(); err != nil {
		t.Fatal(err)
	}
	beforeServer, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareFixture(ctx, collectorPath, serverPath); err == nil {
		t.Fatal("hosted database reset by personal fixture command")
	}
	afterServer, err := os.ReadFile(serverPath)
	if err != nil || !bytes.Equal(beforeServer, afterServer) {
		t.Fatal("rejected hosted database mutated", err)
	}
	collector, err = db.OpenWritable(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = collector.Close() }()
	var afterStream string
	if err := collector.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&afterStream); err != nil {
		t.Fatal(err)
	}
	if beforeStream != afterStream {
		t.Fatal("collector reset before rejecting hosted storage")
	}
}

func fixturePaths(t *testing.T) (string, string) {
	t.Helper()
	options := environment(t)
	root := filepath.Join(filepath.Dir(options.DBPath), ".tokeninsights-dev")
	return filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.duckdb")
}

func TestPrepareFixtureSeparateRolesPreserveInodesAndUnrelatedHistory(t *testing.T) {
	collectorPath, serverPath := fixturePaths(t)
	ctx := context.Background()
	if err := PrepareFixture(ctx, collectorPath, serverPath); err != nil {
		t.Fatal(err)
	}
	collector, err := db.OpenWritable(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	var stream string
	if err := collector.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&stream); err != nil {
		t.Fatal(err)
	}
	_ = collector.Close()
	server, err := datastore.Open(ctx, serverPath)
	if err != nil {
		t.Fatal(err)
	}
	before, err := server.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.SQL().Exec("INSERT INTO ingestion.legacy_receipts(dataset_id,stream_id,batch_id,request_hash,receipt_json) VALUES(?,'fixture-stream','fixture-batch','hash','{}')", datastore.DatasetID); err != nil {
		t.Fatal(err)
	}
	_ = server.Close()
	root := filepath.Dir(serverPath)
	legacy := filepath.Join(root, "tokeninsights.sqlite")
	if err := os.WriteFile(legacy, []byte("synthetic-legacy-preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("synthetic-note"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "old.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := []string{collectorPath, serverPath, collectorPath + ".lock", serverPath + ".lock", serverPath + ".service.op.lock", legacy}
	infos := make([]os.FileInfo, len(paths))
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		infos[i] = info
	}
	if err := PrepareFixture(ctx, collectorPath, serverPath); err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(infos[i], after) {
			t.Fatalf("inode replaced: %s", path)
		}
	}
	if contents, err := os.ReadFile(legacy); err != nil || string(contents) != "synthetic-legacy-preserve" {
		t.Fatal("legacy history changed", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "source")); !os.IsNotExist(err) {
		t.Fatal("generated sources not cleared", err)
	}
	collector, err = db.OpenWritable(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = collector.Close() }()
	var afterStream string
	if err := collector.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&afterStream); err != nil {
		t.Fatal(err)
	}
	if afterStream == stream {
		t.Fatal("fixture reset retained old delivery stream")
	}
	server, err = datastore.Open(ctx, serverPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()
	after, err := server.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.DatabaseID == after.DatabaseID || after.Revision != 0 || after.LastIngestionAtMs != 0 {
		t.Fatal("fixture server generation not reset", after)
	}
	var count int
	if err := server.SQL().QueryRow("SELECT COUNT(*) FROM ingestion.legacy_receipts").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestPrepareFixtureRejectsLegacyRoleBeforeResettingCollector(t *testing.T) {
	collectorPath, serverPath := fixturePaths(t)
	collector, _, err := db.CreateIfMissing(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	var stream string
	if err := collector.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&stream); err != nil {
		t.Fatal(err)
	}
	_ = collector.Close()
	legacy, err := sql.Open("sqlite", serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("CREATE TABLE synthetic_legacy(value TEXT); INSERT INTO synthetic_legacy VALUES('preserved')"); err != nil {
		t.Fatal(err)
	}
	_ = legacy.Close()
	before, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareFixture(context.Background(), collectorPath, serverPath); err == nil {
		t.Fatal("legacy server role accepted")
	}
	after, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("legacy bytes changed")
	}
	collector, err = db.OpenWritable(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = collector.Close() }()
	var afterStream string
	if err := collector.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&afterStream); err != nil {
		t.Fatal(err)
	}
	if stream != afterStream {
		t.Fatal("collector reset before server role validation")
	}
}

func TestPrepareFixtureRejectsRunningAndUncontrolledServer(t *testing.T) {
	collectorPath, serverPath := fixturePaths(t)
	ctx := context.Background()
	if err := PrepareFixture(ctx, collectorPath, serverPath); err != nil {
		t.Fatal(err)
	}
	port := 0
	if _, err := Ensure(ctx, Options{DBPath: serverPath, Port: &port}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Stop(ctx, serverPath) }()
	if err := PrepareFixture(ctx, collectorPath, serverPath); err == nil {
		t.Fatal("running fixture reset accepted")
	}
	if err := PrepareFixture(ctx, filepath.Join(t.TempDir(), "collector.sqlite"), serverPath); err == nil {
		t.Fatal("uncontrolled fixture accepted")
	}
}
