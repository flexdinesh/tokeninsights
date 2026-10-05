package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigCommandsPersistPreferencesWithoutService(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "http://environment.test:8765")
	t.Setenv("TOKENINSIGHTS_SERVER_TOKEN", "")
	var output bytes.Buffer
	run := func(args ...string) error {
		output.Reset()
		return Run(context.Background(), append([]string{"--config-file", path, "config"}, args...), &output, &output, time.Now())
	}
	if err := run("get", "server-url"); err != nil || output.String() != "\n" {
		t.Fatalf("default: %q %v", output.String(), err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("get created config", err)
	}
	if err := run("set", "server-url", "http://fixture.test:8765"); err != nil {
		t.Fatal(err)
	}
	if err := run("get", "server-url"); err != nil || output.String() != "http://fixture.test:8765\n" {
		t.Fatal(output.String(), err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"set", "port", "65536"}, {"set", "unknown", "value"}, {"set", "server-url", "http://user:secret@fixture.test"}, {"get", "host", "extra"}} {
		if err := run(args...); err == nil {
			t.Fatal("accepted", args)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, before) {
			t.Fatal("invalid command mutated config", err)
		}
	}
	if err := run("remove", "server-url"); err != nil {
		t.Fatal(err)
	}
	if err := run("get", "server-url"); err != nil || output.String() != "\n" {
		t.Fatal(output.String(), err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "sqlite") || entry.IsDir() {
			t.Fatal("config created service state", entry.Name())
		}
	}
}

func TestMalformedConfigHelpAndVersionRemainSideEffectFree(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte("malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"--version"}, {"sync", "--help"}, {"tui", "--help"}, {"config", "--help"}} {
		if err := Run(t.Context(), append([]string{"--config-file", path}, args...), io.Discard, io.Discard, time.Now()); err != nil {
			t.Fatal(args, err)
		}
	}
	if err := Run(t.Context(), []string{"--config-file", path, "sync", "--dry-run"}, io.Discard, io.Discard, time.Now()); err == nil {
		t.Fatal("malformed config accepted")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("help/config validation created state", entries, err)
	}
}

func TestPositionalHelpDoesNotBypassConfigValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"help", "version"} {
		err := Run(t.Context(), []string{"--config-file", path, "sync", "--source-dir", name, "--dry-run"}, io.Discard, io.Discard, time.Now())
		if err == nil || !strings.Contains(err.Error(), "configuration:") {
			t.Fatal("source directory bypassed configuration", name, err)
		}
	}
}

func TestConfiguredRemoteQueryOnlyTUIUsesSameDestination(t *testing.T) {
	remote, store, counts := newViewQueryServer(t, true)
	defer remote.Close()
	defer func() { _ = store.Close() }()
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	collectorPath := filepath.Join(root, "absent-collector.sqlite")
	body := fmt.Sprintf(`{"server-url":%q,"collector-db-path":%q,"server-db-path":%q}`, remote.URL, collectorPath, filepath.Join(root, "absent-server.sqlite"))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "")
	if err := os.Unsetenv("TOKENINSIGHTS_SERVER_URL"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_TOKEN", "")
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		if model.options.serverURL != remote.URL || model.options.syncBeforeView {
			t.Fatal("wrong viewer routing", model.options)
		}
		data := model.loadServerDashboard()
		if data.err != nil {
			t.Fatal(data.err)
		}
		return model, nil
	})
	defer restore()
	if err := Run(t.Context(), []string{"--config-file", path, "tui", "--sync=false"}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
	if counts.posts.Load() != 0 || counts.gets.Load() == 0 {
		t.Fatal("query-only viewer wrote", counts)
	}
	if _, err := os.Stat(collectorPath); !os.IsNotExist(err) {
		t.Fatal("query-only viewer opened collector", err)
	}
}
