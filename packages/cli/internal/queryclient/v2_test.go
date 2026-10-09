package queryclient

import (
	"errors"
	"net/http"
	"testing"

	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func TestV2DatasetPinnedReads(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing authentication")
		}
		switch r.URL.Path {
		case "/api/v2/instance":
			serveJSON(w, api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "instance", DataEpoch: "database", DataReadiness: "ready", DatasetId: "user-a", ServerKind: "hosted", Capabilities: []string{"usage", "future-capability"}, Permissions: []api.InstanceResponseV2Permissions{"read"}})
		case "/api/v2/usage":
			serveJSON(w, api.UsageResponseV2{InstanceId: "instance", DataEpoch: "database", DatasetId: "user-b", Revision: 0})
		case "/api/v2/status":
			serveJSON(w, api.StatusResponseV2{InstanceId: "instance", DataEpoch: "database", DatasetId: "user-a", DataReadiness: "ready", Pending: 1})
		default:
			t.Error("unexpected endpoint", r.URL.Path)
		}
	})).WithToken("secret").WithDataset("user-a")
	descriptor, err := client.Descriptor(t.Context())
	if err != nil || !DescriptorCapabilities(descriptor).Has(serverfeatures.Usage) || DescriptorCapabilities(descriptor).Has("future-capability") || !HasPermission(descriptor, serverfeatures.Read) || HasPermission(descriptor, serverfeatures.Ingest) {
		t.Fatal(descriptor, err)
	}
	if _, err := client.Usage(t.Context(), api.GetUsageParams{}); !errors.Is(err, ErrSnapshotChanged) {
		t.Fatal("foreign dataset accepted", err)
	}
	status, err := client.Status(t.Context())
	if err != nil || status.Pending == 0 {
		t.Fatal(status, err)
	}
}

func TestDescriptorRequiresUpgradeAndKnownKind(t *testing.T) {
	client := newClient(t, http.NotFoundHandler())
	if _, err := client.Descriptor(t.Context()); err == nil {
		t.Fatal("old server accepted")
	}
	client = newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "instance", DataEpoch: "database", DataReadiness: "ready", DatasetId: "dataset", ServerKind: "future"})
	}))
	if _, err := client.Descriptor(t.Context()); err == nil {
		t.Fatal("unknown kind accepted")
	}
}
