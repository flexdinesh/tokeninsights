package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func driveStartupForTest(t *testing.T, model startupModel) tea.Model {
	t.Helper()
	model.dashboard.width, model.dashboard.height = 120, 35
	model.Init()
	t.Cleanup(func() { model.dashboard.cancelSync(); model.workers.Wait() })
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case msg, ok := <-model.messages:
			if !ok {
				t.Fatal("startup closed without result")
			}
			updated, _ := model.Update(msg)
			if dashboard, ok := updated.(interactiveModel); ok {
				return dashboard
			}
			model = updated.(startupModel)
			if model.err != nil {
				return model
			}
		case <-deadline.C:
			t.Fatal("startup did not finish")
		}
	}
}

func TestTUIDefaultSyncAcceptsInsideLoadingScreenAndReloadQueriesProcessedData(t *testing.T) {
	remote, store, requests := newRawViewQueryServer(t)
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
	collectorPath, serverPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "unused-server.sqlite")
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		assertViewMissingPath(t, collectorPath)
		startup := newStartupModel(model)
		startup.dashboard.width, startup.dashboard.height = 120, 35
		if !strings.Contains(startup.View(), "Updating usage") || strings.Contains(startup.View(), "Total tokens") {
			t.Fatal("startup skipped the loading screen")
		}
		final := driveStartupForTest(t, startup)
		dashboard, ok := final.(interactiveModel)
		if !ok || dashboard.loading || len(dashboard.rows) != 0 {
			t.Fatal("startup must finish after acceptance, before asynchronous processing")
		}
		if !dashboard.sharedSync.Running || !strings.Contains(ansi.Strip(dashboard.View()), "processing") {
			t.Fatal("accepted processing not shown")
		}
		for {
			worked, err := store.ProcessNext(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !worked {
				break
			}
		}
		assertCLIQueryCount(t, store.SQL(), "SELECT SUM(total_tokens) FROM analytics.confirmed", 100)
		before := requests.posts.Load()
		updated, cmd := dashboard.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		dashboard = updated.(interactiveModel)
		dashboard.Update(cmd())
		if requests.posts.Load() != before {
			t.Fatal("dashboard reload collected/published")
		}
		repeated := driveStartupForTest(t, newStartupModel(model)).(interactiveModel)
		if len(repeated.rows) != 1 || repeated.rows[0].totalValue != 100 {
			t.Fatal("repeat sync changed saved usage")
		}
		for _, row := range repeated.syncProgressRows {
			if row.harness == pipeline.HarnessPi && row.status != pipeline.SyncProgressSkipped {
				t.Fatal("unchanged Pi source was not skipped")
			}
		}
		if status := ansi.Strip(newStartupModel(repeated).harnessStatus(pipeline.SyncProgressSkipped)); status != "No new usage" {
			t.Fatalf("unchanged source label=%q", status)
		}
		return dashboard, nil
	})
	defer restore()
	var stdout bytes.Buffer
	if err := Run(t.Context(), []string{"tui", "--server-url", remote.URL, "--collector-db-path", collectorPath, "--server-db-path", serverPath, "--all-time"}, &stdout, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || requests.posts.Load() == 0 {
		t.Fatal("startup printed CLI summaries or did not publish")
	}
	assertViewMissingPath(t, serverPath)
}

func TestStartupFailureCanRetryOrViewSavedWithoutCollection(t *testing.T) {
	for _, key := range []rune{'r', 'v'} {
		t.Run(string(key), func(t *testing.T) {
			remote, _, _ := newViewQueryServer(t, true)
			calls := 0
			replaceViewCollector(t, func(context.Context, collector.Options) (collector.Result, error) {
				calls++
				if calls == 1 {
					return collector.Result{DeliveryError: errors.New("unavailable")}, errors.New("unavailable")
				}
				return collector.Result{}, nil
			})
			dashboard := newInteractiveModel(t.Context(), tableOptions{serverURL: remote.URL, period: periodAllTime, bucket: bucketDay}, time.Now(), "unknown")
			failed := driveStartupForTest(t, newStartupModel(dashboard)).(startupModel)
			if failed.busy || !strings.Contains(failed.View(), "Couldn't publish usage") || !strings.Contains(failed.View(), "View saved") {
				t.Fatal("failure lost recovery actions")
			}
			updated, _ := failed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
			retry := updated.(startupModel)
			// Update starts the next worker; drive its messages without another Init.
			for retry.busy {
				select {
				case msg := <-retry.messages:
					updated, _ = retry.Update(msg)
					if result, ok := updated.(interactiveModel); ok {
						if len(result.rows) != 1 || result.rows[0].totalValue != 100 {
							t.Fatal("saved data not restored")
						}
						want := 2
						if key == 'v' {
							want = 1
						}
						if calls != want {
							t.Fatalf("collection calls=%d want=%d", calls, want)
						}
						return
					}
					retry = updated.(startupModel)
				case <-time.After(10 * time.Second):
					t.Fatal("recovery did not finish")
				}
			}
			t.Fatal("recovery failed")
		})
	}
}

