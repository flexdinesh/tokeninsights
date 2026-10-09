package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func TestIngestionPreflightRejectsBeforeCapture(t *testing.T) {
	for _, command := range []string{"sync", "web"} {
		for _, incompatible := range []string{"protocol", "extractor", "database", "dataset", "completion", "body-limit", "entry-limit"} {
			t.Run(command+"/"+incompatible, func(t *testing.T) {
				kind := serverfeatures.Hosted
				caps := evidence.Capabilities{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: "database", DatasetID: "dataset", Completion: "acceptance", MaxBodyBytes: evidence.MaxBodyBytes, MaxEntries: evidence.MaxEntries}
				switch incompatible {
				case "protocol":
					caps.ProtocolVersion++
				case "extractor":
					caps.ExtractorVersion++
				case "database":
					caps.DatabaseID = "replacement"
				case "dataset":
					caps.DatasetID = "other-user"
				case "completion":
					caps.Completion = "processing"
				case "body-limit":
					caps.MaxBodyBytes--
				case "entry-limit":
					caps.MaxEntries--
				}
				var requests atomic.Int64
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Header.Get("Authorization") != "Bearer fixture-token" {
						t.Error("missing bearer on negotiation")
					}
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/api/v2/instance":
						capabilities := []string{"usage", "facets", "web-dashboard", "raw-ingestion"}
						if kind == serverfeatures.Personal {
							capabilities = append(capabilities, "terminal-dashboard")
						}
						_ = json.NewEncoder(w).Encode(api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "instance", DataEpoch: "database", DataReadiness: "ready", DatasetId: "dataset", ServerKind: api.InstanceResponseV2ServerKind(kind), Capabilities: capabilities, Permissions: []api.InstanceResponseV2Permissions{"read", "ingest"}})
					case "/api/v3/ingestion/capabilities":
						_ = json.NewEncoder(w).Encode(caps)
					default:
						t.Error("unexpected work", r.URL.Path)
					}
				}))
				t.Cleanup(remote.Close)
				settings := config.Defaults()
				settings.ServerKind, settings.ServerURL, settings.ServerToken = kind, remote.URL, "fixture-token"
				settings.CollectorDBPath, settings.ServerDBPath = filepath.Join(t.TempDir(), "collector.sqlite"), filepath.Join(t.TempDir(), "server.duckdb")
				invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}
				spec, ok := commandByName(command)
				if !ok {
					t.Fatal("missing command")
				}
				args := []string{}
				if command == "sync" {
					args = append(args, "--wait")
				}
				if err := spec.run(invocation, args); err == nil || strings.Contains(err.Error(), "fixture-token") {
					t.Fatal("incompatible raw contract accepted", err)
				}
				if requests.Load() != 2 {
					t.Fatal("unexpected preflight work", requests.Load())
				}
				assertViewMissingPath(t, settings.CollectorDBPath)
				assertViewMissingPath(t, settings.ServerDBPath)
			})
		}
	}
}

func TestHostedQueryOnlyWebNeedsNoIngestionNegotiation(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/instance" {
			t.Error("read-only web negotiated ingestion", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "instance", DataEpoch: "database", DataReadiness: "ready", DatasetId: "dataset", ServerKind: "hosted", Capabilities: []string{"usage", "facets", "web-dashboard"}, Permissions: []api.InstanceResponseV2Permissions{"read"}})
	}))
	defer remote.Close()
	settings := config.Defaults()
	settings.ServerKind, settings.ServerURL, settings.ServerToken = serverfeatures.Hosted, remote.URL, "read-only-token"
	previous := openDashboard
	openDashboard = func(string) error { return nil }
	defer func() { openDashboard = previous }()
	if err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--sync=false"}); err != nil {
		t.Fatal(err)
	}
}
