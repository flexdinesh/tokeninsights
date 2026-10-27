package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGateProcess(t *testing.T) {
	if os.Getenv("CHECK_GATE_TEST_PROCESS") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Exit(run(os.Args[i+1:]))
			}
		}
		// The gate re-execs this test binary as its supervisor.
		os.Exit(run(os.Args[1:]))
	}
}

// TestMain allows the supervisor's private argv before testing parses flags.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--supervise" {
		os.Exit(run(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "init", "--quiet", first)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	// A linked-worktree administrative directory: Git resolves commondir,
	// not the worktree-specific gitdir. No user checkout is created or switched.
	admin := filepath.Join(first, ".git", "worktrees", "second")
	if err := os.MkdirAll(admin, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string]string{
		filepath.Join(second, ".git"):     "gitdir: " + admin + "\n",
		filepath.Join(admin, "commondir"): "../..\n",
		filepath.Join(admin, "HEAD"):      "ref: refs/heads/fixture\n",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return first, second
}

func gate(t *testing.T, dir string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestGateProcess$", "--"}, args...)...)
	cmd.Dir = dir
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, activeKey+"=") && !strings.HasPrefix(value, "GIT_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "CHECK_GATE_TEST_PROCESS=1")
	output := &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, output
}

func waitFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestCommonDirectoryExclusionAndFailure(t *testing.T) {
	first, second := fixture(t)
	entered, release, next := filepath.Join(first, "entered"), filepath.Join(first, "release"), filepath.Join(second, "next")
	one, _ := gate(t, first, "sh", "-c", `touch "$1"; while ! test -f "$2"; do sleep 0.05; done; exit 23`, "sh", entered, release)
	waitFile(t, entered)
	two, output := gate(t, second, "sh", "-c", `touch "$1"`, "sh", next)
	time.Sleep(250 * time.Millisecond)
	if _, err := os.Stat(next); !os.IsNotExist(err) {
		t.Fatal("second worktree entered while first owned gate")
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := exitCode(one.Wait()); code != 23 {
		t.Fatalf("exit = %d, want 23", code)
	}
	if err := two.Wait(); err != nil {
		t.Fatalf("second gate: %v: %s", err, output)
	}
	waitFile(t, next)
	if !strings.Contains(output.String(), "wait ") || !strings.Contains(output.String(), "execution ") {
		t.Fatalf("missing timing: %s", output)
	}
}

func TestCancellationAndCrashRelease(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		t.Run(fmt.Sprint(sig), func(t *testing.T) {
			first, second := fixture(t)
			entered, stopped, next := filepath.Join(first, "entered"), filepath.Join(first, "stopped"), filepath.Join(second, "next")
			one, _ := gate(t, first, "sh", "-c", `trap 'touch "$2"; exit' TERM; touch "$1"; while :; do sleep 0.05; done`, "sh", entered, stopped)
			waitFile(t, entered)
			two, _ := gate(t, second, "sh", "-c", `test -f "$1" && touch "$2"`, "sh", stopped, next)
			if err := one.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			_ = one.Wait()
			if err := two.Wait(); err != nil {
				t.Fatal(err)
			}
			waitFile(t, next)
		})
	}
}

func TestWaitingCancellationAndNestedRejection(t *testing.T) {
	first, second := fixture(t)
	entered := filepath.Join(first, "entered")
	one, _ := gate(t, first, "sh", "-c", `touch "$1"; sleep 30`, "sh", entered)
	waitFile(t, entered)
	two, _ := gate(t, second, "true")
	time.Sleep(150 * time.Millisecond)
	_ = two.Process.Signal(syscall.SIGTERM)
	if code := exitCode(two.Wait()); code != 143 {
		t.Fatalf("waiter exit %s", strconv.Itoa(code))
	}
	_ = one.Process.Signal(syscall.SIGTERM)
	_ = one.Wait()
	nested, _ := gate(t, first, os.Args[0], "-test.run=^TestGateProcess$", "--", "true")
	if code := exitCode(nested.Wait()); code != 1 {
		t.Fatalf("nested exit = %d", code)
	}
}

func TestLauncherLossAndStartFailure(t *testing.T) {
	first, second := fixture(t)
	failed, _ := gate(t, first, filepath.Join(first, "missing-executable"))
	if code := exitCode(failed.Wait()); code != 1 {
		t.Fatalf("start failure exit = %d", code)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	entered, stopped := filepath.Join(first, "entered"), filepath.Join(first, "stopped")
	cmd := exec.Command(os.Args[0], "-test.run=^TestGateProcess$", "--", "--watch-parent", "sh", "-c",
		`trap 'touch "$2"; exit' TERM; touch "$1"; while :; do sleep 0.05; done`, "sh", entered, stopped)
	cmd.Dir = first
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "CHECK_GATE_TEST_PROCESS=1"}
	cmd.ExtraFiles = []*os.File{reader}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	waitFile(t, entered)
	_ = writer.Close()
	if code := exitCode(cmd.Wait()); code != 143 {
		t.Fatalf("launcher loss exit = %d", code)
	}
	waitFile(t, stopped)
	next, _ := gate(t, second, "true")
	if err := next.Wait(); err != nil {
		t.Fatal(err)
	}
}
