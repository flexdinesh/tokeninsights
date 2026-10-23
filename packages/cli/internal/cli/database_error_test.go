package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
)

// Without a terminal, older local storage is reported and never reset. The
// reported path follows every configuration source.
func TestLocalOlderStorageWithoutTerminalChangesNothing(t *testing.T) {
	nonInteractiveStorage(t)
	for _, command := range []string{"tui", "web", "data"} {
		for _, source := range []string{"default", "config", "environment", "flag"} {
			t.Run(command+"/"+source, func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("XDG_DATA_HOME", root)
				t.Setenv("XDG_CONFIG_HOME", root)
				for _, key := range []string{"TOKENINSIGHTS_CONFIG_PATH", "TOKENINSIGHTS_COLLECTOR_DB_PATH", "TOKENINSIGHTS_SERVER_DB_PATH", "TOKENINSIGHTS_APP_DB_PATH", "TOKENINSIGHTS_HOST", "TOKENINSIGHTS_PORT"} {
					t.Setenv(key, "")
				}
				t.Setenv("TOKENINSIGHTS_HOST", "127.0.0.1")
				configPath := filepath.Join(root, "tokeninsights", "config.json")
				appPath := filepath.Join(root, "tokeninsights", "app.sqlite")
				args := []string{command}
				if command == "data" {
					args = append(args, "wait")
				} else {
					args = append(args, "--sync=false")
				}
				if command == "web" {
					args = append(args, "--open=false", "--port=0")
				}
				if source != "default" {
					appPath = filepath.Join(root, "custom accounts.sqlite")
				}
				switch source {
				case "config":
					configPath = filepath.Join(root, "preferences.json")
					if err := os.WriteFile(configPath, []byte(`{"in-process":{"app-db-path":"custom accounts.sqlite"}}`), 0o600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--config-file", configPath)
				case "environment":
					t.Setenv("TOKENINSIGHTS_APP_DB_PATH", appPath)
					t.Setenv("TOKENINSIGHTS_CONFIG_PATH", configPath)
				case "flag":
					t.Chdir(root)
					args = append(args, "--app-db-path", "custom accounts.sqlite")
				}
				app, err := appstore.Open(t.Context(), appPath, "fixture", "personal")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := app.SQL().ExecContext(t.Context(), "PRAGMA user_version=1"); err != nil {
					_ = app.Close()
					t.Fatal(err)
				}
				if err := app.Close(); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(appPath)
				if err != nil {
					t.Fatal(err)
				}
				err = Run(t.Context(), args, io.Discard, io.Discard, time.Now())
				if err == nil {
					t.Fatal("older storage opened")
				}
				for _, want := range []string{"written by an older version", storageUnchanged, filepath.Base(appPath) + "   schema 1 -> 2   reset", "\n  " + appPath, "Run this command in a terminal"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("guidance missing %q: %v", want, err)
					}
				}
				after, err := os.ReadFile(appPath)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("older database mutated", err)
				}
			})
		}
	}
}

// Confirmed resets move the guard with its application; a missing application
// beside an intact guard still requires restoring the stopped backup set.
func TestPairGuardStillRejectsMissingApplication(t *testing.T) {
	nonInteractiveStorage(t)
	root := t.TempDir()
	collectorPath, dataPath, appPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite"), filepath.Join(root, "app.sqlite")
	runtime, err := localruntime.OpenWithApp(t.Context(), collectorPath, dataPath, appPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(appPath); err != nil {
		t.Fatal(err)
	}
	args := []string{"--config-file", filepath.Join(root, "config.json"), "data", "wait", "--collector-db-path", collectorPath, "--server-db-path", dataPath, "--app-db-path", appPath}
	err = Run(t.Context(), args, io.Discard, io.Discard, time.Now())
	if err == nil || err.Error() != "paired_application_mismatch_restore_backup" {
		t.Fatalf("pair guard recovery changed: %v", err)
	}
	if _, err := os.Stat(dataPath + ".application.json"); err != nil {
		t.Fatal("guard moved without a reset", err)
	}
	if _, err := os.Stat(appPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing application recreated", err)
	}
}
