package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "tokeninsights-cli-config-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", root); err != nil {
		panic(err)
	}
	if err := os.Setenv("TOKENINSIGHTS_CONFIG_PATH", filepath.Join(root, "config.json")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
