// Package syncjob owns finite sync requests independently of collector locks.
package syncjob

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const ApplicationID = 1414091594
const SchemaVersion = 1
const PollInterval = 100 * time.Millisecond
const Timeout = 10 * time.Minute
const terminalHistoryLimit = 1000

//go:embed schema/jobs.sql
var Schema string

type Spec struct {
	Debug         bool               `json:"debug,omitempty"`
	Mode          string             `json:"mode"`
	CollectorPath string             `json:"collectorPath"`
	DataPath      string             `json:"dataPath"`
	AppPath       string             `json:"appPath"`
	URL           string             `json:"url,omitempty"`
	Credential    string             `json:"credential,omitempty"`
	Harnesses     []pipeline.Harness `json:"harnesses"`
	SourceDir     string             `json:"sourceDir,omitempty"`
	PublishOnly   bool               `json:"publishOnly,omitempty"`
	FullRefresh   bool               `json:"fullRefresh,omitempty"`
}

func Fingerprint(secret string) string {
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
func NewSpec(settings config.Settings, harnesses []pipeline.Harness, source string, publishOnly, fullRefresh bool) (Spec, error) {
	collector, _, err := serverownership.Identify(settings.CollectorDBPath)
	if err != nil {
		return Spec{}, err
	}
	data, _, err := serverownership.Identify(settings.ServerDBPath)
	if err != nil {
		return Spec{}, err
	}
	app, _, err := serverownership.Identify(settings.ApplicationPath())
	if err != nil {
		return Spec{}, err
	}
	if source != "" {
		source, err = filepath.Abs(source)
		if err != nil {
			return Spec{}, err
		}
	}
	return Spec{Mode: settings.EffectiveMode(), CollectorPath: collector, DataPath: data, AppPath: app, URL: settings.ServerURL, Credential: Fingerprint(settings.ServerToken), Harnesses: harnesses, SourceDir: source, PublishOnly: publishOnly, FullRefresh: fullRefresh}, nil
}

type Job struct {
	Receipts   []evidence.Receipt `json:"receipts,omitempty"`
	ID         string             `json:"id"`
	Spec       Spec               `json:"-"`
	State      string             `json:"state"`
	DatabaseID string             `json:"databaseId,omitempty"`
	DatasetID  string             `json:"datasetId,omitempty"`
	Attempts   int                `json:"attempts"`
	Accepted   int64              `json:"accepted"`
	Error      string             `json:"error,omitempty"`
	Created    int64              `json:"created"`
	Updated    int64              `json:"updated"`
}

func (j Job) Terminal() bool {
	return j.State == "accepted" || j.State == "failed" || j.State == "interrupted"
}

type Store struct {
	database *sql.DB
	Path     string
}

func (s *Store) Close() error { return s.database.Close() }
func Open(ctx context.Context, collectorPath string) (*Store, error) {
	canonical, _, err := serverownership.Identify(collectorPath)
	if err != nil {
		return nil, err
	}
	path := canonical + ".jobs.sqlite"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	release, err := db.AcquireWriterLock(ctx, path+".init")
	if err != nil {
		return nil, err
	}
	defer release()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := initialize(ctx, path); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	database, err := connect(path)
	if err != nil {
		return nil, err
	}
	var role, version int
	if err = database.QueryRowContext(ctx, "PRAGMA application_id").Scan(&role); err == nil {
		err = database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	}
	if err == nil && (role != ApplicationID || version != SchemaVersion) {
		err = errors.New("incompatible_sync_jobs")
	}
	if err == nil {
		_, err = database.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	}
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return &Store{database: database, Path: path}, nil
}
func connect(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	q.Set("mode", "rw")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	database, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	return database, nil
}
func initialize(ctx context.Context, path string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".sync-jobs-*.sqlite")
	if err != nil {
		return err
	}
	candidate := file.Name()
	defer func() { _ = os.Remove(candidate) }()
	if err := file.Close(); err != nil {
		return err
	}
	database, err := connect(candidate)
	if err != nil {
		return err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err == nil {
		_, err = tx.ExecContext(ctx, Schema)
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	err = errors.Join(err, database.Close())
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
func (s *Store) Enqueue(ctx context.Context, spec Spec) (Job, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Job{}, err
	}
	body, err := json.Marshal(spec)
	if err != nil {
		return Job{}, err
	}
	now := time.Now().UnixMilli()
	id := hex.EncodeToString(random[:])
	_, err = s.database.ExecContext(ctx, "INSERT INTO jobs(id,spec,state,created_ms,updated_ms) VALUES(?,?,'queued',?,?)", id, string(body), now, now)
	return Job{ID: id, Spec: spec, State: "queued", Created: now, Updated: now}, err
}

const columns = "id,spec,state,database_id,dataset_id,attempts,accepted,receipts,error_code,created_ms,updated_ms"

func scan(row *sql.Row) (Job, error) {
	var job Job
	var body, receipts string
	err := row.Scan(&job.ID, &body, &job.State, &job.DatabaseID, &job.DatasetID, &job.Attempts, &job.Accepted, &receipts, &job.Error, &job.Created, &job.Updated)
	if err == nil {
		err = json.Unmarshal([]byte(body), &job.Spec)
		if err == nil {
			err = json.Unmarshal([]byte(receipts), &job.Receipts)
		}
	}
	return job, err
}
func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	return scan(s.database.QueryRowContext(ctx, "SELECT "+columns+" FROM jobs WHERE id=?", id))
}
func (s *Store) Latest(ctx context.Context) (Job, error) {
	return scan(s.database.QueryRowContext(ctx, "SELECT "+columns+" FROM jobs ORDER BY created_ms DESC,rowid DESC LIMIT 1"))
}

// Acquire excludes workers without holding a SQLite transaction during capture.
func (s *Store) Acquire() (*os.File, bool, error) {
	return serverownership.Lifetime(s.Path+".worker", true)
}
func (s *Store) Start(ctx context.Context, id string) error {
	// The caller holds Acquire. Every previous running claim therefore lost its owner.
	if _, err := s.database.ExecContext(ctx, "UPDATE jobs SET state='interrupted',error_code='worker_interrupted',updated_ms=? WHERE state='running'", time.Now().UnixMilli()); err != nil {
		return err
	}
	_, err := s.database.ExecContext(ctx, "UPDATE jobs SET state='running',attempts=attempts+1,error_code='',updated_ms=? WHERE id=? AND state='queued'", time.Now().UnixMilli(), id)
	return err
}
func (s *Store) Bind(ctx context.Context, id, database, dataset string) error {
	result, err := s.database.ExecContext(ctx, "UPDATE jobs SET database_id=?,dataset_id=? WHERE id=? AND state='running' AND (database_id='' OR (database_id=? AND dataset_id=?))", database, dataset, id, database, dataset)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("sync_destination_changed")
	}
	return nil
}
func (s *Store) Finish(ctx context.Context, id, state, code string, accepted int64) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "UPDATE jobs SET state=?,error_code=?,accepted=?,updated_ms=? WHERE id=?", state, code, accepted, time.Now().UnixMilli(), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM jobs WHERE state IN ('accepted','failed','interrupted') AND id NOT IN (SELECT id FROM jobs WHERE state IN ('accepted','failed','interrupted') ORDER BY updated_ms DESC,rowid DESC LIMIT ?)", terminalHistoryLimit); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) NextLocal(ctx context.Context, dataPath, appPath string) (Job, error) {
	return scan(s.database.QueryRowContext(ctx, "SELECT "+columns+" FROM jobs WHERE state='queued' AND json_extract(spec,'$.mode')=? AND json_extract(spec,'$.dataPath')=? AND json_extract(spec,'$.appPath')=? ORDER BY created_ms,rowid LIMIT 1", config.SingleProcess, dataPath, appPath))
}
func (s *Store) Wait(ctx context.Context, id string) (Job, error) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		job, err := s.Get(ctx, id)
		if err != nil {
			return job, err
		}
		if job.Terminal() {
			return job, nil
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-ticker.C:
		}
	}
}

