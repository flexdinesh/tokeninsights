// Package serverstore opens canonical-only server databases without producer recovery.
package serverstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	_ "modernc.org/sqlite"
)

const SupportedSchemaVersion = 1
const ApplicationID = 0x54495356

//go:embed schema/server.sql
var Schema string

type Store struct{ database *sql.DB }
type Metadata struct {
	DatabaseID        string
	OwnerID           string
	IdentityVersion   int
	SemanticsVersion  int
	Revision          int64
	LastIngestionAtMs int64
	CreatedAtMs       int64
}

func (s *Store) SQL() *sql.DB { return s.database }
func (s *Store) Close() error { return s.database.Close() }
func (s *Store) BeginRead(ctx context.Context) (*sql.Tx, error) {
	return s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
}
func (s *Store) Metadata(ctx context.Context) (Metadata, error) { return ReadMetadata(ctx, s.database) }
func ReadMetadata(ctx context.Context, reader db.Reader) (Metadata, error) {
	var m Metadata
	err := reader.QueryRowContext(ctx, `SELECT database_id, owner_id, identity_version, semantics_version, revision, last_ingestion_at_ms, created_at_ms FROM server_metadata WHERE id = 1`).Scan(&m.DatabaseID, &m.OwnerID, &m.IdentityVersion, &m.SemanticsVersion, &m.Revision, &m.LastIngestionAtMs, &m.CreatedAtMs)
	return m, err
}

// Open inspects role and compatibility read-only before opening a writer.
func Open(path string) (*Store, error) {
	inspection, err := connect(path, "ro")
	if err != nil {
		return nil, err
	}
	err = inspect(inspection)
	_ = inspection.Close()
	if err != nil {
		return nil, err
	}
	database, err := connect(path, "rw")
	if err != nil {
		return nil, err
	}
	if err := inspect(database); err != nil {
		_ = database.Close()
		return nil, err
	}
	return &Store{database: database}, nil
}

func CreateIfMissing(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err == nil {
		return Open(abs)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Initialize privately and publish only a complete, checkpointed database.
	// A crash during initialization leaves no final role-zero file to poison
	// retries. Exclusive rename publishes one name atomically and never replaces
	// history; there is no second hard link that a crash could leave behind.
	file, err := os.CreateTemp(filepath.Dir(abs), ".tokeninsights-server-init-*.sqlite")
	if err != nil {
		return nil, err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary); _ = os.Remove(temporary + "-wal"); _ = os.Remove(temporary + "-shm") }()
	if err := file.Close(); err != nil {
		return nil, err
	}
	store, err := initialize(temporary)
	if err != nil {
		return nil, err
	}
	if err := store.Close(); err != nil {
		return nil, err
	}
	if err := publishDatabase(temporary, abs); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Open(abs)
		}
		return nil, err
	}
	return Open(abs)
}

func initialize(path string) (*Store, error) {
	database, err := connect(path, "rw")
	if err != nil {
		return nil, err
	}
	var tx *sql.Tx
	fail := func(err error) (*Store, error) {
		if tx != nil {
			_ = tx.Rollback()
		}
		_ = database.Close()
		return nil, err
	}
	if _, err := database.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fail(err)
	}
	tx, err = database.Begin()
	if err != nil {
		return fail(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(strings.Replace(Schema, "PRAGMA journal_mode = WAL;", "", 1)); err != nil {
		return fail(err)
	}
	var id [32]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(`INSERT INTO server_metadata(id,database_id,owner_id,identity_version,semantics_version,created_at_ms) VALUES(1,?,'default',1,1,?)`, hex.EncodeToString(id[:]), time.Now().UnixMilli()); err != nil {
		return fail(err)
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	return &Store{database: database}, nil
}

func connect(path, mode string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Set("mode", mode)
	// Reserve the writer before reading receipt state. Deferred read-to-write
	// upgrades can fail immediately under concurrent independent connections.
	if mode != "ro" {
		q.Set("_txlock", "immediate")
	}
	q.Add("_pragma", "foreign_keys(on)")
	q.Add("_pragma", "busy_timeout(5000)")
	if mode == "ro" {
		q.Add("_pragma", "query_only(true)")
	}
	u.RawQuery = q.Encode()
	database, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func inspect(database *sql.DB) error {
	var application, version int
	if err := database.QueryRow("PRAGMA application_id").Scan(&application); err != nil {
		return err
	}
	if application != ApplicationID {
		return fmt.Errorf("incompatible server database role")
	}
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != SupportedSchemaVersion {
		return fmt.Errorf("incompatible server schema version")
	}
	m, err := ReadMetadata(context.Background(), database)
	if err != nil {
		return err
	}
	if m.DatabaseID == "" || m.OwnerID == "" || m.IdentityVersion != 1 || m.SemanticsVersion != 1 {
		return fmt.Errorf("incompatible server metadata")
	}
	return nil
}
