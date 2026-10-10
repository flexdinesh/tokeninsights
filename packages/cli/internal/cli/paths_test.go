package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFreshRoleDefaultsPreserveLegacyDatabase(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)
	t.Setenv("TOKENINSIGHTS_DB_PATH", filepath.Join(root, "legacy-override.sqlite"))
	t.Setenv("TOKENINSIGHTS_COLLECTOR_DB_PATH", "")
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", "")
	legacy := filepath.Join(root, "tokeninsights", "tokeninsights.sqlite")
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy-history"), 0600); err != nil {
		t.Fatal(err)
	}
	if defaultCollectorDBPath() != filepath.Join(root, "tokeninsights", "collector.sqlite") || defaultServerDBPath() != filepath.Join(root, "tokeninsights", "server.sqlite") {
		t.Fatal("role defaults used legacy storage")
	}
	value, err := os.ReadFile(legacy)
	if err != nil || string(value) != "legacy-history" {
		t.Fatal("legacy changed")
	}
}

func TestRoleEnvironmentPathsIndependent(t *testing.T) {
	t.Setenv("TOKENINSIGHTS_COLLECTOR_DB_PATH", "/fixture/collector.sqlite")
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", "/fixture/server.sqlite")
	if defaultCollectorDBPath() != "/fixture/collector.sqlite" || defaultServerDBPath() != "/fixture/server.sqlite" {
		t.Fatal("role environment selection")
	}
}
