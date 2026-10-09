package localruntime

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
)

// ImportLegacy stages a new target while preserving verified source history.
func ImportLegacy(ctx context.Context, path, legacyPath string) error {
	if legacyPath == "" {
		return errors.New("--legacy-server-db-path required")
	}
	canonical, _, err := serverownership.Identify(path)
	if err != nil {
		return err
	}
	release, err := db.AcquireWriterLock(ctx, canonical+".service.op")
	if err != nil {
		return err
	}
	defer release()
	owner, held, err := serverownership.Lifetime(canonical, true)
	if err != nil {
		return err
	}
	if held {
		return ErrOwned
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
