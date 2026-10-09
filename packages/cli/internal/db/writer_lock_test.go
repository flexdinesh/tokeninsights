package db

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWriterLockAliasesAndCancellation(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(root, "alias")
	if err := os.Symlink(realDir, aliasDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realDir, "nested", "usage.sqlite")
	release, err := AcquireWriterLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, alias := range []string{path, filepath.Join(aliasDir, "nested", "usage.sqlite")} {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		unlock, err := AcquireWriterLock(ctx, alias)
		cancel()
		if unlock != nil {
			unlock()
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("alias acquired held lock: %v", err)
		}
	}
	before, err := os.Stat(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	release()
	release() // Release is idempotent; never unlock a later owner's descriptor.
	unlock, err := AcquireWriterLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	after, err := os.Stat(path + ".lock")
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("lock file removed or replaced: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	missing := filepath.Join(root, "cancelled", "usage.sqlite")
	if _, err := AcquireWriterLock(ctx, missing); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lock: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(missing)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled acquisition created directory: %v", err)
	}
}

func TestWriterLockDatabaseSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	database, _, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	alias := filepath.Join(t.TempDir(), "linked.sqlite")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireWriterLock(context.Background(), alias)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if unlock, err := AcquireWriterLock(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		if unlock != nil {
			unlock()
		}
		t.Fatalf("symlink used a different lock: %v", err)
	}
}

func TestWriterLockSubprocessCrashRelease(t *testing.T) {
	if path := os.Getenv("TOKENINSIGHTS_LOCK_TEST_PATH"); path != "" {
		_, err := AcquireWriterLock(context.Background(), path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println("locked")
		// The parent kills this process; no deferred release can run.
		for {
			time.Sleep(time.Hour)
		}
	}
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWriterLockSubprocessCrashRelease$")
	command.Env = append(os.Environ(), "TOKENINSIGHTS_LOCK_TEST_PATH="+path)
	command.Stderr = os.Stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatalf("child lock handshake failed: %v", scanner.Err())
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	unlock, err := AcquireWriterLock(waitCtx, path)
	waitCancel()
	if unlock != nil {
		unlock()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("subprocess lock ignored: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("child unexpectedly exited successfully")
	}
	unlock, err = AcquireWriterLock(ctx, path)
	if err != nil {
		t.Fatalf("crash retained lock: %v", err)
	}
	unlock()
}
