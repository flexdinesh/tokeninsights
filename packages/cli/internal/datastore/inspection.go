package datastore

import (
	"context"
	"errors"
	"os"
)

func (s *Store) checkFile() error {
	if s.path == "" {
		return nil
	}
	file, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if s.fileInfo != nil && !os.SameFile(s.fileInfo, info) {
		return errors.New("server_data_file_replaced")
	}
	var magic [4]byte
	if _, err := file.ReadAt(magic[:], 8); err != nil {
		return err
	}
	if string(magic[:]) != "DUCK" {
		return errors.New("invalid_server_data_file")
	}
	return nil
}

// Ready verifies the initialized owner contract without assuming a default
// dataset exists. Hosted stores are ready before the first user is provisioned.
func (s *Store) Ready(ctx context.Context) error {
	tx, err := s.BeginRead(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var role, databaseID, kind string
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT role,schema_version,database_id,server_kind FROM ingestion.instance WHERE id=1").Scan(&role, &version, &databaseID, &kind); err != nil {
		return err
	}
	if role != "server-data" || version != SchemaVersion || databaseID == "" || kind != s.kind {
		return errors.New("incompatible_server_data")
	}
	return nil
}

// InspectKind validates the current storage contract read-only.
func InspectKind(ctx context.Context, path, kind string) error {
	if kind != KindPersonal && kind != KindHosted {
		return errors.New("invalid_server_kind")
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return inspectPath(ctx, path, kind)
}
func Inspect(ctx context.Context, path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return inspectPath(ctx, path, "")
}
