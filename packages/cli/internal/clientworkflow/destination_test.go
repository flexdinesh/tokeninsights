package clientworkflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

func TestLocalDestinationSeparatesEndpointFromPrivateTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v2/instance" {
			t.Error(r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "instance", DataEpoch: "database", DataReadiness: "ready", DatasetId: "default", ServerKind: "personal", Capabilities: []string{"raw-ingestion"}, Permissions: []api.InstanceResponseV2Permissions{"ingest"}})
	}))
	defer server.Close()
	settings := config.Settings{ServerKind: serverfeatures.Personal}
	session, err := Resolve(t.Context(), settings, func(context.Context) (service.State, error) {
		return service.State{Record: &service.Record{URL: server.URL + "/", Socket: "/private/socket", InstanceID: "instance"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Destination == nil || session.Destination.URL != "http://local" || session.Destination.Identity != server.URL || session.Destination.DatabaseID != "database" || session.Destination.DatasetID != "default" || !session.Destination.Local {
		t.Fatal("private transport became persisted identity", session.Destination)
	}
}

func TestRemoteDestinationCanonicalizesTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v2/instance" {
			t.Error(r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "instance", DataEpoch: "database", DataReadiness: "ready", DatasetId: "default", ServerKind: "hosted", Capabilities: []string{"raw-ingestion"}, Permissions: []api.InstanceResponseV2Permissions{"ingest"}})
	}))
	defer server.Close()
	for _, suffix := range []string{"/", ""} {
		session, err := Resolve(t.Context(), config.Settings{ServerKind: serverfeatures.Hosted, ServerToken: "fixture", ServerURL: server.URL + suffix}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if session.URL != server.URL || session.Destination == nil || session.Destination.URL != server.URL || session.Destination.Identity != server.URL {
			t.Fatal("slash changed canonical transport or identity", session)
		}
	}
}
