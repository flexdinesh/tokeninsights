package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemovedCommandsRejectWithoutStorageOrServiceSideEffects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("XDG_STATE_HOME", root)
	t.Setenv("XDG_RUNTIME_DIR", root)
	t.Setenv("XDG_DATA_HOME", root)
	path := filepath.Join(root, "existing.sqlite")
	contents := []byte("existing storage must remain untouched")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"view", "serve", "service", "server", "collector", "normalize", "reset-canonical", "reset-all", "__service-run"} {
		if _, exists := commandByName(name); exists {
			t.Fatalf("removed command %s still registered", name)
		}
		for _, args := range [][]string{
			{name},
			{name, "--help"},
			{name, "--confirm", "--collector-db-path", path},
		} {
			err := Run(context.Background(), args, io.Discard, io.Discard, time.Now())
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("removed %v: %v", args, err)
			}
			contentsAfter, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(contentsAfter, contents) {
				t.Fatalf("removed %v changed storage: %v", args, err)
			}
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("removed commands created state: %v %v", entries, err)
	}
}

func TestRemovedFlagsRejectBeforeSideEffects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("XDG_STATE_HOME", root)
	t.Setenv("XDG_RUNTIME_DIR", root)
	t.Setenv("XDG_DATA_HOME", root)
	path := filepath.Join(root, "missing.sqlite")
	for _, args := range [][]string{
		{"sync", "--db-path", path},
		{"collector", "normalize", "--db-path", path},
		{"collector", "reset-canonical", "--confirm", "--db-path", path},
		{"collector", "reset-all", "--confirm", "--db-path", path},
		{"tui", "--db-path", path},
		{"tui", "--no-sync"},
		{"service", "start", "--db-path", path},
		{"server", "run", "--db-path", path},
	} {
		err := Run(context.Background(), args, io.Discard, io.Discard, time.Now())
		if !errors.Is(err, ErrUsage) {
			t.Fatalf("removed flags %v: %v", args, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("removed flags created state: %v %v", entries, err)
	}
}
