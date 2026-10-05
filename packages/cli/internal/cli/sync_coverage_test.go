package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestReadOnlyViewerLoadsSavedUsageWithoutSourceCoverage(t *testing.T) {
	remote, _, requests := newViewQueryServer(t, true)
	root := t.TempDir()
	isolateViewSources(t, root)
	options := tableOptions{serverURL: remote.URL, dbPath: filepath.Join(root, "unopened.sqlite"), collectorDBPath: filepath.Join(root, "collector.sqlite"), period: periodAllTime, bucket: bucketDay}
	model := newInteractiveModel(context.Background(), options, time.Now(), "unknown")
	defer model.cancelSync()
	if model.syncing || model.syncInFlight || !model.loading {
		t.Fatalf("initial read state: syncing=%v collection=%v loading=%v", model.syncing, model.syncInFlight, model.loading)
	}
	loaded := model.loadDashboard()
	if loaded.err != nil {
		t.Fatal(loaded.err)
	}
	updated, _ := model.Update(loaded)
	shown := updated.(interactiveModel)
	shown.width, shown.height = 120, 35
	if shown.loading || len(shown.rows) != 1 || shown.rows[0].totalValue != 100 || len(shown.coverage) != 0 {
		t.Fatalf("saved view=%+v", shown.rows)
	}
	view := ansi.Strip(shown.View())
	for _, invented := range []string{"days checked", "sources checked", "Checking local sources", "Local sources not checked", "retry sync"} {
		if strings.Contains(view, invented) {
			t.Fatalf("query-only server claims source coverage %q: %s", invented, view)
		}
	}
	if requests.posts.Load() != 0 {
		t.Fatal("view requested server collection")
	}
	assertViewMissingPath(t, options.dbPath)
	assertViewMissingPath(t, options.collectorDBPath)
}

func TestViewerReloadReadsServerAndPreservesFilters(t *testing.T) {
	remote, _, requests := newViewQueryServer(t, true)
	selected := filters{providers: stringList{"fixture-provider"}, models: stringList{"fixture-model"}, harnesses: stringList{"pi"}, sessionIDs: stringList{"fixture-view-session"}}
	options := tableOptions{serverURL: remote.URL, period: periodAllTime, bucket: bucketDay, filters: selected}
	model := newInteractiveModel(context.Background(), options, time.Now(), "unknown")
	defer model.cancelSync()
	updated, _ := model.Update(model.loadDashboard())
	model = updated.(interactiveModel)
	model.rows[0].totalValue = 0 // Successful reload must replace saved presentation.
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(interactiveModel)
	if cmd == nil {
		t.Fatal("reload shortcut did not request data")
	}
	updated, _ = model.Update(cmd())
	model = updated.(interactiveModel)
	if model.err != nil || len(model.rows) != 1 || model.rows[0].totalValue != 100 || !reflect.DeepEqual(model.options.filters, selected) {
		t.Fatalf("reload lost saved data/filter scope: err=%v filters=%+v rows=%+v", model.err, model.options.filters, model.rows)
	}
	if requests.posts.Load() != 0 {
		t.Fatal("reload wrote server data")
	}
}

func TestViewerOldSyncShortcutDoesNotCollect(t *testing.T) {
	remote, _, requests := newViewQueryServer(t, true)
	model := newInteractiveModel(context.Background(), tableOptions{serverURL: remote.URL, period: periodAllTime, bucket: bucketDay}, time.Now(), "unknown")
	defer model.cancelSync()
	before := requests.gets.Load()
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	model = updated.(interactiveModel)
	if cmd != nil || model.syncing || model.syncInFlight || requests.posts.Load() != 0 || requests.gets.Load() != before {
		t.Fatal("obsolete sync shortcut triggered work")
	}
}

func TestViewerQueryFailureRetainsSavedRowsAndFilters(t *testing.T) {
	var fail atomic.Bool
	remote, _, _ := newViewQueryServer(t, true)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		target := remote.URL + r.URL.RequestURI()
		request, err := http.NewRequestWithContext(r.Context(), r.Method, target, nil)
		if err != nil {
			t.Error(err)
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = response.Body.Close() }()
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	defer proxy.Close()
	selected := filters{providers: stringList{"fixture-provider"}, models: stringList{"fixture-model"}}
	model := newInteractiveModel(context.Background(), tableOptions{serverURL: proxy.URL, period: periodAllTime, bucket: bucketDay, filters: selected}, time.Now(), "unknown")
	defer model.cancelSync()
	loaded := model.loadDashboard()
	if loaded.err != nil {
		t.Fatal(loaded.err)
	}
	updated, _ := model.Update(loaded)
	model = updated.(interactiveModel)
	fail.Store(true)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(interactiveModel)
	if cmd == nil {
		t.Fatal("retry missing query")
	}
	updated, _ = model.Update(cmd())
	model = updated.(interactiveModel)
	if model.err == nil || len(model.rows) != 1 || model.rows[0].totalValue != 100 || !reflect.DeepEqual(model.options.filters, selected) {
		t.Fatalf("query error discarded filters/data: err=%v filters=%+v rows=%+v", model.err, model.options.filters, model.rows)
	}
	fail.Store(false)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(interactiveModel)
	if cmd == nil {
		t.Fatal("recovery retry missing query")
	}
	updated, _ = model.Update(cmd())
	model = updated.(interactiveModel)
	if model.err != nil || len(model.rows) != 1 || model.rows[0].totalValue != 100 {
		t.Fatalf("query retry failed: %v", model.err)
	}
}

func TestQuitCancelsOutstandingViewerReads(t *testing.T) {
	started := make(chan struct{})
	remote := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer remote.Close()
	model := newInteractiveModel(context.Background(), tableOptions{serverURL: remote.URL, period: periodAllTime, bucket: bucketDay}, time.Now(), "unknown")
	defer model.cancelSync()
	finished := make(chan reloadMsg, 1)
	go func() { finished <- model.loadDashboard() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("viewer did not begin server read")
	}
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !errors.Is(model.ctx.Err(), context.Canceled) {
		t.Fatal("quit left query context running")
	}
	select {
	case result := <-finished:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("query cancellation=%v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("quit left HTTP query running")
	}
}
