package collector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func TestDatabaseAliasRejectedBeforeCollection(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "collector.sqlite")
	b := filepath.Join(root, "alias.sqlite")
	if err := os.WriteFile(a, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, link := range []func(string, string) error{os.Link, os.Symlink} {
		if err := link(a, b); err != nil {
			t.Fatal(err)
		}
		_, err := Run(context.Background(), Options{CollectorDBPath: a, ServerDBPath: b, PublishOnly: true})
		if err == nil || !strings.Contains(err.Error(), "database_paths_alias") {
			t.Fatalf("alias: %v", err)
		}
		_ = os.Remove(b)
	}
	value, err := os.ReadFile(a)
	if err != nil || string(value) != "untouched" {
		t.Fatal("alias guard mutated database")
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(a)
	if err := ValidatePaths(a, b); err == nil {
		t.Fatal("dangling alias accepted")
	}
}

func TestRemoteBindingPinsDatabaseAndNeverStartsLocal(t *testing.T) {
	databaseID := "server-first"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(publication.NewCapabilities(databaseID))
	}))
	defer server.Close()
	root := t.TempDir()
	options := Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite"), ServerURL: server.URL, PublishOnly: true, EnsureLocal: func(context.Context) (string, error) { t.Fatal("remote started local service"); return "", nil }}
	if _, err := Run(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	databaseID = "server-replacement"
	if _, err := Run(context.Background(), options); err == nil || !strings.Contains(err.Error(), "server_database_changed") {
		t.Fatalf("replacement: %v", err)
	}
	if _, err := os.Stat(options.ServerDBPath); !os.IsNotExist(err) {
		t.Fatal("remote created local server database")
	}
}

func TestReceiptMismatchKeepsExactBatchForNextManualSync(t *testing.T) {
	root := t.TempDir()
	collectorPath := filepath.Join(root, "collector.sqlite")
	database, _, err := db.CreateIfMissing(collectorPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	f := publication.Fact{Harness: "pi", Session: publication.Session{Harness: "pi", NativeID: "fixture-session", FirstOccurredAtMs: 10, LastOccurredAtMs: 10}, Message: &publication.Message{NativeID: "fixture-message", OccurredAtMs: 10}, OccurredAtMs: 10, Provider: "fixture-provider", ProviderSource: "explicit", Model: "fixture-model", UsageScope: "message", Quality: "exact", Countable: true, InputTokens: 10, OutputTokens: 2, TotalTokens: 12}
	publication.SetIDs(&f)
	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collectorstore.Record(context.Background(), tx, f, 20); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var requests [][]byte
	incompatible := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			capabilities := publication.NewCapabilities("fixture-server")
			if incompatible {
				capabilities.ProtocolVersion++
			}
			_ = json.NewEncoder(w).Encode(capabilities)
			return
		}
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, body)
		batch, err := publication.DecodeBatch(body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		receipt := publication.Receipt{DatabaseID: batch.DatabaseID, StreamID: batch.StreamID, BatchID: batch.BatchID, FromSequence: batch.FromSequence, ToSequence: batch.ToSequence, RequestHash: publication.RequestHash(body), Inserted: 1, CommittedAtMs: 30, Revision: 1}
		if len(requests) == 1 {
			receipt.BatchID = "wrong-batch"
		}
		_ = json.NewEncoder(w).Encode(receipt)
	}))
	defer server.Close()
	options := Options{CollectorDBPath: collectorPath, ServerDBPath: filepath.Join(root, "server.sqlite"), ServerURL: server.URL, PublishOnly: true}
	var progress []DeliveryProgress
	options.DeliveryProgress = func(value DeliveryProgress) { progress = append(progress, value) }
	first, err := Run(context.Background(), options)
	if err == nil || first.Pending != 1 || first.Batches != 0 {
		t.Fatalf("invalid receipt advanced progress: %+v %v", first, err)
	}
	if len(progress) != 2 || progress[1] != (DeliveryProgress{Pending: 1, PendingKnown: true}) {
		t.Fatalf("unacknowledged receipt changed visible progress: %+v", progress)
	}
	incompatible = true
	if _, err := Run(context.Background(), options); err == nil || len(requests) != 1 {
		t.Fatalf("incompatible capabilities submitted pending batch: %v", err)
	}
	incompatible = false
	progress = nil
	second, err := Run(context.Background(), options)
	if err != nil || second.Pending != 0 || second.Batches != 1 {
		t.Fatalf("resume: %+v %v", second, err)
	}
	if len(progress) != 3 || progress[2] != (DeliveryProgress{Batches: 1, PendingKnown: true}) {
		t.Fatalf("acknowledged batch missing from visible progress: %+v", progress)
	}
	if len(requests) != 2 || string(requests[0]) != string(requests[1]) {
		t.Fatal("retry changed saved request bytes")
	}
}

func TestTransportErrorsNeverEchoCredentials(t *testing.T) {
	options := Options{CollectorDBPath: filepath.Join(t.TempDir(), "collector.sqlite"), ServerDBPath: filepath.Join(t.TempDir(), "server.sqlite"), ServerURL: "http://secret:password@example.test", PublishOnly: true}
	_, err := Run(context.Background(), options)
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") {
		t.Fatalf("unsafe error: %v", err)
	}
}
