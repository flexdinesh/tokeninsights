package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/rawcollectorstore"
)

func windowSources(t *testing.T, count int) string {
	t.Helper()
	root := t.TempDir()
	for i := range count {
		rawTestWrite(t, filepath.Join(root, fmt.Sprintf("session-%02d.jsonl", i)), fmt.Sprintf("{\"type\":\"session\",\"id\":\"session-%d\"}\n{\"type\":\"message\",\"id\":\"message-%d\",\"message\":{\"role\":\"assistant\",\"usage\":{\"input\":%d,\"output\":2}}}\n", i, i, i+1))
	}
	return root
}

// Fixed lineages make sanitized evidence comparable across stores.
func windowStore(t *testing.T, root string) *rawcollectorstore.Store {
	t.Helper()
	store := rawTestStore(t)
	adapter, _ := AdapterFor(HarnessPi)
	sources, err := adapter.Discover(t.Context(), DiscoverOptions{SourceDir: root})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.BeginCapture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for i, source := range sources {
		key := evidence.Tuple("source-file", source.Harness, source.ID, source.Path)
		state := rawcollectorstore.Checkpoint{SourceKey: key, SourceID: key, Format: "pi-jsonl", Lineage: fmt.Sprintf("lineage-%d", i), Context: json.RawMessage(`{}`)}
		if err := tx.SaveCheckpoint(t.Context(), state); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return store
}

// blockedHead holds the first source open while recording later job starts.
type blockedHead struct {
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	started map[int]bool
	changed chan struct{}
}

func newBlockedHead(t *testing.T) *blockedHead {
	b := &blockedHead{release: make(chan struct{}), started: map[int]bool{}, changed: make(chan struct{}, 64)}
	t.Cleanup(b.unblock)
	return b
}

func (b *blockedHead) unblock() { b.once.Do(func() { close(b.release) }) }

func (b *blockedHead) before(job int) {
	if job == 0 {
		<-b.release
		return
	}
	b.mu.Lock()
	b.started[job] = true
	b.mu.Unlock()
	b.changed <- struct{}{}
}

func (b *blockedHead) waitStarted(t *testing.T, want int) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		b.mu.Lock()
		count := len(b.started)
		b.mu.Unlock()
		if count >= want {
			return
		}
		select {
		case <-b.changed:
		case <-deadline:
			t.Fatalf("head-of-line blocking: %d later sources started, want %d", count, want)
		}
	}
}

func (b *blockedHead) maxStarted() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	highest := 0
	for job := range b.started {
		highest = max(highest, job)
	}
	return highest
}

func TestRawCaptureWindowAvoidsHeadOfLineBlockingAndKeepsOrder(t *testing.T) {
	const workers, sources = 2, 20
	window := workers * rawCompletedSourcesPerWorker
	root := windowSources(t, sources)
	serialStore := windowStore(t, root)
	if _, err := Extract(t.Context(), SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: 1}, serialStore); err != nil {
		t.Fatal(err)
	}
	serial := rawTestRecords(t, serialStore)

	head := newBlockedHead(t)
	store := windowStore(t, root)
	done := make(chan error, 1)
	go func() {
		summary, err := Extract(t.Context(), SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: workers, beforePrepare: head.before}, store)
		if err == nil && summary.RawFacts != 2*sources {
			err = fmt.Errorf("captured %d facts", summary.RawFacts)
		}
		done <- err
	}()
	// Every other source in the window prepares while the head is still open.
	head.waitStarted(t, window-1)
	settle(head.changed, 100*time.Millisecond)
	select {
	case err := <-done:
		t.Fatal("capture finished before head source", err)
	default:
	}
	if highest := head.maxStarted(); highest >= window {
		t.Fatalf("source %d started beyond the %d-source window", highest, window)
	}
	head.unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(serial, rawTestRecords(t, store)) {
		t.Fatal("capture window changed sanitized evidence/order")
	}
}

// settle gives sources beyond the window time to start if admission were wrong.
func settle(changed chan struct{}, period time.Duration) {
	timer := time.After(period)
	for {
		select {
		case <-changed:
		case <-timer:
			return
		}
	}
}

func TestRawCaptureWindowRemovesSpoolsOnCancellation(t *testing.T) {
	const workers = 2
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	root := windowSources(t, 12)
	store := windowStore(t, root)
	head := newBlockedHead(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Extract(ctx, SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: workers, beforePrepare: head.before}, store)
		done <- err
	}()
	head.waitStarted(t, workers*rawCompletedSourcesPerWorker-1)
	cancel()
	head.unblock()
	if err := <-done; err == nil {
		t.Fatal("cancelled capture succeeded")
	}
	entries, err := os.ReadDir(temp)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "tokeninsights-capture-") {
			t.Fatal("capture spool directory survived cancellation", entry.Name())
		}
	}
}
