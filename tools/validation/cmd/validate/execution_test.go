package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCorrectnessSelectionExcludesOnlyPerformance(t *testing.T) {
	data, err := os.ReadFile("../../groups.json")
	if err != nil {
		t.Fatal(err)
	}
	var configured manifest
	if err := json.Unmarshal(data, &configured); err != nil {
		t.Fatal(err)
	}
	if err := configured.validate(); err != nil {
		t.Fatal(err)
	}
	selected := defaultGroups()
	for name := range configured.Groups {
		if slices.Contains(selected, name) != (name != "benchmarks") {
			t.Fatalf("incorrect default coverage for %s", name)
		}
	}
	for _, group := range groupPriority {
		if _, ok := configured.Groups[group]; !ok {
			t.Fatalf("priority names unknown group %s", group)
		}
	}
}

func noAdmission(context.Context) (func(), error) { return func() {}, nil }

func TestSchedulingFormatsFirstThenBoundsParallelismLongestFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var formatted, admittedAfterFormat atomic.Bool
	var mu sync.Mutex
	running, peak := 0, 0
	var order []string
	groups := []string{"web", "tooling", "static", "format", "deployment", "distribution", "browser", "go"}
	err := scheduleChecks(ctx, groups, func(context.Context) (func(), error) {
		admittedAfterFormat.Store(formatted.Load())
		return func() {}, nil
	}, func(_ context.Context, group string) groupResult {
		if group == "format" {
			formatted.Store(true)
			return groupResult{}
		}
		mu.Lock()
		order = append(order, group)
		running++
		peak = max(peak, running)
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		running--
		mu.Unlock()
		return groupResult{}
	}, func(string, groupResult) {})
	if err != nil {
		t.Fatal(err)
	}
	if !admittedAfterFormat.Load() {
		t.Fatal("admission preceded formatting")
	}
	if peak != maxParallelGroups {
		t.Fatalf("peak concurrency %d, want %d", peak, maxParallelGroups)
	}
	// Workers dequeue in priority order; their start records may interleave.
	first := slices.Clone(order[:maxParallelGroups])
	slices.Sort(first)
	if !reflect.DeepEqual(first, []string{"browser", "distribution", "go"}) {
		t.Fatal("longest groups did not start first", order)
	}
	if !reflect.DeepEqual(prioritized(groups[:3]), []string{"static", "tooling", "web"}) {
		t.Fatal("priority order", prioritized(groups[:3]))
	}
}

func TestSchedulingFailureLetsSiblingsFinishAndReportsAll(t *testing.T) {
	first, second := errors.New("go failed"), errors.New("web failed")
	var completed []string
	err := scheduleChecks(t.Context(), []string{"go", "web", "static", "tooling"}, noAdmission, func(ctx context.Context, group string) groupResult {
		switch group {
		case "go":
			return groupResult{err: first}
		case "web":
			return groupResult{err: second}
		}
		time.Sleep(20 * time.Millisecond)
		if ctx.Err() != nil {
			return groupResult{err: ctx.Err()}
		}
		return groupResult{}
	}, func(group string, result groupResult) {
		if result.err == nil {
			completed = append(completed, group)
		}
	})
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatal("not every failure reported", err)
	}
	slices.Sort(completed)
	if !reflect.DeepEqual(completed, []string{"static", "tooling"}) {
		t.Fatal("siblings did not finish", completed)
	}
}

func TestFormatFailureStopsBeforeAdmission(t *testing.T) {
	want := errors.New("format failed")
	admitted, called := false, false
	err := scheduleChecks(t.Context(), []string{"web", "format"}, func(context.Context) (func(), error) {
		admitted = true
		return func() {}, nil
	}, func(_ context.Context, group string) groupResult {
		if group != "format" {
			called = true
		}
		return groupResult{err: want}
	}, func(string, groupResult) {})
	if !errors.Is(err, want) || admitted || called {
		t.Fatal("failed formatting admitted checks", err)
	}
}

func TestCancellationJoinsRunningAndSkipsQueued(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered := make(chan struct{}, maxParallelGroups)
	var executed, joined atomic.Int32
	released := false
	groups := []string{"go", "browser", "distribution", "deployment", "static"}
	done := make(chan error, 1)
	go func() {
		done <- scheduleChecks(ctx, groups, func(context.Context) (func(), error) {
			return func() { released = joined.Load() == maxParallelGroups }, nil
		}, func(ctx context.Context, group string) groupResult {
			executed.Add(1)
			entered <- struct{}{}
			<-ctx.Done()
			joined.Add(1)
			return groupResult{err: ctx.Err()}
		}, func(string, groupResult) {})
	}()
	for range maxParallelGroups {
		<-entered
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if executed.Load() != maxParallelGroups || !released {
		t.Fatal("queued groups started or admission released before join", executed.Load(), released)
	}
}

func TestWebIdentityIgnoresBackendButTracksFrontendAndContract(t *testing.T) {
	r := fixture(t)
	data, err := os.ReadFile("../../groups.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &r.manifest); err != nil {
		t.Fatal(err)
	}
	write := func(path string) {
		t.Helper()
		path = filepath.Join(r.root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	identity := func() string {
		t.Helper()
		key, err := r.identity(commit(t, r), "web")
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	previous := identity()
	for _, path := range []string{"packages/core/datastore/store.go", "packages/backend/cmd/main.go", "packages/desktop/main.go"} {
		write(path)
		if next := identity(); next != previous {
			t.Fatalf("backend input invalidated web: %s", path)
		}
	}
	// go.mod feeds the shared deps stage that every image builds on.
	for _, path := range []string{"packages/web/src/app.tsx", "packages/dashboard/src/client.tsx", "packages/contracts/src/generated/api.ts", "pnpm-lock.yaml", "go.mod"} {
		write(path)
		next := identity()
		if next == previous {
			t.Fatalf("web input failed to invalidate: %s", path)
		}
		previous = next
	}
}

func TestForcedCancellationSupersedesPassingReceipt(t *testing.T) {
	r := fixture(t)
	sha := commit(t, r)
	base := t.TempDir()
	artifacts := filepath.Join(t.TempDir(), "go")
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"status.json": `{"exitCode":0}`, "check.log": "passed"} {
		if err := os.WriteFile(filepath.Join(artifacts, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(base, "go", "key.json")
	if err := saveReceipt(path, receipt{Version: receiptVersion, Group: "go", Key: "key", SHA: sha, Artifacts: artifacts, Completed: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r.ctx = ctx
	r.docker = "/nonexistent/docker"
	inv := invocation{sha: sha, tree: "tree", out: t.TempDir(), base: base, source: t.TempDir(), temp: t.TempDir(), force: true, queued: time.Now(), executionGroup: func(g string) string { return g }}
	result := r.executeGroup(inv, "go", "key")
	if result.err == nil || result.executed {
		t.Fatal("cancelled check executed or succeeded", result.err)
	}
	if _, ok := readReceipt(path, "key", "go"); ok {
		t.Fatal("failed forced check reused earlier success")
	}
}
