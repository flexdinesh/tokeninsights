package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResetAllWithoutConfirmDoesNotCreateDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"reset-all", "--db-path", dbPath}, &stdout, io.Discard, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Re-run with --confirm") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
	entries, err := os.ReadDir(filepath.Dir(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("reset preview created files: %v", entries)
	}
}
