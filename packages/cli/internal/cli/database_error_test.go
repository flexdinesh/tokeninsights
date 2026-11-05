package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/collectordb"
)

// Older current-role evidence is never reset without a terminal. Every path
// selection route reports the selected store and preserves its exact bytes.
func TestLocalOlderStorageWithoutTerminalChangesNothing(t *testing.T) {
	nonInteractiveStorage(t)
	for _, command := range []string{"tui", "web", "data"} {
		for _, source := range []string{"default", "config", "environment", "flag"} {
			t.Run(command+"/"+source, func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("XDG_DATA_HOME", root)
				t.Setenv("XDG_CONFIG_HOME", root)
				for _, key := range configurationEnvironment {
					t.Setenv(key, "")
				}
				t.Setenv("TOKENINSIGHTS_HOST", "127.0.0.1")
				collectorPath := filepath.Join(root, "tokeninsights", "collector.sqlite")
				configPath := filepath.Join(root, "config.json")
				args := []string{"--config-file", configPath, command}
				if command == "data" {
					args = append(args, "wait")
				} else {
					args = append(args, "--capture=false")
				}
				if command == "web" {
					args = append(args, "--open=false")
				}
				if source != "default" {
					collectorPath = filepath.Join(root, "custom collector.sqlite")
				}
				switch source {
				case "config":
					if err := os.WriteFile(configPath, []byte(`{"collector":{"db-path":"custom collector.sqlite"}}`), 0o600); err != nil {
						t.Fatal(err)
					}
				case "environment":
					t.Setenv("TOKENINSIGHTS_COLLECTOR_DB_PATH", collectorPath)
				case "flag":
					args = append(args, "--collector-db-path", collectorPath)
				}
				if err := os.MkdirAll(filepath.Dir(collectorPath), 0o700); err != nil {
					t.Fatal(err)
				}
				database, _, err := collectordb.CreateIfMissing(collectorPath)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := database.Exec("PRAGMA user_version=19"); err != nil {
					t.Fatal(err)
				}
				if err := database.Close(); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(collectorPath)
				if err != nil {
					t.Fatal(err)
				}
				err = Run(t.Context(), args, io.Discard, io.Discard, time.Now())
				if err == nil {
					t.Fatal("older storage opened")
				}
				for _, want := range []string{"written by an older version", storageUnchanged, collectorPath, "Run this command in a terminal"} {
					if !strings.Contains(err.Error(), want) {
						t.Error(want, err)
					}
				}
				after, err := os.ReadFile(collectorPath)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("older evidence mutated", err)
				}
			})
		}
	}
}
