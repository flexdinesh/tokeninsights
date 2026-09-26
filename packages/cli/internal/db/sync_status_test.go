package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestAbandonedJobIsInterruptedUntilNextWriter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	database, _, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	release, err := AcquireWriterLock(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO sync_jobs (scope_key, status, phase, started_at_ms, updated_at_ms, normalize, all_harnesses) VALUES ('scope', 'running', 'syncing', 1, 1, 1, 1)"); err != nil {
		t.Fatal(err)
	}
	status, err := ReadSyncStatus(ctx, path)
	if err != nil || !status.Running {
		t.Fatalf("active owner: %+v %v", status, err)
	}
	release()
	status, err = ReadSyncStatus(ctx, path)
	if err != nil || status.Running || status.Phase != "interrupted" {
		t.Fatalf("abandoned owner: %+v %v", status, err)
	}
}

func TestCoverageCalendarDSTAndUnknownDoesNotMeanZero(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	database, _, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	zone, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = zone
	defer func() { time.Local = old }()
	f := Filter{DayFrom: "2026-10-03", DayTo: "2026-10-05"}
	coverage, err := ViewerDayCoverage(ctx, database, f, time.Date(2026, 10, 5, 12, 0, 0, 0, zone))
	if err != nil || len(coverage) != 3 {
		t.Fatalf("DST dates: %+v %v", coverage, err)
	}
	for _, day := range coverage {
		if day.Status != "unverified" || day.Total != nil {
			t.Fatalf("unknown implied zero: %+v", day)
		}
	}
}
