package cli

import (
	"bytes"
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncDefaultsAllHarnesses(t *testing.T) {
	selected, err := syncHarnesses(false, nil)
	if err != nil || len(selected) != 4 {
		t.Fatalf("default selection: %v %v", selected, err)
	}
}

func TestSyncFullRefreshFlagForcesSourceRefresh(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	serverPath := filepath.Join(t.TempDir(), "server.sqlite")
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createOpenCodeSQLiteMessagesForCLI(t, sourcePath)
	setFileModTimeForCLI(t, sourcePath, now.Add(-72*time.Hour))

	var stdout bytes.Buffer
	err := Run(ctx, []string{"sync", "--collector-db-path", dbPath, "--server-db-path", serverPath, "--harness", "opencode", "--source-dir", sourceDir}, &stdout, io.Discard, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=1 observations=0 canonical=0 diagnostics=0") {
		t.Fatalf("unexpected first sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = Run(ctx, []string{"sync", "--collector-db-path", dbPath, "--server-db-path", serverPath, "--harness", "opencode", "--source-dir", sourceDir}, &stdout, io.Discard, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=0 skipped=1 failed=0 raw_facts=0 observations=0 canonical=0 diagnostics=0") {
		t.Fatalf("unexpected skipped sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = Run(ctx, []string{"sync", "--collector-db-path", dbPath, "--server-db-path", serverPath, "--harness", "opencode", "--source-dir", sourceDir, "--full-refresh"}, &stdout, io.Discard, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=0 observations=0 canonical=0 diagnostics=0") {
		t.Fatalf("unexpected full-refresh output: %q", stdout.String())
	}
	store, err := datastore.Open(ctx, serverPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	assertCLIQueryCount(t, store.SQL(), "SELECT COUNT(*) FROM analytics.confirmed", 1)
	assertCLIQueryCount(t, store.SQL(), "SELECT SUM(total_tokens) FROM analytics.confirmed", 150)
	if !strings.Contains(stdout.String(), "delivery: status=accepted batches=0 accepted=0 pending=0 processing=async") {
		t.Fatalf("unchanged sync grew publication: %q", stdout.String())
	}
}
