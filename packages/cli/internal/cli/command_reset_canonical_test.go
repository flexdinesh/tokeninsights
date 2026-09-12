package cli

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestResetCanonicalRejectsPendingRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	database, _, err := db.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec("UPDATE database_lifecycle SET rebuild_pending = 1, rebuild_source_key = 'test-scope' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}

	err = Run(context.Background(), []string{"reset-canonical", "--confirm", "--db-path", path}, io.Discard, io.Discard, time.Now())
	if !errors.Is(err, db.ErrRebuildPending) {
		t.Fatalf("expected pending recovery error, got %v", err)
	}
	assertCLIQueryCount(t, database, "SELECT rebuild_pending FROM database_lifecycle WHERE id = 1", 1)
}
