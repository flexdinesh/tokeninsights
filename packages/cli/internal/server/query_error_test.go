package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

func TestCanceledDashboardRequestsAreQuiet(t *testing.T) {
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, route := range []string{"instance", "status", "usage", "usage/facets"} {
		t.Run(route, func(t *testing.T) {
			var log bytes.Buffer
			handler := NewDataHandler(t.Context(), sqlanalytics.Source{Store: store}, &log, "127.0.0.1", "test", false)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			request := httptest.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/api/v2/"+route, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if log.Len() != 0 || response.Body.Len() != 0 {
				t.Fatalf("canceled request reported a failure: log=%q body=%q", log.String(), response.Body.String())
			}
		})
	}
}

func TestDashboardQueryErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		canceled bool
		quiet    bool
	}{
		{"client cancellation", context.Canceled, true, true},
		{"wrapped client cancellation", fmt.Errorf("facets provider: %w", context.Canceled), true, true},
		{"internal cancellation", context.Canceled, false, false},
		{"query timeout", context.DeadlineExceeded, false, false},
		{"storage failure", errors.New("storage failed"), false, false},
		{"storage failure during cancellation", errors.New("storage failed"), true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.canceled {
				cancel()
			}
			var log bytes.Buffer
			a := newApp(t.Context(), Options{InstanceID: "test"}, &log)
			request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v2/usage", nil)
			response := httptest.NewRecorder()
			a.queryError(response, request, test.err)
			if test.quiet {
				if log.Len() != 0 || response.Body.Len() != 0 {
					t.Fatalf("canceled request reported a failure: log=%q body=%q", log.String(), response.Body.String())
				}
				return
			}
			var body api.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusServiceUnavailable || body.Code != api.ErrorCodeUnavailable || !strings.Contains(log.String(), test.err.Error()) {
				t.Fatalf("query failure was not reported: status=%d body=%+v log=%q", response.Code, body, log.String())
			}
		})
	}
}
