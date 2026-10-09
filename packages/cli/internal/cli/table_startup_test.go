package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func replaceViewCollector(t *testing.T, run func(context.Context, collector.Options) (collector.Result, error)) {
	t.Helper()
	previous := runViewCollector
	runViewCollector = run
	t.Cleanup(func() { runViewCollector = previous })
}

func finishCollectionForTest(t *testing.T, model interactiveModel) interactiveModel {
	t.Helper()
	select {
	case result := <-model.collectionDone:
		updated, _ := model.Update(collectionFinishedMsg{result: result})
		return updated.(interactiveModel)
	case <-time.After(10 * time.Second):
		t.Fatal("background collection did not finish")
		return model
	}
}

func loadDashboardForTest(t *testing.T, model interactiveModel) interactiveModel {
	t.Helper()
	request := model
	request.ctx, request.requestID = model.queryContext(model.queries)
	message := request.loadDashboard()
	if message.err != nil {
		t.Fatal(message.err)
	}
	updated, _ := model.Update(message)
	return updated.(interactiveModel)
}

func loadRefreshStatusForTest(t *testing.T, model interactiveModel) interactiveModel {
	t.Helper()
	request := model
	request.ctx, request.requestID = model.queryContext(model.statusQueries)
	message := request.loadSharedSync()
	if message.err != nil {
		t.Fatal(message.err)
	}
	updated, _ := model.Update(message)
	return updated.(interactiveModel)
}

