// Package appstore owns application SQLite independently of token storage.
package appstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const ApplicationID = 1414091585
const SchemaVersion = 2

//go:embed schema/app.sql
var Schema string

type Store struct{ database *sql.DB }

func (s *Store) SQL() *sql.DB { return s.database }
func (s *Store) Close() error { return s.database.Close() }
func (s *Store) WriteTransaction(ctx context.Context, operation func(*sql.Tx) error) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Open verifies role and pairing before changing journal settings or schema.
func Open(ctx context.Context, path, databaseID, kind string) (*Store, error) {
	if databaseID == "" || (kind != "personal" && kind != "hosted") {
		return nil, errors.New("invalid_application_pair")
	}
	existing := true
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		existing = false
	} else if err != nil {
		return nil, err
	} else if !info.Mode().IsRegular() {
		return nil, errors.New("invalid_application_path")
	}
	if existing {
		db, err := connect(path, true)
		if err != nil {
			return nil, err
		}
		err = validate(ctx, db, databaseID, kind)
		_ = db.Close()
		if err != nil {
			return nil, err
		}
	} else {
		if err := initialize(ctx, path, databaseID, kind); err != nil {
			return nil, err
		}
	}
	database, err := connect(path, false)
	if err != nil {
		return nil, err
	}
	s := &Store{database: database}
	err = validate(ctx, database, databaseID, kind)
	if err == nil {
		_, err = database.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	}
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}
func connect(path string, readOnly bool) (*sql.DB, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	if readOnly {
		q.Set("mode", "ro")
	} else {
		q.Set("mode", "rw")
	}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(on)")
	u.RawQuery = q.Encode()
	database, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	return database, nil
}
func validate(ctx context.Context, database *sql.DB, id, kind string) error {
	var role, version int
	if err := database.QueryRowContext(ctx, "PRAGMA application_id").Scan(&role); err != nil {
		return err
	}
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if role != ApplicationID || version != SchemaVersion {
		return errors.New("incompatible_application_database")
	}
	var savedID, savedKind string
	if err := database.QueryRowContext(ctx, "SELECT database_id,server_kind FROM application_metadata WHERE id=1 AND role='application'").Scan(&savedID, &savedKind); err != nil {
		return err
	}
	if savedID != id || savedKind != kind {
		return errors.New("application_database_pair_mismatch")
	}
	return nil
}

// Publish a fully initialized SQLite file; a crash never leaves an empty live DB.
func initialize(ctx context.Context, path, id, kind string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".application-*.sqlite")
	if err != nil {
		return err
	}
	candidate := f.Name()
	defer func() { _ = os.Remove(candidate) }()
	if err := f.Close(); err != nil {
		return err
	}
	database, err := connect(candidate, false)
	if err != nil {
		return err
	}
	store := &Store{database: database}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		_ = store.Close()
		return err
	}
	instance := hex.EncodeToString(random[:])
	err = store.WriteTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, Schema); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO application_metadata(id,role,database_id,server_kind,instance_id) VALUES(1,'application',?,?,?)", id, kind, instance)
		return err
	})
	err = errors.Join(err, store.Close())
	if err != nil {
		return err
	}
	if err := os.Link(candidate, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