// RefreshAbandoned is safe only after proving no worker holds the OS lock.
func (s *Store) RefreshAbandoned(ctx context.Context) error {
	owner, held, err := s.Acquire()
	if err != nil {
		return err
	}
	if held {
		return nil
	}
	defer func() { _ = owner.Close() }()
	_, err = s.database.ExecContext(ctx, "UPDATE jobs SET state='interrupted',error_code='worker_interrupted',updated_ms=? WHERE state='running'", time.Now().UnixMilli())
	return err
}

func (s *Store) SaveReceipt(ctx context.Context, id string, receipt evidence.Receipt) error {
	body, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = s.database.ExecContext(ctx, "UPDATE jobs SET receipts=json_insert(receipts,'$[#]',json(?)),updated_ms=? WHERE id=?", string(body), time.Now().UnixMilli(), id)
	return err
}
func (s *Store) NextRemote(ctx context.Context, job Job) (Job, error) {
	return scan(s.database.QueryRowContext(ctx, "SELECT "+columns+" FROM jobs WHERE state='queued' AND json_extract(spec,'$.mode')=? AND json_extract(spec,'$.url')=? AND json_extract(spec,'$.credential')=? AND rowid<=(SELECT rowid FROM jobs WHERE id=?) ORDER BY rowid LIMIT 1", config.Distributed, job.Spec.URL, job.Spec.Credential, job.ID))
}
