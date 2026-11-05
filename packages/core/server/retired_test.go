package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/server/api"
)

func TestPersonalReadAPIHasNoAccountOrIngestionRoutes(t *testing.T) {
	a := fixtureApp(t, fixture(t), Options{})
	handler := a.handler()
	for _, path := range []string{
		"/api/v2/auth", "/api/v2/auth/session", "/api/v2/auth/signup",
		"/api/v2/account", "/api/v2/account/password", "/api/v2/account/keys",
		"/api/v3/ingestion/capabilities", "/api/v3/ingestion/batches",
		"/api/v3/ingestion/batches/stream/batch",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("retired %s %s: %d", method, path, response.Code)
			}
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v2/instance", nil))
	var instance api.InstanceResponseV2
	if err := json.Unmarshal(response.Body.Bytes(), &instance); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || instance.ServerKind != api.Personal || len(instance.Permissions) != 1 || instance.Permissions[0] != api.Read {
		t.Fatal("personal instance contract", response.Code, instance)
	}
	for _, capability := range instance.Capabilities {
		if capability == "raw-ingestion" {
			t.Fatal("retired ingestion capability advertised")
		}
	}
}
