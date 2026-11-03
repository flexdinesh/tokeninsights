package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIWithoutCrossOriginAccess(t *testing.T) {
	a := fixtureApp(t, fixture(t), Options{})
	for _, route := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/api/v2/instance", http.StatusOK},
		{http.MethodGet, "/api/v2/usage", http.StatusOK},
		{http.MethodGet, "/api/v2/usage/facets", http.StatusOK},
		{http.MethodPost, "/api/v2/sync", http.StatusNotFound},
		{http.MethodOptions, "/api/v2/usage", http.StatusNotFound},
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
