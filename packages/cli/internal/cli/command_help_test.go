package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestHelpAliases(t *testing.T) {
	for _, alias := range []string{"help", "--help", "-h"} {
		t.Run(alias, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := Run(context.Background(), []string{alias}, &stdout, io.Discard, time.Now()); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), "usage: tokeninsights <command>") {
				t.Fatalf("expected usage in output: %q", stdout.String())
			}
		})
	}
}

func TestHelpListsCurrentCommands(t *testing.T) {
	help := usageText()
	for _, name := range []string{"tui", "web", "browse", "sync --debug", "sync --print", "sync status", "config set", "data reprocess|wait", "tokeninsights-server"} {
		if !strings.Contains(help, name) {
			t.Fatalf("help missing %q", name)
		}
	}
	for _, name := range []string{"service stop", "collector normalize", "reset-canonical", "reset-all", "server run", "Deprecated aliases"} {
		if strings.Contains(help, name) {
			t.Fatalf("help advertises removed %q", name)
		}
	}
}
