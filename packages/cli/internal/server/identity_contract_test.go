package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

// R08: unavailable metadata stays observable, while successful analytics always
// identify their database and explicitly report revision zero for empty history.
func TestQueryIdentityContractForEmptyAndUnavailableStorage(t *testing.T) {
	for _, ready := range []bool{false, true} {
		name := "unavailable"
		if ready {
			name = "ready-empty"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.sqlite")
			epoch := ""
			if ready {
				store, err := datastore.Open(t.Context(), path)
				if err != nil {
					t.Fatal(err)
				}
				metadata, err := store.Metadata(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				epoch = metadata.DatabaseID
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			var store *datastore.Store
			if ready {
				var err error
				store, err = datastore.Open(t.Context(), path)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = store.Close() }()
			}
			handler := NewDataHandler(t.Context(), store, io.Discard, "0.0.0.0", "test-instance", false)
			for _, route := range []string{"instance", "status", "usage", "usage/facets"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v2/"+route, nil))
				if !ready {
					if response.Code != http.StatusServiceUnavailable {
						t.Fatalf("%s unavailable status = %d", route, response.Code)
					}
					continue
				}
				if response.Code != http.StatusOK {
					t.Fatalf("%s status = %d", route, response.Code)
				}
				var body map[string]json.RawMessage
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				for field, want := range map[string]string{"instanceId": "test-instance", "dataEpoch": epoch} {
					var value string
					data, exists := body[field]
					if !exists || json.Unmarshal(data, &value) != nil || value != want {
						t.Fatalf("%s %s = %s, want %q", route, field, data, want)
					}
				}
				if route == "instance" || route == "status" {
					want := `"unavailable"`
					if ready {
						want = `"ready"`
					}
					if string(body["dataReadiness"]) != want {
						t.Fatalf("%s readiness = %s, want %s", route, body["dataReadiness"], want)
					}
				}
				if route != "instance" && string(body["revision"]) != "0" {
					t.Fatalf("%s revision = %s, want explicit zero", route, body["revision"])
				}
			}
		})
	}
}
