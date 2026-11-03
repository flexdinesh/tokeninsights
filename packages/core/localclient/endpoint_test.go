package localclient

import (
	"errors"
	"os"
	"path/filepath"
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
	first, _ := serveForTest(t, testOptions(t))
	secondOptions := testOptions(t)
	secondOptions.Host = "0.0.0.0"
	second, _ := serveForTest(t, secondOptions)
	if second.URL == first.URL || second.InstanceID == first.InstanceID {
		t.Fatal("fixture owners not independent")
	}
	session, err := ConnectEndpoint(t.Context(), second.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if session.InstanceID != second.InstanceID || session.URL != second.URL {
		t.Fatal("desktop selected another local dataset")
	}
	instance, err := session.Query.Instance(t.Context())
	if err != nil || instance.InstanceID != second.InstanceID {
		t.Fatal("query did not pin endpoint owner", instance.InstanceID, err)
	}
	if _, err := os.Stat(defaults); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("endpoint launch resolved or created default storage", err)
	}
	for _, bad := range []string{"https://127.0.0.1:8765", "http://example.test:8765", "http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:1", "http://127.0.0.1:8765/?token=secret"} {
		if other, err := ConnectEndpoint(t.Context(), bad, false); err == nil {
			_ = other.Close()
			t.Fatal("invalid or missing endpoint accepted", bad)
		}
	}
	if _, err := os.Stat(defaults); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed endpoint fell back to defaults", err)
	}
}

func TestDescriptorRejectsChangedProtocolAndSettings(t *testing.T) {
	options := testOptions(t)
	descriptor, _ := serveForTest(t, options)
	if err := descriptor.Compatible(descriptor.Settings); err != nil {
		t.Fatal(err)
	}
	descriptor.ProtocolVersion++
	if !errors.Is(descriptor.Compatible(descriptor.Settings), localprotocol.ErrIncompatible) {
		t.Fatal("incompatible protocol accepted")
	}
	descriptor.ProtocolVersion--
	descriptor.URL = "http://0.0.0.0:8765"
	if !errors.Is(descriptor.Compatible(descriptor.Settings), localprotocol.ErrIncompatible) {
		t.Fatal("undialable descriptor accepted")
	}
}
