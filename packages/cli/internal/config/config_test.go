package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func TestResolvedModeAndDestinationContract(t *testing.T) {
	for _, scenario := range []struct {
		name, fileMode, environmentMode, flagMode, url, token, wantMode string
		valid                                                           bool
	}{
		{name: "default local", wantMode: SingleProcess, valid: true},
		{name: "URL selects remote", url: "https://remote.example", token: "token", wantMode: Distributed, valid: true},
		{name: "URL without token cannot fall back", url: "https://remote.example", wantMode: Distributed},
		{name: "token without URL rejects", token: "token", wantMode: SingleProcess},
		{name: "explicit local rejects remote settings", fileMode: SingleProcess, url: "https://remote.example", token: "token", wantMode: SingleProcess},
		{name: "explicit remote requires URL", fileMode: Distributed, token: "token", wantMode: Distributed},
		{name: "environment overrides file", fileMode: SingleProcess, environmentMode: Distributed, url: "https://remote.example", token: "token", wantMode: Distributed, valid: true},
		{name: "flag overrides environment", fileMode: Distributed, environmentMode: Distributed, flagMode: SingleProcess, wantMode: SingleProcess, valid: true},
		{name: "remote flag requires credentials", flagMode: Distributed, wantMode: Distributed},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv("TOKENINSIGHTS_MODE", scenario.environmentMode)
			t.Setenv("TOKENINSIGHTS_SERVER_URL", scenario.url)
			t.Setenv("TOKENINSIGHTS_ACCESS_TOKEN", scenario.token)
			path := filepath.Join(t.TempDir(), "config.json")
			if scenario.fileMode != "" {
				if err := Update(t.Context(), path, func(v *Values) error { return v.Set("mode", scenario.fileMode, false) }); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.environmentMode == "" {
				if err := os.Unsetenv("TOKENINSIGHTS_MODE"); err != nil {
					t.Fatal(err)
				}
			}
			overrides := Values{}
			if scenario.flagMode != "" {
				overrides.Mode = &scenario.flagMode
			}
			settings, err := ResolveWithOverrides(path, true, overrides)
			if err != nil {
				t.Fatal(err)
			}
			if settings.EffectiveMode() != scenario.wantMode {
				t.Fatalf("mode %q, want %q", settings.EffectiveMode(), scenario.wantMode)
			}
			wantKind := serverfeatures.Personal
			if scenario.wantMode == Distributed {
				wantKind = serverfeatures.Hosted
			}
			if settings.ExpectedKind() != wantKind {
				t.Fatalf("mode %q selected kind %q", scenario.wantMode, settings.ExpectedKind())
			}
			if err := settings.ValidateDestination(); (err == nil) != scenario.valid {
				t.Fatalf("destination error %v, want valid %v", err, scenario.valid)
			}
		})
	}
}

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
