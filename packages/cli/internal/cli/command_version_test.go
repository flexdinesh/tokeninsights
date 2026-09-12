package cli

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestVersionAliases(t *testing.T) {
	for _, alias := range []string{"version", "--version"} {
		t.Run(alias, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := Run(context.Background(), []string{alias}, &stdout, io.Discard, time.Now()); err != nil {
				t.Fatal(err)
			}
			if got, want := stdout.String(), "tokeninsights dev\n"; got != want {
				t.Fatalf("version output = %q, want %q", got, want)
			}
		})
	}
}
