package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
)

// queryServerURL uses the production REST handlers against canonical-only
// storage. Every migrated viewer test also checks that querying never posts.
func queryServerURL(t *testing.T, path string) string {
	t.Helper()
	var writes atomic.Int64
	handler := server.NewHandler(context.Background(), path, nil, io.Discard, "127.0.0.1")
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
