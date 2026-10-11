// Package localruntime composes command-owned acceptance, processing and queries.
// No listener, service discovery or detached process is required.
package localruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlite"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqliteaccounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
)

var ErrOwned = errors.New("database already owned; close the process using this database")

var ErrProcessingFailed = errors.New("processing_failed: saved usage needs attention; retry or run tokeninsights data reprocess")
var ErrProcessingTimeout = fmt.Errorf("processing_timeout: usage is still processing; retry or view saved data: %w", context.DeadlineExceeded)

const visibilityPoll = 25 * time.Millisecond

type Runtime struct {
	InstanceID     string
	Hostname       string
	Policy         serverfeatures.Policy
	Progress       *collectorprogress.Registry
	Store          *datastore.Store
	App            *appstore.Store
	Query          *queryclient.Client
	Destination    *collector.Destination
	queries        analytics.Repository
	owner          *os.File
	cancel         context.CancelFunc
	ctx            context.Context
	collectorPath  string
	dataPath       string
	lifecycleMu    sync.Mutex
	closing        bool
	collections    sync.WaitGroup
	captureDetails bool
	done           chan struct{}
	once           sync.Once
}

// Options selects command-owned local behavior at composition time.
// Only the TUI enables capture details; public progress retains its wire shape.
type Options struct {
	CaptureDetails bool
}

func Open(ctx context.Context, collectorPath, dataPath string) (*Runtime, error) {
	path, _, err := serverownership.Identify(dataPath)
	if err != nil {
		return nil, err
	}
	return OpenWithApp(ctx, collectorPath, path, filepath.Join(filepath.Dir(path), "app.sqlite"))
}
func OpenWithApp(ctx context.Context, collectorPath, dataPath, appPath string) (*Runtime, error) {
	return OpenWithAppOptions(ctx, collectorPath, dataPath, appPath, Options{})
}

func OpenWithAppOptions(ctx context.Context, collectorPath, dataPath, appPath string, options Options) (*Runtime, error) {
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
	store, err := sqlite.Open(ctx, path, datastore.Options{Kind: datastore.KindPersonal})
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
	repository := sqliteaccounts.NewSQLite(app, store)
	if err := repository.EnsureDefault(ctx); err != nil {
		_ = app.Close()
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	workerCtx, cancel := context.WithCancel(ctx)
	policy, err := serverfeatures.NewLocalViewer(true)
	if err != nil {
		cancel()
		_ = app.Close()
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	var instance [16]byte
	if _, err := rand.Read(instance[:]); err != nil {
		cancel()
		_ = app.Close()
		_ = store.Close()
		_ = owner.Close()
		return nil, err
	}
	hostname := resolveHostname(os.Hostname)
	id := hex.EncodeToString(instance[:])
	r := &Runtime{InstanceID: id, Hostname: hostname, Policy: policy, Progress: collectorprogress.New(id), Store: store, App: app, owner: owner, cancel: cancel, ctx: workerCtx, collectorPath: canonicalCollector, dataPath: path, done: make(chan struct{}), queries: sqlanalytics.Queries{Store: store}, captureDetails: options.CaptureDetails}
	r.Query = queryclient.NewDirect(server.NewDirectQuery(r.queries, id, hostname))
	r.Destination = &collector.Destination{Identity: "http://local", DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, Local: true, Transport: collector.DirectDelivery{Receiver: store}}
	// WaitVisible reads durable failure state, including failures from a prior owner.
	go func() { defer close(r.done); store.Run(workerCtx, nil) }()
	return r, nil
}

func (r *Runtime) Observe(ctx context.Context) *Observer {
	if !r.Policy.Capabilities.Has(serverfeatures.CollectorProgress) {
		return newObserver(ctx, nil)
	}
	observer := newObserver(ctx, func(_ context.Context, message collectorprogress.Message) error {
		return r.Progress.Apply(message)
	})
	if r.captureDetails && observer.publish != nil {
		observer.publishCapture = r.Progress.UpdateCapture
	}
	return observer
}

func resolveHostname(lookup func() (string, error)) string {
	hostname, err := lookup()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "unknown"
	}
	return strings.TrimSpace(hostname)
}

// WaitVisible reads durable publication readiness, including interrupted generation recovery.
func (r *Runtime) WaitVisible(ctx context.Context) error {
	ticker := time.NewTicker(visibilityPoll)
	defer ticker.Stop()
	for {
		status, err := r.queries.Status(ctx)
		if err != nil {
			return visibilityError(ctx, err)
		}
		if status.Pending == 0 && status.Metadata.Generation == status.Metadata.TargetGeneration {
			return nil
		}
		// Let due retries run before reporting an old failure. Otherwise finite
		// commands cancel their worker before it can recover a transient error.
		if status.Failed > 0 && status.FailedRetryAtMs > time.Now().UnixMilli() {
			return ErrProcessingFailed
		}
		select {
		case <-ctx.Done():
			return visibilityError(ctx, ctx.Err())
		case <-ticker.C:
		}
	}
}

func visibilityError(ctx context.Context, err error) error {
	// SQL queries may report Interrupted instead of wrapping ctx.Err().
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrProcessingTimeout
	}
	return err
}

// ProcessingStatus describes durable readiness without waiting for publication.
func (r *Runtime) ProcessingStatus(ctx context.Context) (analytics.ProcessingStatus, error) {
	return r.queries.Status(ctx)
}

func (r *Runtime) Close() error {
	var err error
	r.once.Do(func() {
		r.lifecycleMu.Lock()
		r.closing = true
		r.cancel()
		r.lifecycleMu.Unlock()
		r.collections.Wait()
		<-r.done
		r.Progress.InterruptAll()
		err = errors.Join(r.Store.Close(), r.App.Close(), r.owner.Close())
	})
	return err
}

var _ io.Closer = (*Runtime)(nil)
