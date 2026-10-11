package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestCommandScopedResolution(t *testing.T) {
	path := writeConfig(t, `{"collector":{"db-path":"collector.sqlite"},"in-process":{"port":0,"server-db-path":"server.sqlite"},"distributed":{"server-url":"https://remote.example","server-token":"token"}}`)
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "invalid-remote-url")
	t.Setenv("TOKENINSIGHTS_ACCESS_TOKEN", "invalid remote token")
	local, err := ResolveLocal(path, true, Overrides{})
	if err != nil || local.Port != 0 || local.ServerDBPath != filepath.Join(filepath.Dir(path), "server.sqlite") {
		t.Fatal(local, err)
	}
	remote, err := ResolveSync(path, true, Overrides{})
	if err != nil || remote.Validate() == nil {
		t.Fatal(remote, err)
	}
	t.Setenv("TOKENINSIGHTS_PORT", "invalid-local-port")
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", " ")
	url, token := "https://flag.example", "flag-token"
	remote, err = ResolveSync(path, true, Overrides{ServerURL: &url, ServerToken: &token})
	if err != nil || remote.Validate() != nil || remote.ServerURL != url || remote.CollectorDBPath != local.CollectorDBPath {
		t.Fatal(remote, err)
	}
	browse, err := ResolveBrowse(path, true, Overrides{ServerURL: &url})
	if err != nil || browse.Validate() != nil {
		t.Fatal(browse, err)
	}
}
func TestBrowseRequiresURLButNoTokenOrStorage(t *testing.T) {
	path := writeConfig(t, `{"in-process":{"host":"invalid","port":-1},"distributed":{"server-url":"https://remote.example","server-token":"invalid token"}}`)
	browse, err := ResolveBrowse(path, false, Overrides{})
	if err != nil || browse.Validate() != nil {
		t.Fatal(browse, err)
	}
	remote, err := ResolveSync(path, false, Overrides{})
	if err != nil || remote.Validate() == nil {
		t.Fatal(remote, err)
	}
	if _, err := ResolveLocal(path, false, Overrides{}); err == nil {
		t.Fatal("invalid local config accepted")
	}
}
func TestConfigValidationAndNoMutation(t *testing.T) {
	for _, body := range []string{
		"null", "[]", `{"host":"127.0.0.1"}`, `{"mode":"distributed"}`,
		`{"in-process":null}`, `{"in-process":{"host":null}}`,
		`{"in-process":{"port":"8765"}}`, `{"in-process":{"port":1,"port":2}}`,
		`{"distributed":{"server-url":"https://first","server-url":"https://second"}}`,
		`{"distributed":{"unknown":true}}`, `{"unknown":{}}`, `{} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			path := writeConfig(t, body)
			if _, err := Read(path); err == nil {
				t.Fatal("invalid config accepted")
			}
			if err := Update(t.Context(), path, func(v *Values) error { return v.Set("in-process.port", "0", false) }); err == nil {
				t.Fatal("invalid config overwritten")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != body {
				t.Fatal("config changed", err)
			}
		})
	}
}
func TestExplicitOverridesWinIncludingZeroAndEmpty(t *testing.T) {
	path := writeConfig(t, `{"distributed":{"server-url":"http://file.test"},"in-process":{"port":1234}}`)
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "invalid-environment-url")
	t.Setenv("TOKENINSIGHTS_PORT", "invalid-environment-port")
	url, port := "", 0
	local, err := ResolveLocal(path, true, Overrides{Port: &port})
	if err != nil || local.Port != 0 {
		t.Fatal(local, err)
	}
	browse, err := ResolveBrowse(path, true, Overrides{ServerURL: &url})
	if err != nil || browse.ServerURL != "" || browse.Validate() == nil {
		t.Fatal(browse, err)
	}
}
func TestConcurrentSettersPreserveUpdatesAndAtomicReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Update(t.Context(), path, func(v *Values) error { return v.Set("in-process.port", "8765", false) }); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var readers, writers sync.WaitGroup
	errs := make(chan error, 3)
	readers.Go(func() {
		for ctx.Err() == nil {
			if _, err := Read(path); err != nil {
				errs <- err
				return
			}
		}
	})
	for key, value := range map[string]string{"in-process.host": "0.0.0.0", "distributed.server-url": "http://fixture.test"} {
		writers.Go(func() {
			if err := Update(ctx, path, func(v *Values) error { return v.Set(key, value, false) }); err != nil {
				errs <- err
			}
		})
	}
	writers.Wait()
	cancel()
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	values, err := Read(path)
	if err != nil || values.InProcess.Host == nil || *values.InProcess.Host != "0.0.0.0" || values.Distributed.ServerURL == nil || *values.Distributed.ServerURL != "http://fixture.test" || values.InProcess.Port == nil || *values.InProcess.Port != 8765 {
		t.Fatal(values, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private config", err)
	}
	if _, err := values.Get("mode", filepath.Dir(path)); err == nil {
		t.Fatal("mode remains supported")
	}
	if err := values.Set("distributed.server-token", "secret token", false); err == nil || strings.Contains(err.Error(), "secret token") {
		t.Fatal("invalid secret", err)
	}
}
