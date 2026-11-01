package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixture(t *testing.T) runner {
	t.Helper()
	r := runner{ctx: context.Background(), root: t.TempDir(), home: t.TempDir(), manifest: manifest{Common: []string{"*", "packages/"}, Groups: map[string]groupSpec{"go": {Class: "heavy"}}}}
	for _, a := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "Test"}} {
		if _, e := r.git(a...); e != nil {
			t.Fatal(e)
		}
	}
	return r
}

func TestSourceAndLockfileInvalidate(t *testing.T) {
	r := fixture(t)
	var previous string
	for _, name := range []string{"packages/cli/example.go", "pnpm-lock.yaml", "mise.toml"} {
		path := filepath.Join(r.root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("input"), 0600); err != nil {
			t.Fatal(err)
		}
		key, err := r.identity(commit(t, r), "go")
		if err != nil || key == previous {
			t.Fatalf("%s did not invalidate: %v", name, err)
		}
		previous = key
	}
}

func TestCommandCancellationKillsDescendants(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := runner{ctx: ctx, root: t.TempDir(), home: t.TempDir()}
	pidFile := filepath.Join(r.root, "child.pid")
	done := make(chan error, 1)
	go func() {
		_, err := r.command(r.root, "sh", "-c", `sleep 120 & child=$!; printf '%s' "$child" > "$1"; wait`, "fixture", pidFile)
		done <- err
	}()
	deadline := time.After(3 * time.Second)
	var pid int
	for pid == 0 {
		select {
		case <-deadline:
			t.Fatal("fixture failed to start")
		default:
		}
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		if pid == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancellation succeeded")
		}
	case <-time.After(7 * time.Second):
		t.Fatal("cancellation hung")
	}
	// A killed orphan can briefly remain a zombie until the container's init reaps it.
	if err := syscall.Kill(pid, 0); err == nil {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err == nil && !strings.Contains(string(data), ") Z ") {
			time.Sleep(50 * time.Millisecond)
			data, err = os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
			if err == nil && !strings.Contains(string(data), ") Z ") {
				t.Fatal("descendant still running")
			}
		}
	}
}
func commit(t *testing.T, r runner) string {
	t.Helper()
	for _, a := range [][]string{{"add", "."}, {"commit", "--allow-empty", "-qm", "fixture"}} {
		if _, e := r.git(a...); e != nil {
			t.Fatal(e)
		}
	}
	s, e := r.clean()
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestSnapshotIdentityAndIsolation(t *testing.T) {
	r := fixture(t)
	p := filepath.Join(r.root, "mise.toml")
	if e := os.WriteFile(p, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	sha := commit(t, r)
	if e := os.WriteFile(filepath.Join(r.root, "secret"), []byte("untracked"), 0600); e != nil {
		t.Fatal(e)
	}
	dest := t.TempDir()
	key, e := r.snapshot(sha, "go", dest)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dest, "secret")); !os.IsNotExist(e) {
		t.Fatal("untracked input leaked")
	}
	if st, e := os.Stat(filepath.Join(dest, ".git")); e != nil || !st.IsDir() {
		t.Fatal("snapshot lacks independent git metadata")
	}
	next := commit(t, r)
	key2, e := r.snapshot(next, "go", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if key == key2 {
		t.Fatal("new root input must invalidate")
	}
	history := commit(t, r)
	key3, e := r.snapshot(history, "go", t.TempDir())
	if e != nil || key2 != key3 {
		t.Fatal("history-only change invalidated", e)
	}
	if e = os.WriteFile(p, []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = r.clean(); e == nil {
		t.Fatal("dirty tree accepted")
	}
	changed := commit(t, r)
	key4, e := r.snapshot(changed, "go", t.TempDir())
	if e != nil || key3 == key4 {
		t.Fatal("configuration change not invalidated", e)
	}
}
func TestEvidenceReceiptRequiresCompleteSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(t.TempDir(), "receipt.json")
	r := receipt{Version: receiptVersion, Group: "go", Key: "key", SHA: "sha", Artifacts: dir, Completed: time.Now()}
	if e := saveReceipt(path, r); e != nil {
		t.Fatal(e)
	}
	for _, status := range []string{"{}", "{\"exitCode\":1}", "invalid", "{\"exitCode\":0}"} {
		if e := os.WriteFile(filepath.Join(dir, "status.json"), []byte(status), 0600); e != nil {
			t.Fatal(e)
		}
		if _, ok := readReceipt(path, "key", "go"); ok {
			t.Fatal("incomplete evidence accepted")
		}
	}
	if e := os.WriteFile(filepath.Join(dir, "check.log"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	if _, ok := readReceipt(path, "key", "go"); !ok {
		t.Fatal("complete success rejected")
	}
	if _, ok := readReceipt(path, "different", "go"); ok {
		t.Fatal("wrong identity accepted")
	}
}
func TestAdmissionCancellationAndRelease(t *testing.T) {
	release, e := slot(context.Background(), "heavy")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if other, e := slot(ctx, "heavy"); e == nil {
		other()
		release()
		t.Fatal("concurrent capacity admitted")
	}
	release()
	release, e = slot(context.Background(), "heavy")
	if e != nil {
		t.Fatal(e)
	}
	release()
}

func TestPrunePreservesActiveLease(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "runs", "active")
	if e := os.MkdirAll(p, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(p, "completed"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if e := os.Chtimes(filepath.Join(p, "completed"), old, old); e != nil {
		t.Fatal(e)
	}
	lease, e := lockFile(filepath.Join(p, "active.lock"))
	if e != nil {
		t.Fatal(e)
	}
	if e = pruneRuns(base); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p); e != nil {
		t.Fatal("active evidence removed")
	}
	lease.Close()
	if e = pruneRuns(base); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("expired evidence retained")
	}
}
func TestIdentityIgnoresHistoryAndReadme(t *testing.T) {
	r := fixture(t)
	r.manifest.Common = []string{"mise.toml"}
	if e := os.WriteFile(filepath.Join(r.root, "mise.toml"), []byte("config"), 0600); e != nil {
		t.Fatal(e)
	}
	first, e := r.identity(commit(t, r), "go")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(r.root, "README.md"), []byte("docs"), 0600); e != nil {
		t.Fatal(e)
	}
	second, e := r.identity(commit(t, r), "go")
	if e != nil || first != second {
		t.Fatal("README invalidated", e)
	}
	third, e := r.identity(commit(t, r), "go")
	if e != nil || second != third {
		t.Fatal("history invalidated", e)
	}
}

func TestPruneAbandonedRun(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "runs", "abandoned")
	if err := os.MkdirAll(p, 0700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if err := pruneRuns(base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("abandoned run retained", err)
	}
}

func TestStorageSamplesHaveSeparateIdentity(t *testing.T) {
	r := fixture(t)
	sha := commit(t, r)
	smoke, err := r.identity(sha, "go")
	if err != nil {
		t.Fatal(err)
	}
	r.storageBenchmarks = true
	full, err := r.identity(sha, "go")
	if err != nil || smoke == full {
		t.Fatal("full samples share smoke identity", err)
	}
	for _, args := range [][]string{{"--storage-benchmarks"}, {"--storage-benchmarks", "--group", "web"}} {
		if err := run(context.Background(), args); err == nil {
			t.Fatal("invalid storage selector accepted")
		}
	}
}

func TestIdentityTracksModeAndImplementation(t *testing.T) {
	r := fixture(t)
	p := filepath.Join(r.root, "task.sh")
	if err := os.WriteFile(p, []byte("exit 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := r.identity(commit(t, r), "go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0700); err != nil {
		t.Fatal(err)
	}
	second, err := r.identity(commit(t, r), "go")
	if err != nil || first == second {
		t.Fatal("executable mode did not invalidate", err)
	}
	if err := os.WriteFile(p, []byte("exit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	third, err := r.identity(commit(t, r), "go")
	if err != nil || second == third {
		t.Fatal("task implementation did not invalidate", err)
	}
}
