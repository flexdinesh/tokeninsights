package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

// PrepareFixture resets only the two compatible development databases. Keep
// database/lock inodes, WAL sidecars and unrelated files, including old databases.
func PrepareFixture(ctx context.Context, collectorPath, serverPath string) error {
	collectorPath, _, err := identify(collectorPath)
	if err != nil {
		return err
	}
	serverPath, key, err := identify(serverPath)
	if err != nil {
		return err
	}
	root := filepath.Dir(collectorPath)
	if filepath.Base(root) != ".tokeninsights-dev" || filepath.Base(collectorPath) != "collector.sqlite" || filepath.Base(serverPath) != "server.sqlite" || filepath.Dir(serverPath) != root {
		return fmt.Errorf("uncontrolled development databases")
	}
	release, err := admission(ctx, serverPath)
	if err != nil {
		return err
	}
	defer release()
	state, err := Probe(ctx, serverPath)
	if err != nil {
		return err
	}
	if state.Running {
		return fmt.Errorf("stop development service before recreating fixtures")
	}
	// Check both roles before resetting either file. Legacy databases stay intact.
	if err := inspectFixtureRoles(collectorPath, serverPath); err != nil {
		return err
	}
	writer, err := db.AcquireWriterLock(ctx, collectorPath)
	if err != nil {
		return err
	}
	defer writer()
	serverWriter, err := db.AcquireWriterLock(ctx, serverPath)
	if err != nil {
		return err
	}
	defer serverWriter()
	if err := inspectFixtureRoles(collectorPath, serverPath); err != nil {
		return err
	}
	if err := db.ResetAllLocked(ctx, collectorPath); err != nil {
		return err
	}
	server, err := serverstore.CreateIfMissing(serverPath)
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()
	tx, err := server.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"canonical_token_usage", "canonical_messages", "canonical_sessions", "usage_locations", "ingestion_receipts", "ingestion_producers"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE server_metadata SET database_id=?,revision=0,last_ingestion_at_ms=0,created_at_ms=? WHERE id=1`, instanceID(), time.Now().UnixMilli()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, name := range []string{"source", "home"} {
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	p, err := servicePaths(key, false)
	if err != nil {
		return err
	}
	if err := os.Remove(p.config); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func inspectFixtureRoles(collectorPath, serverPath string) error {
	if _, err := os.Stat(collectorPath); err == nil {
		collector, err := db.OpenWritable(collectorPath)
		if err != nil {
			return err
		}
		if err := collector.Close(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(serverPath); err == nil {
		server, err := serverstore.Open(serverPath)
		if err != nil {
			return err
		}
		if err := server.Close(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
