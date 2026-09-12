package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncRequiresHarnessSelection(t *testing.T) {
	err := Run(context.Background(), []string{"sync", "--db-path", filepath.Join(t.TempDir(), "test.sqlite")}, io.Discard, io.Discard, time.Now())
	if err == nil || !strings.Contains(err.Error(), "choose --all or --harness") {
		t.Fatalf("expected harness selection error, got %v", err)
	}
}

func TestSyncFullRefreshFlagForcesSourceRefresh(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createOpenCodeSQLiteMessagesForCLI(t, sourcePath)
	setFileModTimeForCLI(t, sourcePath, now.Add(-72*time.Hour))

	var stdout bytes.Buffer
	err := Run(ctx, []string{"sync", "--db-path", dbPath, "--harness", "opencode", "--source-dir", sourceDir}, &stdout, io.Discard, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=1 observations=1 canonical=1 diagnostics=0") {
		t.Fatalf("unexpected first sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = Run(ctx, []string{"sync", "--db-path", dbPath, "--harness", "opencode", "--source-dir", sourceDir}, &stdout, io.Discard, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=0 skipped=1 failed=0 raw_facts=0 observations=0 canonical=0 diagnostics=0") {
		t.Fatalf("unexpected skipped sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = Run(ctx, []string{"sync", "--db-path", dbPath, "--harness", "opencode", "--source-dir", sourceDir, "--full-refresh"}, &stdout, io.Discard, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=0 observations=1 canonical=0 diagnostics=0") {
		t.Fatalf("unexpected full-refresh output: %q", stdout.String())
	}
}
