package config

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
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

func TestDefaultsUseXDGThenHomeAndIgnoreLegacyDatabase(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)
	t.Setenv("TOKENINSIGHTS_DB_PATH", filepath.Join(root, "legacy-override.sqlite"))
	legacy := filepath.Join(root, "tokeninsights", "tokeninsights.sqlite")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy-history"), 0o600); err != nil {
		t.Fatal(err)
	}
	defaults := Defaults()
	if defaults.CollectorDBPath != filepath.Join(root, "tokeninsights", "collector.sqlite") || defaults.ServerDBPath != filepath.Join(root, "tokeninsights", "server.sqlite") {
		t.Fatal("role defaults used legacy storage", defaults)
	}
	if value, err := os.ReadFile(legacy); err != nil || string(value) != "legacy-history" {
		t.Fatal("legacy changed")
	}
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", " ")
	t.Setenv("HOME", home)
	if got := Defaults().CollectorDBPath; got != filepath.Join(home, ".local", "share", "tokeninsights", "collector.sqlite") {
		t.Fatal("home fallback", got)
	}
	t.Setenv("HOME", "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := Defaults().ServerDBPath; got != filepath.Join(cwd, ".tokeninsights-data", "server.sqlite") {
		t.Fatal("working directory fallback", got)
	}
}

func TestEnvironmentDefaultsApplyRolePathsIndependently(t *testing.T) {
	t.Setenv("TOKENINSIGHTS_COLLECTOR_DB_PATH", "/fixture/collector.sqlite")
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", "/fixture/server.sqlite")
	settings := EnvironmentDefaults()
	if settings.CollectorDBPath != "/fixture/collector.sqlite" || settings.ServerDBPath != "/fixture/server.sqlite" {
		t.Fatal("role environment selection", settings)
	}
	t.Setenv("TOKENINSIGHTS_PORT", "not-a-port")
	if settings := EnvironmentDefaults(); settings.Port != Defaults().Port {
		t.Fatal("invalid environment broke help defaults", settings)
	}
}

func TestFlagOverridesUseOnlyExplicitFlags(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	collector := flags.String("collector-db-path", "/default/collector.sqlite", "")
	flags.String("server-db-path", "/default/server.sqlite", "")
	flags.String("host", "", "")
	flags.Int("port", 0, "")
	flags.Bool("sync", true, "")
	if err := flags.Parse([]string{"-collector-db-path", "/flag/collector.sqlite", "--host=", "--port", "9000", "--sync=false"}); err != nil {
		t.Fatal(err)
	}
	overrides, err := FlagOverrides(flags)
	if err != nil {
		t.Fatal(err)
	}
	if overrides.CollectorDBPath == nil || *overrides.CollectorDBPath != *collector {
		t.Fatalf("collector override = %v", overrides.CollectorDBPath)
	}
	if overrides.ServerDBPath != nil || overrides.AppDBPath != nil || overrides.ServerURL != nil {
		t.Fatalf("unset flags became overrides: %+v", overrides)
	}
	if overrides.Host == nil || *overrides.Host != Defaults().Host {
		t.Fatalf("empty host override = %v, want default host", overrides.Host)
	}
	if overrides.Port == nil || *overrides.Port != 9000 {
		t.Fatalf("port override = %v", overrides.Port)
	}
}

func TestFlagOverridesTakePrecedenceOverInvalidEnvironment(t *testing.T) {
	t.Setenv("TOKENINSIGHTS_PORT", "not-a-port")
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.Int("port", 0, "")
	if err := flags.Parse([]string{"-port", "0"}); err != nil {
		t.Fatal(err)
	}
	overrides, err := FlagOverrides(flags)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveLocal(filepath.Join(t.TempDir(), "config.json"), true, overrides); err != nil {
		t.Fatalf("single-dash --port did not override invalid environment: %v", err)
	}
}

func TestViewerOpenPrecedenceAndValidation(t *testing.T) {
	empty := writeConfig(t, `{}`)
	local, err := ResolveLocal(empty, false, Overrides{})
	if err != nil || local.Open != browser.ModeAuto {
		t.Fatal("default open mode", local.Open, err)
	}
	saved := writeConfig(t, `{"viewer":{"open":"browser"},"distributed":{"server-url":"https://remote.example"}}`)
	local, err = ResolveLocal(saved, true, Overrides{})
	if err != nil || local.Open != browser.ModeBrowser {
		t.Fatal("file open mode", local.Open, err)
	}
	flagValue := "app"
	browse, err := ResolveBrowse(saved, true, Overrides{Open: &flagValue})
	if err != nil || browse.Open != browser.ModeApp {
		t.Fatal("flag open mode", browse.Open, err)
	}
	invalid := writeConfig(t, `{"viewer":{"open":"sometimes"},"distributed":{"server-url":"https://remote.example"}}`)
	if _, err := ResolveLocal(invalid, false, Overrides{}); err == nil || !strings.Contains(err.Error(), "viewer.open") {
		t.Fatal("invalid file open mode accepted", err)
	}
	if _, err := ResolveBrowse(invalid, false, Overrides{}); err == nil {
		t.Fatal("invalid file open mode accepted by browse")
	}
	if browse, err := ResolveBrowse(invalid, false, Overrides{Open: &flagValue}); err != nil || browse.Open != browser.ModeApp {
		t.Fatal("flag did not override invalid file value", err)
	}
	var values Values
	for _, test := range []struct{ set, want string }{{"BROWSER", "browser"}, {"true", "auto"}, {"false", "false"}, {"mygo", "mygo"}, {"electron", "electron"}, {"auto", "auto"}, {"app", "mygo"}} {
		if err := values.Set("viewer.open", test.set, false); err != nil {
			t.Fatal(err)
		}
		if got, err := values.Get("viewer.open", ""); err != nil || got != test.want {
			t.Fatalf("get after set %q = %q, %v", test.set, got, err)
		}
	}
	if err := values.Set("viewer.open", "sometimes", false); err == nil || *values.Viewer.Open != "mygo" {
		t.Fatal("invalid set accepted or mutated")
	}
	if err := values.Set("viewer.open", "", true); err != nil || values.Viewer.Open != nil {
		t.Fatal("remove failed", err)
	}
	if got, _ := values.Get("viewer.open", ""); got != "auto" {
		t.Fatal("removed open mode", got)
	}
}

