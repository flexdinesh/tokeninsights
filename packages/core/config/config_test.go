package config

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLocalStoragePathsAndExplicitPrecedence(t *testing.T) {
	path := writeConfig(t, `{"collector":{"db-path":"collector.sqlite"},"in-process":{"server-db-path":"server.sqlite","port":1234}}`)
	saved, err := ResolveLocal(path, false, Overrides{})
	if err != nil || saved.ServerDBPath != filepath.Join(filepath.Dir(path), "server.sqlite") || !saved.Endpoint.IsLocal() {
		t.Fatal(saved, err)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", "/environment/server.sqlite")
	t.Setenv("TOKENINSIGHTS_PORT", "invalid")
	port, data := 0, "/flag/server.sqlite"
	selected, err := ResolveLocal(path, true, Overrides{Port: &port, ServerDBPath: &data})
	if err != nil || selected.ServerDBPath != data || selected.Port != 0 {
		t.Fatal(selected, err)
	}
}

func TestObsoleteSettingsRequireManualRepairWithoutMutationOrSecrets(t *testing.T) {
	for _, body := range []string{
		`{"distributed":{"server-token":"private-secret"}}`,
		`{"in-process":{"app-db-path":"private-secret"}}`,
		`{"remote":{"endpoint":"ssh://private-secret"}}`,
		`{"viewer":{"open":"private-secret"}}`,
	} {
		path := writeConfig(t, body)
		if _, err := ResolveLocal(path, false, Overrides{}); err == nil || strings.Contains(err.Error(), "private-secret") || !strings.Contains(err.Error(), "config unchanged") || !strings.Contains(err.Error(), "viewer.open") {
			t.Fatal(err)
		}
		if err := Update(t.Context(), path, func(v *Values) error { return v.Set("remote.command", "/opt/tokeninsights", false) }); err == nil {
			t.Fatal("obsolete config rewritten")
		}
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, []byte(body)) {
			t.Fatal("config mutated", err)
		}
	}
	for _, key := range []string{"TOKENINSIGHTS_ENDPOINT", "TOKENINSIGHTS_APP_DB_PATH", "TOKENINSIGHTS_SERVER_URL", "TOKENINSIGHTS_ACCESS_TOKEN"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "private-secret")
			if _, err := ResolveLocal(filepath.Join(t.TempDir(), "missing.json"), true, Overrides{}); err == nil || strings.Contains(err.Error(), "private-secret") || !strings.Contains(err.Error(), key) {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectionIsInvocationOnlyAndSSHCommandIsLocalPreference(t *testing.T) {
	path := writeConfig(t, `{"remote":{"command":"/opt/tokeninsights"}}`)
	t.Setenv("SSH_CONNECTION", "192.0.2.1 123 192.0.2.2 22")
	local, err := ResolveLocal(path, true, Overrides{})
	if err != nil || !local.Endpoint.IsLocal() {
		t.Fatal(local, err)
	}
	target := "ssh://alice@example.test:2222"
	remote, err := ResolveLocal(path, false, Overrides{Connection: &target})
	if err != nil || remote.Endpoint.SSH == nil || remote.Endpoint.SSH.Command != "/opt/tokeninsights" {
		t.Fatal(remote, err)
	}
	data := "/cannot/apply.sqlite"
	if _, err := ResolveLocal(path, false, Overrides{Connection: &target, ServerDBPath: &data}); err == nil {
		t.Fatal("remote accepted local storage override")
	}
}

func TestViewerOpenIsNotAConfigKey(t *testing.T) {
	var v Values
	if err := v.Set("viewer.open", "mygo", false); err == nil {
		t.Fatal("accepted viewer.open")
	}
	if _, err := v.Get("viewer.open", t.TempDir()); err == nil {
		t.Fatal("read viewer.open")
	}
}

func TestConfigRejectsMalformedDuplicateAndUnknownDocuments(t *testing.T) {
	for _, body := range []string{`{"remote":{"command":"first","command":"second"}}`, `{"unknown":"private"}`, `{"remote":null}`, `{}{}`, `[]`} {
		path := writeConfig(t, body)
		if _, err := Read(path); err == nil {
			t.Fatal("accepted", body)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != body {
			t.Fatal("changed invalid config", err)
		}
	}
}

func TestConcurrentSettersPreserveUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	var workers sync.WaitGroup
	for key, value := range map[string]string{"in-process.host": "127.0.0.2", "remote.command": "/opt/tokeninsights", "in-process.port": "0"} {
		workers.Go(func() {
			if err := Update(context.Background(), path, func(v *Values) error { return v.Set(key, value, false) }); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	values, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]string{"in-process.host": "127.0.0.2", "remote.command": "/opt/tokeninsights", "in-process.port": "0"} {
		actual, err := values.Get(key, filepath.Dir(path))
		if err != nil || actual != expected {
			t.Fatal(key, actual, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("config permissions", err)
	}
}

func TestDefaultPathsPreservePersonalHistory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)
	defaults := Defaults()
	if defaults.CollectorDBPath != filepath.Join(root, "tokeninsights", "collector.sqlite") || defaults.ServerDBPath != filepath.Join(root, "tokeninsights", "server.sqlite") || defaults.Port != 0 {
		t.Fatal(defaults)
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", root)
	if Defaults().ServerDBPath != filepath.Join(root, ".local/share/tokeninsights/server.sqlite") {
		t.Fatal(Defaults())
	}
}

func TestFlagOverridesOnlyExplicitValues(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.String("connection", "local", "")
	flags.String("server-db-path", "/default", "")
	flags.Int("port", 42, "")
	if err := flags.Parse([]string{"--port=0", "--connection=ssh://host"}); err != nil {
		t.Fatal(err)
	}
	overrides, err := FlagOverrides(flags)
	if err != nil || overrides.ServerDBPath != nil || overrides.Port == nil || *overrides.Port != 0 || overrides.Connection == nil || *overrides.Connection != "ssh://host" {
		t.Fatal(overrides, err)
	}
}
