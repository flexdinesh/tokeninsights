// Package localruntime composes command-owned acceptance, processing and queries.
// No listener, service discovery or detached process is required.
package localruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
)

var ErrOwned = errors.New("database already owned; close the viewer or stop the legacy service")

const visibilityPoll = 25 * time.Millisecond

type Runtime struct {
	Jobs        *syncjob.Store
	jobsDone    chan struct{}
	Store       *datastore.Store
	App         *appstore.Store
	Query       *queryclient.Client
	Destination *collector.Destination
	queries     analytics.Repository
	owner       *os.File
	cancel      context.CancelFunc
	done        chan struct{}
	once        sync.Once
}

func Open(ctx context.Context, collectorPath, dataPath string) (*Runtime, error) {
	path, _, err := serverownership.Identify(dataPath)
	if err != nil {
		return nil, err
	}
	return OpenWithApp(ctx, collectorPath, path, filepath.Join(filepath.Dir(path), "app.sqlite"))
}
func OpenWithApp(ctx context.Context, collectorPath, dataPath, appPath string) (*Runtime, error) {
	canonicalCollector, _, err := serverownership.Identify(collectorPath)
	if err != nil {
		return nil, err
	}
	for _, other := range []string{dataPath, appPath} {
		if err := collector.ValidatePaths(canonicalCollector+".jobs.sqlite", other); err != nil {
			return nil, err
		}
	}
	if err := collector.ValidatePaths(dataPath+".application.json", appPath); err != nil {
		return nil, err
	}

	for _, other := range []string{collectorPath, dataPath} {
		if err := collector.ValidatePaths(appPath, other); err != nil {
			return nil, err
		}
	}

	if err := collector.ValidatePaths(collectorPath, dataPath); err != nil {
		return nil, err
	}
	path, _, err := serverownership.Identify(dataPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	release, err := db.AcquireWriterLock(ctx, path+".service.op")
	if err != nil {
		return nil, err
	}
	defer release()
	owner, held, err := serverownership.Lifetime(path, true)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, ErrOwned
	}
	store, err := serverruntime.Open(ctx, path, datastore.Options{Kind: datastore.KindPersonal})
	if err != nil {
		_ = owner.Close()
		return nil, err
	}
	metadata, err := store.Metadata(ctx)
	if err != nil {
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	app, err := appstore.OpenPaired(ctx, appPath, path, metadata.DatabaseID, datastore.KindPersonal)
	if err != nil {
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	repository := accounts.NewSQLite(app, store)
	if err := repository.ImportLegacy(ctx, store.SQL()); err != nil {
		_ = app.Close()
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	if err := repository.EnsureDefault(ctx); err != nil {
		_ = app.Close()
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	jobs, err := syncjob.Open(ctx, collectorPath)
	if err != nil {
		_ = app.Close()
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	appPath, _, err = serverownership.Identify(appPath)
	if err != nil {
		_ = jobs.Close()
		_ = app.Close()
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	workerCtx, cancel := context.WithCancel(ctx)
	r := &Runtime{Jobs: jobs, jobsDone: make(chan struct{}), Store: store, App: app, owner: owner, cancel: cancel, done: make(chan struct{}), queries: analytics.DuckDB{Store: store}}
	r.Query = queryclient.NewDirect(server.NewDirectQuery(workerCtx, r.queries, ""))
	r.Destination = &collector.Destination{URL: "http://local", Identity: "http://local", DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, Local: true, Transport: collector.DirectDelivery{Receiver: store}}
	go func() { defer close(r.done); store.Run(workerCtx, func(error) {}) }()
	go r.runJobs(workerCtx, path, appPath)
	return r, nil
}

// WaitVisible includes interrupted generation recovery. Local ownership excludes
// competing collectors while the command's initial capture is being displayed.
func (r *Runtime) WaitVisible(ctx context.Context) error {
	ticker := time.NewTicker(visibilityPoll)
	defer ticker.Stop()
	for {
		status, err := r.queries.Status(ctx)
		if err != nil {
			return err
		}
		if status.Pending == 0 && status.Metadata.Generation == status.Metadata.TargetGeneration {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runtime) Close() error {
	var err error
	r.once.Do(func() {
		r.cancel()
		<-r.done
		<-r.jobsDone
		err = errors.Join(r.Store.Close(), r.Jobs.Close(), r.App.Close(), r.owner.Close())
	})
	return err
}

var _ io.Closer = (*Runtime)(nil)
