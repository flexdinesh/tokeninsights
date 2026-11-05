package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigCommandsPersistPreferenceWithoutBackend(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	var output bytes.Buffer
	run := func(args ...string) error {
		output.Reset()
		return Run(t.Context(), append([]string{"--config-file", path, "config"}, args...), &output, &output, time.Now())
	}
	if err := run("get", "remote.command"); err != nil || output.String() != "tokeninsights\n" {
		t.Fatal(output.String(), err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("get created config", err)
	}
	if err := run("set", "remote.command", "/opt/tokeninsights"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"set", "viewer.open", "mygo"}, {"set", "distributed.server-token", "private-secret"}, {"set", "remote.endpoint", "ssh://host"}, {"get", "remote.command", "extra"}} {
		if err := run(args...); err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatal(err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("invalid update mutated config", err)
		}
	}
	if err := run("remove", "remote.command"); err != nil {
		t.Fatal(err)
	}
	if err := run("get", "remote.command"); err != nil || output.String() != "tokeninsights\n" {
		t.Fatal(output.String(), err)
	}
}

func TestMalformedConfigHelpAndVersionRemainSideEffectFree(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte("malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"--version"}, {"tui", "--help"}, {"web", "--help"}, {"remote", "--help"}, {"config", "--help"}} {
		if err := Run(t.Context(), append([]string{"--config-file", path}, args...), io.Discard, io.Discard, time.Now()); err != nil {
			t.Fatal(args, err)
		}
	}
	if err := Run(t.Context(), []string{"--config-file", path, "--capture=false"}, io.Discard, io.Discard, time.Now()); err == nil {
		t.Fatal("malformed config accepted")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("validation created state", entries, err)
	}
}

func TestPositionalHelpDoesNotBypassConfigValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"help", "version"} {
		err := Run(t.Context(), []string{"--config-file", path, "web", "--source-dir", name, "--open=false"}, io.Discard, io.Discard, time.Now())
		if err == nil || !strings.Contains(err.Error(), "configuration:") {
			t.Fatal(name, err)
		}
	}
}
