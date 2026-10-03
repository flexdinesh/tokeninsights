package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

// PrepareFixture is restricted to the repository's development fixture. Never
// unlink DB/lock inodes; reset transactionally while holding both admission and
// writer ownership, and remove only the synthetic source/output files.
func PrepareFixture(ctx context.Context, path string) error {
	path, key, err := identify(path)
	if err != nil {
		return err
	}
	root := filepath.Dir(path)
	if filepath.Base(root) != ".tokeninsights-dev" || filepath.Base(path) != "tokeninsights.sqlite" {
		return fmt.Errorf("uncontrolled development database")
	}
	release, err := admission(ctx, path)
	if err != nil {
		return err
	}
	defer release()
	state, err := Probe(ctx, path)
	if err != nil {
		return err
	}
	if state.Running {
		return fmt.Errorf("stop development service before recreating fixtures")
	}
	if _, err := db.InspectCompatibility(ctx, path); err != nil {
		return err
	}
	writer, err := db.AcquireWriterLock(ctx, path)
	if err != nil {
		return err
	}
	defer writer()
	if err := db.ResetAllLocked(ctx, path); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	keep := map[string]bool{filepath.Base(path): true, filepath.Base(path) + "-wal": true, filepath.Base(path) + "-shm": true, filepath.Base(path) + ".lock": true, filepath.Base(path) + ".service.lock": true, filepath.Base(path) + ".service.op.lock": true}
	for _, entry := range entries {
		if !keep[entry.Name()] {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
	}
	// Next foreground fixture run captures its sanitized environment again.
	p, err := servicePaths(key, false)
	if err != nil {
		return err
	}
	if err := os.Remove(p.config); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
