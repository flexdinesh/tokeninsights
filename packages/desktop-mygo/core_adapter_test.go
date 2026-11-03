package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExternalDashboardDoesNotResolveLocalStorage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "invalid-config.json")
	if err := os.WriteFile(path, []byte("invalid local settings"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", path)
	for _, target := range []string{"https://usage.example.com/tokens", "http://192.0.2.1:8766/"} {
		_, err := connectLocal(t.Context(), target, true)
		var external *externalTarget
		if !errors.As(err, &external) || external.URL != target {
			t.Fatalf("external target resolved local settings: %v", err)
		}
	}
}
