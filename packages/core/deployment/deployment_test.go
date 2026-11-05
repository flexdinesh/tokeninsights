package deployment_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/tcptransport"
)

// TestPersonalNetworkDeployment exercises the invocation-owned gateway across
// the actual container network. Collector replay/accounting contracts remain
// beside the direct collector and real SQLite adapters.
func TestPersonalNetworkDeployment(t *testing.T) {
	control := os.Getenv("TOKENINSIGHTS_TEST_PERSONAL_CONTROL_URL")
	if control == "" {
		t.Skip("run isolated deployment validation")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, control+"/connection", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor struct {
		Connection string `json:"connection"`
	}
	err = json.NewDecoder(response.Body).Decode(&descriptor)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal("gateway fixture unavailable", response.StatusCode, err)
	}
	target, err := endpoint.Parse(descriptor.Connection)
	if err != nil || target.TCP == nil {
		t.Fatal("fixture did not provide a TCP connection")
	}
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", filepath.Join(root, "config.json"))
	session, err := localclient.ConnectRemote(ctx, tcptransport.Dialer(*target.TCP), localclient.RemoteOptions{Token: target.TCP.Token})
	if err != nil {
		t.Fatal("TCP session rejected", err)
	}
	defer func() { _ = session.Close() }()
	instance, err := session.Query.Instance(ctx)
	if err != nil || instance.DatabaseID == "" || instance.DatasetID != "default" || instance.InstanceID != session.InstanceID {
		t.Fatal("TCP identity mismatch", err)
	}
	localHostname, err := os.Hostname()
	if err != nil || instance.Hostname == "" || instance.Hostname == localHostname {
		t.Fatal("TCP selected the viewer machine", err)
	}
	if _, err := session.Query.Status(ctx); err != nil {
		t.Fatal("TCP query failed", err)
	}
	if _, err := session.Collect(ctx, false, ""); err != nil {
		t.Fatal("default remote Refresh failed", err)
	}
	if err := session.Reprocess(ctx); err == nil {
		t.Fatal("viewer token authorized maintenance")
	}
	if _, err := session.Collect(ctx, false, "/private/source-override"); err == nil {
		t.Fatal("viewer token authorized a source override")
	}
	if _, err := session.Query.Status(ctx); err != nil {
		t.Fatal("rejected operations broke the session", err)
	}
	for _, field := range []string{"pin", "token"} {
		changed := *target.TCP
		if field == "pin" {
			changed.Pin = strings.Repeat("0", 64)
		} else {
			changed.Token = strings.Repeat("A", 43)
		}
		rejected, err := localclient.ConnectRemote(ctx, tcptransport.Dialer(changed), localclient.RemoteOptions{Token: changed.Token})
		if err == nil {
			_ = rejected.Close()
			t.Fatalf("wrong %s accepted", field)
		}
		if strings.Contains(err.Error(), target.TCP.Token) || strings.Contains(err.Error(), changed.Token) {
			t.Fatal("connection error exposed a credential")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(err) {
		t.Fatal("TCP session created local storage", err)
	}
}
