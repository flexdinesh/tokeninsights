package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConfigValidationAndNoMutation(t *testing.T) {
	for _, body := range []string{"null", "[]", `{"host":null}`, `{"port":"8765"}`, `{"port":-1}`, `{"port":65536}`, `{"host":"example.test"}`, `{"server-url":"http://u:p@example.test"}`, `{"unknown":true}`, `{"port":1,"port":2}`, `{} {}`} {
		t.Run(body, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(path); err == nil {
				t.Fatal("invalid config accepted")
			}
			if err := Update(t.Context(), path, func(v *Values) error { return v.Set("port", "0", false) }); err == nil {
				t.Fatal("invalid config overwritten")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != body {
				t.Fatal("config changed", err)
			}
		})
	}
}

func TestResolutionPresenceAndRelativePaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte(`{"server-url":"http://file.test","host":"0.0.0.0","port":0,"collector-db-path":"collector.sqlite","server-db-path":"server.sqlite"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "")
	resolved, err := Resolve(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ServerURL != "" || resolved.Port != 0 || resolved.Host != "0.0.0.0" || resolved.CollectorDBPath != filepath.Join(root, "collector.sqlite") || resolved.ServerDBPath != filepath.Join(root, "server.sqlite") {
		t.Fatal(resolved)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "http://environment.test")
	t.Setenv("TOKENINSIGHTS_PORT", "1234")
	resolved, err = Resolve(path, true)
	if err != nil || resolved.ServerURL != "http://environment.test" || resolved.Port != 1234 {
		t.Fatal(resolved, err)
	}
	values, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := values.Get("server-url", root)
	if err != nil || stored != "http://file.test" {
		t.Fatal(stored, err)
	}
}

func TestConcurrentSettersPreserveUpdatesAndAtomicReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Update(t.Context(), path, func(v *Values) error { return v.Set("port", "8765", false) }); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var workers sync.WaitGroup
	errors := make(chan error, 3)
	workers.Go(func() {
		for ctx.Err() == nil {
			if _, err := Read(path); err != nil {
				errors <- err
				return
			}
		}
	})
	var writers sync.WaitGroup
	for key, value := range map[string]string{"host": "0.0.0.0", "server-url": "http://fixture.test"} {
		writers.Go(func() {
			if err := Update(ctx, path, func(v *Values) error { return v.Set(key, value, false) }); err != nil {
				errors <- err
			}
		})
	}
	writers.Wait()
	cancel()
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	values, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if values.Host == nil || *values.Host != "0.0.0.0" || values.ServerURL == nil || *values.ServerURL != "http://fixture.test" || values.Port == nil || *values.Port != 8765 {
		t.Fatal(values)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("private config", info, err)
	}
}

func TestExplicitOverridesWinIncludingZeroAndEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server-url":"http://file.test","port":1234}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "invalid-environment-url")
	t.Setenv("TOKENINSIGHTS_PORT", "invalid-environment-port")
	url, port := "", 0
	settings, err := ResolveWithOverrides(path, true, Values{ServerURL: &url, Port: &port})
	if err != nil || settings.ServerURL != "" || settings.Port != 0 {
		t.Fatal("explicit overrides", settings, err)
	}
}
