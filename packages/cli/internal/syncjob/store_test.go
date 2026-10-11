package syncjob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestChangedSchemaRejectsWithoutMutation(t *testing.T) {
	collector := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := Open(t.Context(), collector)
	if err != nil {
		t.Fatal(err)
	}
	path := store.Path
	if _, err := store.database.ExecContext(t.Context(), "ALTER TABLE jobs ADD COLUMN unexpected TEXT"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(t.Context(), collector); err == nil {
		_ = other.Close()
		t.Fatal("changed schema accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejection mutated jobs", err)
	}
}

func TestQueueAdmissionDoesNotRequireCollectorLockAndRecoversAbandonedClaim(t *testing.T) {
	root := t.TempDir()
	settings := config.SyncSettings{ServerURL: "https://remote.example", ServerToken: "fixture-token"}
	settings.CollectorDBPath = filepath.Join(root, "collector.sqlite")
	spec, err := NewSpec(settings, []pipeline.Harness{pipeline.HarnessPi}, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), spec.CollectorPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	first, err := store.Enqueue(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	owner, held, err := store.Acquire()
	if err != nil || held {
		t.Fatal(err)
	}
	if err := store.Start(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Bind(t.Context(), first.ID, "database", "dataset"); err != nil {
		t.Fatal(err)
	}
	if err := store.Bind(t.Context(), first.ID, "database", "other-user"); err == nil {
		t.Fatal("account rebound")
	}
	second, err := store.Enqueue(t.Context(), spec)
	if err != nil {
		t.Fatal("enqueue blocked by running job", err)
	}
	if err := store.RefreshAbandoned(t.Context()); err != nil {
		t.Fatal(err)
	}
	running, err := store.Get(t.Context(), first.ID)
	if err != nil || running.State != "running" {
		t.Fatal("live worker interrupted", running, err)
	}
	_ = owner.Close()
	if err := store.RefreshAbandoned(t.Context()); err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Get(t.Context(), first.ID)
	if err != nil || interrupted.State != "interrupted" {
		t.Fatal(interrupted, err)
	}
	next, err := store.NextRemote(t.Context(), second)
	if err != nil || next.ID != second.ID {
		t.Fatal("follow-up lost", next, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), spec.CollectorPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	saved, err := reopened.Get(t.Context(), second.ID)
	if err != nil || saved.State != "queued" {
		t.Fatal("pending job lost", saved, err)
	}
	if _, err := os.Stat(spec.CollectorPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("enqueue opened collector", err)
	}
}
func TestSpawnFailureAndOwnerWaitAreBounded(t *testing.T) {
	job := Job{ID: "fixture", Spec: Spec{CollectorPath: filepath.Join(t.TempDir(), "collector.sqlite")}}
	if err := spawn(t.Context(), "/missing/tokeninsights", job, "secret"); err == nil {
		t.Fatal("spawn succeeded")
	}
	store, err := Open(t.Context(), job.Spec.CollectorPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	owner, _, err := store.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if release, err := WaitForOwner(ctx, store); !errors.Is(err, context.DeadlineExceeded) {
		if release != nil {
			release()
		}
		t.Fatal(err)
	}
}

func TestTerminalRetentionNeverDeletesQueuedRequests(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	pending, err := store.Enqueue(t.Context(), Spec{Mode: distributedMode})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.database.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= terminalHistoryLimit; i++ {
		if _, err := tx.ExecContext(t.Context(), "INSERT INTO jobs(id,spec,state,created_ms,updated_ms) VALUES(?,'{}','accepted',?,?)", fmt.Sprint(i), i, i); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(t.Context(), fmt.Sprint(terminalHistoryLimit), "accepted", "", 0); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Get(t.Context(), pending.ID)
	if err != nil || saved.State != "queued" {
		t.Fatal("pending request pruned", saved, err)
	}
	var count int
	if err := store.database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM jobs WHERE state='accepted'").Scan(&count); err != nil || count != terminalHistoryLimit {
		t.Fatal("unbounded history", count, err)
	}
}

func TestRemovedLocalJobsAreReportedWithoutMutationOrRemoteDispatch(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	local, err := store.Enqueue(t.Context(), Spec{Mode: "single-process"})
	if err != nil {
		t.Fatal(err)
	}
	reported, err := store.Latest(t.Context())
	if err != nil || reported.State != "unsupported" || reported.Error != "local_sync_removed" {
		t.Fatal(reported, err)
	}
	var state string
	if err := store.database.QueryRow("SELECT state FROM jobs WHERE id=?", local.ID).Scan(&state); err != nil || state != "queued" {
		t.Fatal("old job mutated", state, err)
	}
	remote, err := store.Enqueue(t.Context(), Spec{Mode: distributedMode, URL: "https://remote.example", Credential: "fingerprint"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.NextRemote(t.Context(), remote)
	if err != nil || next.ID != remote.ID {
		t.Fatal("local request routed remotely", next, err)
	}
}