func TestTUIDefaultSyncShowsSavedUsageWhileCollectionRunsAndQuitCancels(t *testing.T) {
	options := localViewOptions(t, true)
	if err := options.local.Close(); err != nil {
		t.Fatal(err)
	}
	started, stopped := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	replaceViewCollector(t, func(ctx context.Context, _ collector.Options) (collector.Result, error) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		close(stopped)
		return collector.Result{}, ctx.Err()
	})
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("startup collection missing")
		}
		model.width, model.height = 120, 35
		model = loadDashboardForTest(t, model)
		if model.loading || len(model.rows) != 1 || model.rows[0].totalValue != 100 {
			t.Fatal("saved usage blocked on collection")
		}
		view := ansi.Strip(model.View())
		for _, text := range []string{"Total tokens", "Refreshing", "automatically"} {
			if !strings.Contains(view, text) {
				t.Fatalf("missing %q during refresh: %s", text, view)
			}
		}
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		model = updated.(interactiveModel)
		if cmd == nil {
			t.Fatal("query reload unavailable during collection")
		}
		updated, _ = model.Update(cmd())
		model = updated.(interactiveModel)
		if calls.Load() != 1 || len(model.rows) != 1 || model.rows[0].totalValue != 100 {
			t.Fatal("reload collected again or lost saved usage")
		}
		updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		if cmd == nil {
			t.Fatal("quit unavailable during collection")
		}
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Fatal("quit left collection running")
		}
		return updated.(interactiveModel), nil
	})
	defer restore()
	if err := Run(t.Context(), []string{"tui", "--all-time", "--collector-db-path", options.collectorDBPath, "--server-db-path", options.dbPath}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestTUIBackgroundIngestionAndReplayPreserveUsageAndReloadOnlyQueries(t *testing.T) {
	root := t.TempDir()
	isolateViewSources(t, root)
	piPath := filepath.Join(root, "home", ".pi", "agent", "sessions", "project", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(piPath), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `{"type":"session","id":"view-sync-session"}` + "\n" + `{"type":"message","id":"view-sync-request","message":{"role":"assistant","timestamp":1767225600000,"usage":{"input":80,"output":20,"totalTokens":100}}}` + "\n"
	if err := os.WriteFile(piPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	collectorPath, serverPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.duckdb")
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		model.width, model.height = 120, 35
		model = finishCollectionForTest(t, model)
		if model.refresh.result == nil || model.refresh.result.Err != nil {
			t.Fatalf("collection failed: %+v", model.refresh.result)
		}
		if strings.Contains(model.refreshLine(), "Usage refreshed") {
			t.Fatal("acceptance claimed query visibility")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		if err := model.options.local.WaitVisible(ctx); err != nil {
			t.Fatal(err)
		}
		model = loadRefreshStatusForTest(t, model)
		if strings.Contains(model.refreshLine(), "Usage refreshed") {
			t.Fatal("processing claimed displayed usage before dashboard read")
		}
		model = loadDashboardForTest(t, model)
		if len(model.rows) != 1 || model.rows[0].totalValue != 100 || !strings.Contains(model.refreshLine(), "Usage refreshed") {
			t.Fatalf("published refresh missing: rows=%+v state=%s", model.rows, model.refreshLine())
		}
		store := model.options.local.Store
		assertCLIQueryCount(t, store.SQL(), "SELECT SUM(total_tokens) FROM analytics.confirmed", 100)
		var before int
		if err := store.SQL().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM ingestion.batches").Scan(&before); err != nil {
			t.Fatal(err)
		}
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		if cmd == nil {
			t.Fatal("reload unavailable after refresh")
		}
		model = updated.(interactiveModel)
		updated, _ = model.Update(cmd())
		model = updated.(interactiveModel)
		assertCLIQueryCount(t, store.SQL(), "SELECT COUNT(*) FROM ingestion.batches", before)
		repeated := newInteractiveModel(t.Context(), model.options, model.now, "unknown").startCollection()
		defer repeated.cancelSync()
		repeated.width, repeated.height = model.width, model.height
		repeated = finishCollectionForTest(t, repeated)
		if repeated.refresh.result == nil || repeated.refresh.result.Err != nil || repeated.refresh.result.Result.Collection.Skipped == 0 {
			t.Fatalf("unchanged source not skipped: %+v", repeated.refresh.result)
		}
		if err := model.options.local.WaitVisible(ctx); err != nil {
			t.Fatal(err)
		}
		repeated = loadRefreshStatusForTest(t, repeated)
		repeated = loadDashboardForTest(t, repeated)
		if len(repeated.rows) != 1 || repeated.rows[0].totalValue != 100 || !strings.Contains(repeated.refreshLine(), "no new usage") {
			t.Fatalf("replay changed usage or status: rows=%+v state=%s", repeated.rows, repeated.refreshLine())
		}
		assertCLIQueryCount(t, store.SQL(), "SELECT COUNT(*) FROM ingestion.batches", before)
		return model, nil
	})
	defer restore()
	var stdout bytes.Buffer
	if err := Run(t.Context(), []string{"tui", "--collector-db-path", collectorPath, "--server-db-path", serverPath, "--all-time"}, &stdout, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatal("background refresh printed CLI summaries")
	}
}

func TestTUIRefreshFailureAndQuarantinePreserveSavedUsage(t *testing.T) {
	for _, quarantined := range []bool{false, true} {
		name := "submission"
		if quarantined {
			name = "quarantine"
		}
		t.Run(name, func(t *testing.T) {
			options := localViewOptions(t, true)
			replaceViewCollector(t, func(context.Context, collector.Options) (collector.Result, error) {
				if quarantined {
					return collector.Result{Collection: pipeline.Summary{Quarantined: 2}}, nil
				}
				err := &collector.StageError{Stage: "delivery", Code: "reference_conflict", Cause: errors.New("https://private.test?token=secret")}
				return collector.Result{DeliveryError: err}, err
			})
			model := newInteractiveModel(t.Context(), options, time.Now(), "unknown").startCollection()
			defer model.cancelSync()
			model.width, model.height = 120, 35
			model = loadDashboardForTest(t, model)
			model = finishCollectionForTest(t, model)
			model = loadRefreshStatusForTest(t, model)
			if model.err != nil || model.loading || len(model.rows) != 1 || model.rows[0].totalValue != 100 {
				t.Fatal("refresh failure lost saved data")
			}
			view := ansi.Strip(model.View())
			if !strings.Contains(view, "Refresh incomplete") || model.refreshBusy() {
				t.Fatalf("failure still busy or not visible: %s", view)
			}
			if strings.Contains(view, "secret") || strings.Contains(view, "private.test") {
				t.Fatal("refresh failure exposed sensitive error cause")
			}
		})
	}
}

func TestTUIFailedProcessingPreservesPublishedUsageWithoutCollection(t *testing.T) {
	options := localViewOptions(t, true)
	if err := options.local.Store.WriteTransaction(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE ingestion.metadata SET input_revision=input_revision+1"); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE processing.scopes SET revision=revision+1,error_code='processing_failed',attempts=1,retry_at_ms=?", time.Now().Add(time.Hour).UnixMilli())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	replaceViewCollector(t, func(context.Context, collector.Options) (collector.Result, error) {
		t.Error("query-only processing status collected again")
		return collector.Result{}, nil
	})
	model := newInteractiveModel(t.Context(), options, time.Now(), "unknown")
	defer model.cancelSync()
	model.width, model.height = 120, 35
	model = loadDashboardForTest(t, model)
	model = loadRefreshStatusForTest(t, model)
	if model.err != nil || len(model.rows) != 1 || model.rows[0].totalValue != 100 || model.refreshBusy() {
		t.Fatal("failed pending processing blocked saved usage")
	}
	if !strings.Contains(ansi.Strip(model.View()), "Processing needs attention") {
		t.Fatal("processing failure hidden", model.View())
	}
}

func TestTUIStartupSyncDefaultsAndReadOnlyOptOut(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{{nil, true}, {[]string{"--sync"}, true}, {[]string{"--sync=false"}, false}} {
		options, err := parseTableOptions(test.args, io.Discard, false, periodMonth)
		if err != nil || options.syncOnStart != test.want {
			t.Fatalf("args=%v sync=%v err=%v", test.args, options.syncOnStart, err)
		}
	}
}
