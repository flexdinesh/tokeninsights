package localclient

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

func TestEndpointAttachesCustomOwnerWithoutDefaultSettingsOrFallback(t *testing.T) {
	defaults := filepath.Join(t.TempDir(), "default-data")
	t.Setenv("XDG_DATA_HOME", defaults)
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", filepath.Join(t.TempDir(), "invalid-config.json"))
	if err := os.WriteFile(os.Getenv("TOKENINSIGHTS_CONFIG_PATH"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstOptions := testOptions(t)
	serveForTest(t, firstOptions)
	first := dashboardForTest(t, firstOptions)
	secondOptions := testOptions(t)
	secondOptions.Host = "0.0.0.0"
	serveForTest(t, secondOptions)
	second := dashboardForTest(t, secondOptions)
	if second.URL == first.URL || second.InstanceID == first.InstanceID {
		t.Fatal("fixture owners not independent")
	}
	for _, dashboard := range []bool{false, true} {
		session, err := ConnectEndpoint(t.Context(), second.URL, false, dashboard)
		if err != nil {
			t.Fatal(err)
		}
		if session.InstanceID != second.InstanceID || dashboard != (session.URL == second.URL) {
			_ = session.Close()
			t.Fatal("desktop selected another local dataset or lease kind", dashboard, session.URL)
		}
		instance, err := session.Query.Instance(t.Context())
		_ = session.Close()
		if err != nil || instance.InstanceID != second.InstanceID {
			t.Fatal("query did not pin endpoint owner", instance.InstanceID, err)
		}
	}
	if _, err := os.Stat(defaults); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("endpoint launch resolved or created default storage", err)
	}
	for _, bad := range []string{"https://127.0.0.1:8765", "http://example.test:8765", "http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:1", "http://127.0.0.1:8765/?token=secret"} {
		if other, err := ConnectEndpoint(t.Context(), bad, false, true); err == nil {
			_ = other.Close()
			t.Fatal("invalid or missing endpoint accepted", bad)
		}
	}
	if _, err := os.Stat(defaults); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed endpoint fell back to defaults", err)
	}
}

func TestDescriptorRejectsChangedProtocolSettingsAndDashboardShape(t *testing.T) {
	options := testOptions(t)
	descriptor, _ := serveForTest(t, options)
	if descriptor.URL != "" || descriptor.Listen != nil {
		t.Fatal("backend bound a dashboard before any dashboard lease", descriptor.URL)
	}
	if err := descriptor.Compatible(descriptor.Settings); err != nil {
		t.Fatal(err)
	}
	changed := descriptor.Settings
	changed.AppDBPath += ".other"
	if !errors.Is(descriptor.Compatible(changed), localprotocol.ErrSettingsMismatch) {
		t.Fatal("different storage accepted")
	}
	descriptor.ProtocolVersion++
	err := descriptor.Compatible(descriptor.Settings)
	if !errors.Is(err, localprotocol.ErrIncompatible) || !strings.Contains(err.Error(), "backend protocol 3, client protocol 2") {
		t.Fatal("incompatible protocol accepted or unnamed", err)
	}
	descriptor.ProtocolVersion--
	descriptor.URL = "http://127.0.0.1:8765"
	if !errors.Is(descriptor.Compatible(descriptor.Settings), localprotocol.ErrIncompatible) {
		t.Fatal("dashboard URL without listen accepted")
	}
	descriptor.Listen = &localprotocol.Listen{Host: "0.0.0.0", Port: 8765}
	descriptor.URL = "http://0.0.0.0:8765"
	if !errors.Is(descriptor.Compatible(descriptor.Settings), localprotocol.ErrIncompatible) {
		t.Fatal("undialable descriptor accepted")
	}
}
