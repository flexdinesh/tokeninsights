package cli

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "__sync-run" {
		if err := syncjob.Child(context.Background()); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
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
