package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/duckdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
)

// queryServerURL uses the production REST handlers against canonical-only
// storage. Every migrated viewer test also checks that querying never posts.
func queryServerURL(t *testing.T, path string) string {
	t.Helper()
	var writes atomic.Int64
	handler := queryHandler(t, path, "")
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writes.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		fixture.Close()
		if count := writes.Load(); count != 0 {
			t.Errorf("viewer issued %d mutation requests", count)
		}
	})
	return fixture.URL
}

func queryHandler(t *testing.T, path, instance string) http.Handler {
	t.Helper()
	value, ok := queryStores.Load(path)
	if !ok {
		t.Fatal("missing fixture store", path)
	}
	store, ok := value.(*datastore.Store)
	if !ok {
		t.Fatal("invalid fixture store")
	}
	return server.NewDataHandler(context.Background(), duckdb.Source{Store: store}, io.Discard, "127.0.0.1", instance, false)
}

var queryStores sync.Map
