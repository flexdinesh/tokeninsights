// Package postgres owns a single server's PostgreSQL connections and paired
// schemas. The owned connection executes every write and never reconnects.
package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const SchemaVersion = 1
const AccountSchemaVersion = 1
const ownerLock = int64(1414091604)
const connectionLimit = 8
const ownershipCheckInterval = time.Second
const ownershipCheckTimeout = 5 * time.Second

//go:embed schema/data.sql
var DataSchema string

//go:embed schema/app.sql
var AccountSchema string

type Store struct {
	Reader     *sql.DB
	Writer     sync.Mutex
	connection *sql.Conn
	ownerPool  *sql.DB
	ctx        context.Context
	cancel     context.CancelCauseFunc
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

// Configuration errors are intentionally fixed: driver errors may contain DSNs.
func Connect(dsn string) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid_postgres_configuration")
	}
	config.RuntimeParams["search_path"] = "pg_catalog,tokeninsights_data,tokeninsights_accounts"
	config.RuntimeParams["synchronous_commit"] = "on"
	config.ConnectTimeout = ownershipCheckTimeout
	database := stdlib.OpenDB(*config)
	database.SetMaxOpenConns(connectionLimit)
	return database, nil
}

func Open(parent context.Context, dsn string) (*Store, error) {
	pool, err := Connect(dsn)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(1)
	connection, err := pool.Conn(parent)
	if err != nil {
		_ = pool.Close()
		return nil, errors.New("postgres_connection_failed")
	}
	ctx, cancel := context.WithCancelCause(parent)
	s := &Store{connection: connection, ownerPool: pool, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	started := false
	defer func() {
		if !started {
			cancel(context.Canceled)
			_ = connection.Close()
			_ = pool.Close()
			if s.Reader != nil {
				_ = s.Reader.Close()
			}
		}
	}()
	var owned bool
	if err := connection.QueryRowContext(parent, "SELECT pg_try_advisory_lock($1)", ownerLock).Scan(&owned); err != nil {
		return nil, errors.New("postgres_ownership_failed")
	}
	if !owned {
		return nil, errors.New("server already owns database")
	}
	var version int
	if err := connection.QueryRowContext(parent, "SELECT current_setting('server_version_num')::integer").Scan(&version); err != nil {
		return nil, errors.New("postgres_version_unavailable")
	}
	if version < 180000 || version >= 190000 {
		return nil, errors.New("unsupported_postgres_version")
	}
	if err := s.initialize(parent); err != nil {
		return nil, err
	}
	s.Reader, err = Connect(dsn)
	if err != nil {
		return nil, err
	}
	if err := s.validate(parent); err != nil {
		return nil, err
	}
	started = true
	go s.monitor()
	return s, nil
}

func (s *Store) Context() context.Context { return s.ctx }
func (s *Store) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return context.Cause(s.ctx)
}
func (s *Store) BeginWrite(ctx context.Context) (*sql.Tx, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	tx, err := s.connection.BeginTx(ctx, nil)
	if errors.Is(err, sql.ErrConnDone) {
		s.cancel(errors.New("postgres_ownership_lost"))
	}
	return tx, err
}
func (s *Store) SQL() *sql.DB { return s.Reader }
func (s *Store) WriteTransaction(ctx context.Context, operation func(*sql.Tx) error) error {
	s.Writer.Lock()
	defer s.Writer.Unlock()
	tx, err := s.BeginWrite(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) monitor() {
	defer close(s.done)
	ticker := time.NewTicker(ownershipCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// A transaction currently owns the connection; its SQL detects loss.
			if !s.Writer.TryLock() {
				continue
			}
			ctx, cancel := context.WithTimeout(s.ctx, ownershipCheckTimeout)
			err := s.connection.PingContext(ctx)
			cancel()
			s.Writer.Unlock()
			if err != nil {
				s.cancel(errors.New("postgres_ownership_lost"))
				return
			}
		}
	}
}

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.cancel(context.Canceled)
		<-s.done
		s.Writer.Lock()
		defer s.Writer.Unlock()
		// Close the physical session by closing its pool, not by returning an
		// advisory-locked connection to a reusable application pool.
		s.closeErr = errors.Join(s.Reader.Close(), s.connection.Close(), s.ownerPool.Close())
	})
	return s.closeErr
}

func (s *Store) initialize(ctx context.Context) error {
	var data, app bool
	if err := s.connection.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='tokeninsights_data'),EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='tokeninsights_accounts')").Scan(&data, &app); err != nil {
		return errors.New("postgres_schema_unavailable")
	}
	if data || app {
		if data != app {
			return errors.New("postgres_pair_incomplete")
		}
		return nil
	}
	tx, err := s.connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, DataSchema+AccountSchema); err != nil {
		return fmt.Errorf("postgres_schema_initialization: %w", err)
	}
	databaseID, err := evidence.RandomID()
	if err != nil {
		return err
	}
	instanceID, err := evidence.RandomID()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO tokeninsights_data.ingestion_instance(id,role,schema_version,database_id,server_kind,created_at_ms,application_instance_id) VALUES(1,'server-data',$1,$2,'hosted',$3,$4)", SchemaVersion, databaseID, time.Now().UnixMilli(), instanceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO tokeninsights_accounts.application_metadata(id,schema_version,role,database_id,instance_id,server_kind) VALUES(1,$1,'application',$2,$3,'hosted')", AccountSchemaVersion, databaseID, instanceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Ready(ctx context.Context) error {
	if err := s.Check(ctx); err != nil {
		return err
	}
	var paired bool
	err := s.Reader.QueryRowContext(ctx, `SELECT d.role='server-data' AND d.schema_version=1 AND a.role='application' AND a.schema_version=1 AND d.server_kind='hosted' AND a.server_kind='hosted' AND d.database_id=a.database_id AND d.application_instance_id=a.instance_id AND d.database_id<>'' AND a.instance_id<>'' FROM tokeninsights_data.ingestion_instance d CROSS JOIN tokeninsights_accounts.application_metadata a WHERE d.id=1 AND a.id=1`).Scan(&paired)
	if err != nil || !paired {
		return errors.New("postgres_pair_mismatch")
	}
	return nil
}
