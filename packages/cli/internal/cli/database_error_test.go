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

func TestLocalIncompatibleApplicationDatabaseGuidance(t *testing.T) {
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
				if !errors.Is(err, appstore.ErrIncompatibleDatabase) {
					t.Fatalf("expected incompatibility: %v", err)
				}
				for _, want := range []string{"unsupported schema version or database role", "automatic migrations", "Config: " + configPath, "Application database: " + appPath, "Close TokenInsights", "delete the application database", "-wal", "-shm", "Rerun", "resets local accounts and credentials", "Do not delete the config"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("guidance missing %q: %v", want, err)
					}
				}
				after, err := os.ReadFile(appPath)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("incompatible database mutated", err)
				}
				if err := os.Remove(appPath); err != nil {
					t.Fatal(err)
				}
				// The advertised recovery must work with the token database left intact.
				recovery := []string{"--config-file", configPath, "data", "wait", "--app-db-path", appPath}
				if err := Run(t.Context(), recovery, io.Discard, io.Discard, time.Now()); err != nil {
					t.Fatalf("deleting incompatible app did not recover: %v", err)
				}
			})
		}
	}
}

func TestPairedApplicationIncompatibilityRetainsBackupRecovery(t *testing.T) {
	root := t.TempDir()
	collectorPath, dataPath, appPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite"), filepath.Join(root, "app.sqlite")
	runtime, err := localruntime.OpenWithApp(t.Context(), collectorPath, dataPath, appPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.App.SQL().ExecContext(t.Context(), "PRAGMA user_version=1"); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{"--config-file", filepath.Join(root, "config.json"), "data", "wait", "--collector-db-path", collectorPath, "--server-db-path", dataPath, "--app-db-path", appPath}
	err = Run(t.Context(), args, io.Discard, io.Discard, time.Now())
	if err == nil || err.Error() != "paired_application_mismatch_restore_backup" {
		t.Fatalf("pair guard recovery changed: %v", err)
	}
}
