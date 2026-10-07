package service

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

// ImportLegacy stages a new target while preserving verified source history.
func ImportLegacy(ctx context.Context, path, legacyPath string) error {
	if legacyPath == "" {
		return errors.New("--legacy-server-db-path required")
	}
	canonical, _, err := identify(path)
	if err != nil {
		return err
	}
	release, err := admission(ctx, canonical)
	if err != nil {
		return err
	}
	defer release()
	owner, held, err := lifetime(canonical, true)
	if err != nil {
		return err
	}
	if held {
		return errors.New("stop service before legacy import")
	}
	defer func() { _ = owner.Close() }()
	writer, err := db.AcquireWriterLock(ctx, canonical)
	if err != nil {
		return err
	}
	defer writer()
	store, err := datastore.OpenWithLegacy(ctx, canonical, legacyPath)
	if err != nil {
		return err
	}
	return store.Close()
}
