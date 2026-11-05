package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	selected := defaultGroups()
	for name := range configured.Groups {
		if slices.Contains(selected, name) != (name != "benchmarks") {
			t.Fatalf("incorrect default coverage for %s", name)
		}
	}
}

func TestSchedulingFormatsFirstThenOverlapsAndJoinsChecks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var formatted atomic.Bool
	entered := make(chan string, 2)
	release := make(chan struct{})
	finished := make(chan error, 1)
	var completed []string
	go func() {
		finished <- scheduleChecks(ctx, []string{"go", "format", "web"}, func(ctx context.Context, group string) groupResult {
			if group == "format" {
				formatted.Store(true)
				return groupResult{}
			}
			if !formatted.Load() {
				return groupResult{err: errors.New("started before formatting")}
			}
			entered <- group
			select {
			case <-release:
				return groupResult{}
			case <-ctx.Done():
				return groupResult{err: ctx.Err()}
			}
		}, func(group string, _ groupResult) { completed = append(completed, group) })
	}()
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("checks did not overlap")
		}
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	slices.Sort(completed)
	if !reflect.DeepEqual(completed, []string{"format", "go", "web"}) {
		t.Fatal(completed)
	}
}

func TestSchedulingFailureCancelsAndJoinsSibling(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	started := make(chan struct{})
	var joined atomic.Bool
	want := errors.New("check failed")
	err := scheduleChecks(ctx, []string{"go", "web"}, func(ctx context.Context, group string) groupResult {
		if group == "go" {
			select {
			case <-started:
				return groupResult{err: want}
			case <-ctx.Done():
				return groupResult{err: ctx.Err()}
			}
		}
		close(started)
		<-ctx.Done()
		joined.Store(true)
		return groupResult{err: ctx.Err()}
	}, func(string, groupResult) {})
	if !errors.Is(err, want) || !joined.Load() {
		t.Fatal("failed to preserve error and join sibling", err)
	}
	called := false
	err = scheduleChecks(ctx, []string{"web", "format"}, func(_ context.Context, group string) groupResult {
		if group != "format" {
			called = true
		}
		return groupResult{err: want}
	}, func(string, groupResult) {})
	if !errors.Is(err, want) || called {
		t.Fatal("failed formatting admitted checks", err)
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
	for _, path := range []string{"packages/core/datastore/store.go", "packages/backend/cmd/main.go", "packages/desktop/main.go", "go.mod"} {
		write(path)
		if next := identity(); next != previous {
			t.Fatalf("backend input invalidated web: %s", path)
		}
	}
	for _, path := range []string{"packages/web/src/app.tsx", "packages/dashboard/src/client.tsx", "packages/contracts/src/generated/api.ts", "pnpm-lock.yaml"} {
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
	result := r.executeGroup(sha, "tree", "go", "key", t.TempDir(), base, "attempt", "dagger", true, func() error { t.Fatal("cancelled check reached engine"); return nil })
	if result.err == nil {
		t.Fatal("cancelled check succeeded")
	}
	if _, ok := readReceipt(path, "key", "go"); ok {
		t.Fatal("failed forced check reused earlier success")
	}
}
