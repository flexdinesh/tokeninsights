package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestSyncDefaultsAllHarnesses(t *testing.T) {
	selected, err := syncHarnesses(false, nil)
	if err != nil || len(selected) != 4 {
		t.Fatalf("default selection: %v %v", selected, err)
	}
}

func TestLocalCaptureFullRefreshPreservesTotals(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	serverPath := filepath.Join(t.TempDir(), "server.sqlite")
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createOpenCodeSQLiteMessagesForCLI(t, sourcePath)
	setFileModTimeForCLI(t, sourcePath, now.Add(-72*time.Hour))

	var stdout bytes.Buffer
	err := captureForTest(t, ctx, dbPath, serverPath, sourceDir, now, false, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=1 quarantined=0") {
		t.Fatalf("unexpected first sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = captureForTest(t, ctx, dbPath, serverPath, sourceDir, now.Add(time.Hour), false, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=0 skipped=1 failed=0 raw_facts=0 quarantined=0") {
		t.Fatalf("unexpected skipped sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = captureForTest(t, ctx, dbPath, serverPath, sourceDir, now.Add(2*time.Hour), true, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=0 quarantined=0") {
		t.Fatalf("unexpected full-refresh output: %q", stdout.String())
	}
	store, err := datastore.Open(ctx, serverPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	assertCLIQueryCount(t, store.SQL(), "SELECT COUNT(*) FROM analytics_confirmed", 1)
	assertCLIQueryCount(t, store.SQL(), "SELECT SUM(total_tokens) FROM analytics_confirmed", 150)
	if !strings.Contains(stdout.String(), "delivery: status=accepted batches=0 accepted=0 pending=0 processing=async") {
		t.Fatalf("unchanged sync grew publication: %q", stdout.String())
	}
}

func captureForTest(t *testing.T, ctx context.Context, collectorPath, dataPath, source string, now time.Time, full bool, stdout io.Writer) error {
	t.Helper()
	local, err := localruntime.Open(ctx, collectorPath, dataPath)
	if err != nil {
		return err
	}
	defer func() { _ = local.Close() }()
	result, err := collector.Run(ctx, collector.Options{CollectorDBPath: collectorPath, ServerDBPath: dataPath, Destination: local.Destination,
		SyncOptions: pipeline.SyncOptions{Harnesses: []pipeline.Harness{pipeline.HarnessOpenCode}, SourceDir: source, Now: now, FullRefresh: full}})
	printSummary(stdout, "sync", result.Collection, false)
	printDeliverySummary(stdout, result)
	if err != nil {
		return err
	}
	return local.WaitVisible(ctx)
}
