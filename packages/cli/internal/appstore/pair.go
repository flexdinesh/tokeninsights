package appstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
)

type attachment struct {
	ApplicationID string `json:"applicationId"`
}

const maxAttachmentBytes = 1024

// OpenPaired runs under exclusive token-store ownership. Its durable sidecar
// prevents loss/replacement of SQLite from silently resetting account identity.
// Restore the token DB, app DB and this guard together from a stopped backup.
func OpenPaired(ctx context.Context, appPath, dataPath, id, kind string) (*Store, error) {
	canonical, err := dbpath.Canonical(dataPath)
	if err != nil {
		return nil, err
	}
	marker := canonical + ".application.json"
	var saved attachment
	existing := false
	file, err := os.Open(marker)
	if err == nil {
		existing = true
		decoder := json.NewDecoder(io.LimitReader(file, maxAttachmentBytes))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&saved)
		if err == nil && decoder.Decode(new(struct{})) != io.EOF {
			err = errors.New("invalid_attachment")
		}
		_ = file.Close()
		if err != nil || saved.ApplicationID == "" {
			return nil, errors.New("application_pair_guard_mismatch")
		}
		database, err := connect(appPath, true)
		if err != nil {
			return nil, errors.New("paired_application_missing_restore_backup")
		}
		err = validate(ctx, database, id, kind)
		var instance string
		if err == nil {
			err = database.QueryRowContext(ctx, "SELECT instance_id FROM application_metadata WHERE id=1").Scan(&instance)
		}
		_ = database.Close()
		if err != nil || instance != saved.ApplicationID {
			return nil, errors.New("paired_application_mismatch_restore_backup")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	store, err := Open(ctx, appPath, id, kind)
	if err != nil {
		return nil, err
	}
	if existing {
		return store, nil
	}
	var instance string
	if err := store.SQL().QueryRowContext(ctx, "SELECT instance_id FROM application_metadata WHERE id=1").Scan(&instance); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := writeAttachment(marker, attachment{ApplicationID: instance}); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}
func writeAttachment(path string, value attachment) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".application-pair-*")
	if err != nil {
		return err
	}
	candidate := file.Name()
	defer func() { _ = os.Remove(candidate) }()
	err = json.NewEncoder(file).Encode(value)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	if err := os.Link(candidate, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}