func TestStartupQuitCancelsWorkerAndIgnoresOtherKeysWhileBusy(t *testing.T) {
	remote, _, _ := newViewQueryServer(t, false)
	started, stopped := make(chan struct{}), make(chan struct{})
	replaceViewCollector(t, func(ctx context.Context, _ collector.Options) (collector.Result, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return collector.Result{}, ctx.Err()
	})
	dashboard := newInteractiveModel(t.Context(), tableOptions{serverURL: remote.URL}, time.Now(), "unknown")
	model := newStartupModel(dashboard)
	model.Init()
	defer func() { dashboard.cancelSync(); model.workers.Wait() }()
	<-started
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil || !updated.(startupModel).busy {
		t.Fatal("busy retry overlapped collection")
	}
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit missing")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("quit left collector running")
	}
}

func TestStartupFitsTerminalAndKeepsRecoveryKeys(t *testing.T) {
	for _, size := range [][2]int{{120, 35}, {80, 24}, {40, 14}, {30, 9}} {
		for _, failed := range []bool{false, true} {
			m := newStartupModel(newInteractiveModel(t.Context(), tableOptions{serverURL: "http://127.0.0.1:1"}, time.Now(), "unknown"))
			defer m.dashboard.cancelSync()
			m.dashboard.width, m.dashboard.height = size[0], size[1]
			m.phase = startupPublish
			m.delivery = collector.DeliveryProgress{Batches: 33, Pending: 36199, PendingKnown: true}
			m.dashboard = m.dashboard.withSyncProgress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessCodex, Status: pipeline.SyncProgressSyncing})
			if failed {
				m.busy, m.err = false, errors.New("failure")
			}
			view := ansi.Strip(m.View())
			lines := strings.Split(view, "\n")
			if len(lines) != size[1] {
				t.Fatalf("height=%d want=%d", len(lines), size[1])
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("startup overflow: %q", line)
				}
			}
			if !strings.Contains(view, "q Quit") {
				t.Fatalf("quit missing: %s", view)
			}
			if !strings.Contains(view, "33 batches accepted") || !strings.Contains(view, "36199 pending") {
				t.Fatalf("acknowledged progress truncated: %s", view)
			}
			if failed && (!strings.Contains(view, "r Retry") || !strings.Contains(view, "v ")) {
				t.Fatalf("recovery keys missing: %s", view)
			}
		}
	}
}

func TestTUIStartupSyncDefaultsAndReadOnlyOptOut(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{{nil, true}, {[]string{"--sync"}, true}, {[]string{"--sync=false"}, false}} {
		options, err := parseTableOptions(test.args, io.Discard, false, periodMonth)
		if err != nil || options.syncBeforeView != test.want {
			t.Fatalf("args=%v sync=%v err=%v", test.args, options.syncBeforeView, err)
		}
	}
}

func TestStartupFailureShowsSafeReasonAndDiagnosticsAtCompactSizes(t *testing.T) {
	for _, code := range []string{"reference_conflict", "http_401", "server_database_changed", "collector_database"} {
		for _, size := range [][2]int{{120, 35}, {40, 14}, {30, 9}} {
			m := newStartupModel(newInteractiveModel(t.Context(), tableOptions{serverURL: "http://127.0.0.1:1"}, time.Now(), "unknown"))
			m.dashboard.width, m.dashboard.height = size[0], size[1]
			m.busy, m.phase = false, startupPublish
			m.err = &collector.StageError{Stage: "delivery", Code: code, Cause: errors.New("https://private.test?token=secret")}
			m.delivery = collector.DeliveryProgress{Batches: 33, Pending: 36199, PendingKnown: true}
			view := ansi.Strip(m.View())
			m.dashboard.cancelSync()
			for _, want := range []string{code, "tokeninsights sync", "33 batches accepted", "36199 pending", "r Retry", "v ", "q Quit"} {
				if !strings.Contains(view, want) {
					t.Fatalf("missing %q at %v: %s", want, size, view)
				}
			}
			if strings.Contains(view, "secret") || strings.Contains(view, "private.test") {
				t.Fatal("error cause exposed credentials")
			}
		}
	}
}
