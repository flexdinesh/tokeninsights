package tui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/collector"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
)

// Fixture owners retain real capture, processing and storage behind the same
// presentation contract as a production lease. Viewers never own these stores.
type fixtureBackend struct {
	runtime *localruntime.Runtime
	ctx     context.Context
}
type fixtureSession struct {
	backend *fixtureBackend
	done    chan struct{}
	once    sync.Once
}

var fixtureBackends = map[string]*fixtureBackend{}
var fixtureCollector = collector.Run

func replaceViewCollector(t *testing.T, run func(context.Context, collector.Options) (collector.Result, error)) {
	t.Helper()
	previous := fixtureCollector
	fixtureCollector = run
	t.Cleanup(func() { fixtureCollector = previous })
}

func fixtureConnect(t *testing.T) func(context.Context, config.LocalSettings, func() error) (Session, error) {
	t.Helper()
	return func(ctx context.Context, settings config.LocalSettings, prepare func() error) (Session, error) {
		backend := fixtureBackends[settings.ServerDBPath]
		if backend == nil {
			if err := prepare(); err != nil {
				return nil, err
			}
			runtime, err := localruntime.OpenWithAppOptions(t.Context(), settings.CollectorDBPath, settings.ServerDBPath, settings.ApplicationPath(), localruntime.Options{CaptureDetails: true})
			if err != nil {
				return nil, err
			}
			backend = &fixtureBackend{runtime: runtime, ctx: t.Context()}
			fixtureBackends[settings.ServerDBPath] = backend
			t.Cleanup(func() { delete(fixtureBackends, settings.ServerDBPath); _ = runtime.Close() })
		}
		return &fixtureSession{backend: backend, done: make(chan struct{})}, ctx.Err()
	}
}

func fixtureRuntime(t *testing.T, session Session) *localruntime.Runtime {
	t.Helper()
	fixture, ok := session.(*fixtureSession)
	if !ok {
		t.Fatal("session has no fixture owner")
		return nil
	}
	return fixture.backend.runtime
}

func (s *fixtureSession) Queries() *queryclient.Client { return s.backend.runtime.Query }
func (s *fixtureSession) Done() <-chan struct{}        { return s.done }
func (s *fixtureSession) Close() error                 { s.once.Do(func() { close(s.done) }); return nil }
func (s *fixtureSession) Progress(ctx context.Context) (collectorprogress.DetailSnapshot, error) {
	return s.backend.runtime.Progress.Details(), ctx.Err()
}
func (s *fixtureSession) ProcessingStatus(ctx context.Context) (analytics.ProcessingStatus, error) {
	return s.backend.runtime.ProcessingStatus(ctx)
}
func (s *fixtureSession) Collect(ctx context.Context, fullRefresh bool, sourceDir string) (localprotocol.CollectionResult, error) {
	done := s.backend.runtime.StartCollection(s.backend.ctx, pipeline.SyncOptions{
		Harnesses: pipeline.SupportedHarnesses, Now: time.Now(), FullRefresh: fullRefresh, SourceDir: sourceDir,
	}, fixtureCollector)
	select {
	case <-ctx.Done():
		return localprotocol.CollectionResult{}, ctx.Err()
	case result := <-done:
		summary := result.Result.Collection
		completion := localprotocol.CollectionResult{Accepted: result.Result.Accepted, Batches: result.Result.Batches,
			Pending: result.Result.Pending, PendingKnown: result.Result.PendingKnown,
			RawFacts: summary.RawFacts, Skipped: summary.Skipped, Quarantined: summary.Quarantined, Failed: summary.Failed}
		completion.AttemptID = result.AttemptID
		if result.Err != nil {
			completion.ErrorCode = "collection_failed"
		}
		return completion, nil
	}
}
