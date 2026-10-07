package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

func TestLocalReplacementReplaysJournalWithIndependentBinding(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "collector.sqlite")
	database, _, err := db.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	fact := publication.Fact{Harness: "pi", Session: publication.Session{Harness: "pi", NativeID: "fixture-session", FirstOccurredAtMs: 10, LastOccurredAtMs: 10}, Message: &publication.Message{NativeID: "fixture-message", OccurredAtMs: 10}, OccurredAtMs: 10, Provider: "fixture-provider", ProviderSource: "explicit", Model: "fixture-model", UsageScope: "message", Quality: "exact", Countable: true, InputTokens: 10, OutputTokens: 2, TotalTokens: 12}
	publication.SetIDs(&fact)
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collectorstore.Record(ctx, tx, fact, 20); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	first, err := serverstore.CreateIfMissing(filepath.Join(root, "first-server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := serverstore.CreateIfMissing(filepath.Join(root, "second-server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	type handlerHolder struct{ handler http.Handler }
	var current atomic.Value
	current.Store(handlerHolder{ingestion.NewHandler(ingestion.NewCore(first))})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { current.Load().(handlerHolder).handler.ServeHTTP(w, r) }))
	defer server.Close()
	options := Options{CollectorDBPath: path, ServerDBPath: filepath.Join(root, "local-server.sqlite"), PublishOnly: true, EnsureLocal: func(context.Context) (string, error) { return server.URL, nil }}
	for _, store := range []*serverstore.Store{first, second} {
		current.Store(handlerHolder{ingestion.NewHandler(ingestion.NewCore(store))})
		result, err := RunLegacyForTest(ctx, options)
		if err != nil || result.Inserted != 1 || result.Pending != 0 {
			t.Fatalf("replacement publish %+v %v", result, err)
		}
		var facts, total int
		if err := store.SQL().QueryRow("SELECT COUNT(*),SUM(total_tokens) FROM canonical_token_usage").Scan(&facts, &total); err != nil || facts != 1 || total != 12 {
			t.Fatalf("server facts=%d total=%d error=%v", facts, total, err)
		}
	}
	var bindings int
	if err := database.QueryRow("SELECT COUNT(*) FROM publication_destinations WHERE acknowledged_sequence=1").Scan(&bindings); err != nil || bindings != 2 {
		t.Fatalf("reused old cursor bindings=%d error=%v", bindings, err)
	}
	result, err := RunLegacyForTest(ctx, options)
	if err != nil || result.Batches != 0 {
		t.Fatalf("unchanged replacement republished %+v %v", result, err)
	}
}
