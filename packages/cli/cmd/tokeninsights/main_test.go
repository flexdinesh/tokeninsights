package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"nope"},
		io.Discard,
		&stderr,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncRequiresHarnessSelection(t *testing.T) {
	err := runWithTime(
		context.Background(),
		[]string{"sync", "--db-path", filepath.Join(t.TempDir(), "test.sqlite")},
		io.Discard,
		io.Discard,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "choose --all or --harness") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResetAllWithoutConfirmDoesNotCreateDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	var stdout bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"reset-all", "--db-path", dbPath},
		&stdout,
		io.Discard,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Would delete") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestHelp(t *testing.T) {
	var stdout bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"--help"},
		&stdout,
		io.Discard,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "usage:") {
		t.Fatalf("expected usage in output: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "usage: tokeninsights <command>") {
		t.Fatalf("expected public command name in output: %q", stdout.String())
	}
}

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	err := runWithTime(
		context.Background(),
		[]string{"--version"},
		&stdout,
		io.Discard,
		time.Date(2026, 4, 24, 15, 0, 0, 0, time.Local),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "tokeninsights dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
