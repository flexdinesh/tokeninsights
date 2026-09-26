package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestAPIWithoutCrossOriginAccess(t *testing.T) {
	a := newApp(context.Background(), Options{DBPath: fixture(t)}, io.Discard)
	a.syncer = func(context.Context, pipeline.SyncOptions) (pipeline.Summary, error) {
		return pipeline.Summary{}, nil
	}
	defer a.jobs.Wait()
	for _, route := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/api/v1/instance", http.StatusOK},
		{http.MethodGet, "/api/v1/usage", http.StatusOK},
		{http.MethodGet, "/api/v1/usage/facets", http.StatusOK},
		{http.MethodPost, "/api/v1/sync", http.StatusAccepted},
		{http.MethodOptions, "/api/v1/usage", http.StatusMethodNotAllowed},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, "http://usage.example.test"+route.path, nil)
			request.Header.Set("Origin", "http://another-server.example.test")
			response := httptest.NewRecorder()
			a.handler().ServeHTTP(response, request)
			if response.Code != route.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, route.status, response.Body.String())
			}
			for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"} {
				if got := response.Header().Get(header); got != "" {
					t.Fatalf("unexpected %s = %q", header, got)
				}
			}
		})
	}
}
