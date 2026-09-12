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