func TestRemoteEndpointPrecedenceLocalEscapeAndCommand(t *testing.T) {
	path := writeConfig(t, `{"remote":{"endpoint":"ssh://file-host","command":"/opt/ti/tokeninsights"}}`)
	t.Setenv("TOKENINSIGHTS_ENDPOINT", "")
	t.Setenv("TOKENINSIGHTS_REMOTE_COMMAND", "")
	_ = os.Unsetenv("TOKENINSIGHTS_ENDPOINT")
	_ = os.Unsetenv("TOKENINSIGHTS_REMOTE_COMMAND")
	settings, err := ResolveLocal(path, true, Overrides{})
	if err != nil || settings.Endpoint.String() != "ssh://file-host" || settings.Endpoint.SSH.Command != "/opt/ti/tokeninsights" {
		t.Fatal("file endpoint not resolved", settings.Endpoint, err)
	}
	t.Setenv("TOKENINSIGHTS_ENDPOINT", "ssh://env-host:2222")
	t.Setenv("TOKENINSIGHTS_REMOTE_COMMAND", "ti")
	settings, err = ResolveLocal(path, true, Overrides{})
	if err != nil || settings.Endpoint.String() != "ssh://env-host:2222" || settings.Endpoint.SSH.Command != "ti" {
		t.Fatal("environment did not override file", settings.Endpoint, err)
	}
	flagValue := "ssh://dee@flag-host"
	settings, err = ResolveLocal(path, true, Overrides{Endpoint: &flagValue})
	if err != nil || settings.Endpoint.String() != flagValue {
		t.Fatal("flag did not override environment", settings.Endpoint, err)
	}
	local := "local"
	settings, err = ResolveLocal(path, true, Overrides{Endpoint: &local})
	if err != nil || !settings.Endpoint.IsLocal() {
		t.Fatal("--endpoint=local did not force local", settings.Endpoint, err)
	}
	invalid := "tcp://box:8765"
	if _, err := ResolveLocal(path, true, Overrides{Endpoint: &invalid}); err == nil {
		t.Fatal("tcp endpoint accepted")
	}
	server := "/tmp/other.sqlite"
	if _, err := ResolveLocal(path, true, Overrides{ServerDBPath: &server}); err == nil || !strings.Contains(err.Error(), "rejects local database path flags") {
		t.Fatal("remote endpoint accepted local storage flag", err)
	}
	if settings, err := ResolveLocal(path, true, Overrides{Endpoint: &local, ServerDBPath: &server}); err != nil || settings.ServerDBPath != server {
		t.Fatal("local escape rejected storage flag", err)
	}
	t.Setenv("TOKENINSIGHTS_REMOTE_COMMAND", "bad\ncommand")
	if _, err := ResolveLocal(path, true, Overrides{}); err == nil {
		t.Fatal("control characters in remote command accepted")
	}
	// Sync and browse never consume remote endpoint settings.
	if _, err := ResolveSync(path, true, Overrides{}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteConfigSetGetRemoveCanonicalizes(t *testing.T) {
	var values Values
	if err := values.Set("remote.endpoint", "ssh://dee@box:22/", false); err != nil {
		t.Fatal(err)
	}
	if got, err := values.Get("remote.endpoint", ""); err != nil || got != "ssh://dee@box:22" {
		t.Fatal("endpoint not canonical", got, err)
	}
	for _, bad := range []string{"tcp://box", "ssh://-oProxyCommand=x"} {
		if err := values.Set("remote.endpoint", bad, false); err == nil {
			t.Fatal("invalid endpoint saved", bad)
		}
	}
	if err := values.Set("remote.command", "/opt/ti/tokeninsights", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := values.Get("remote.command", ""); got != "/opt/ti/tokeninsights" {
		t.Fatal("remote command not saved", got)
	}
	if err := values.Set("remote.endpoint", "", true); err != nil {
		t.Fatal(err)
	}
	if got, _ := values.Get("remote.endpoint", ""); got != "local" {
		t.Fatal("removed endpoint not local", got)
	}
}
