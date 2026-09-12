package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowAPIOrigins(t *testing.T) {
	t.Run("cross-origin request", func(t *testing.T) {
		called := false
		handler := allowAPIOrigins(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusAccepted)
		}))
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
		request.Header.Set("Origin", "https://example.com")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if !called || response.Code != http.StatusAccepted {
			t.Fatalf("request rejected: called=%t status=%d", called, response.Code)
		}
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("origin = %q", got)
		}
	})

	t.Run("preflight", func(t *testing.T) {
		handler := allowAPIOrigins(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("preflight reached API handler")
		}))
		request := httptest.NewRequest(http.MethodOptions, "/api/v1/usage", nil)
		request.Header.Set("Origin", "https://example.com")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d", response.Code)
		}
		if got := response.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" {
			t.Fatalf("methods = %q", got)
		}
		if got := response.Header().Get("Access-Control-Allow-Headers"); got != "Accept, Content-Type" {
			t.Fatalf("headers = %q", got)
		}
	})

	t.Run("non API", func(t *testing.T) {
		handler := allowAPIOrigins(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("unexpected origin header %q", got)
		}
	})
}
