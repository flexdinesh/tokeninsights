package tui

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// queryClient uses the production in-process reader, the same path the local
// terminal dashboard composes, against canonical-only storage.
func queryClient(t *testing.T, path string) *queryclient.Client {
	t.Helper()
	return queryclient.NewDirect(queryReader(t, path, fixtureInstance))
}

func queryReader(t *testing.T, path, instance string) queryclient.Reader {
	t.Helper()
	value, ok := queryStores.Load(path)
	if !ok {
		t.Fatal("missing fixture store", path)
	}
	store, ok := value.(*datastore.Store)
	if !ok {
		t.Fatal("invalid fixture store")
	}
	return storeReader(t, store, instance)
}

// Production composition always supplies a non-empty instance identity.
const fixtureInstance = "fixture-instance"

func storeReader(t *testing.T, store *datastore.Store, instance string) queryclient.Reader {
	t.Helper()
	policy, err := serverfeatures.NewLocalViewer(false)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewQueries(sqlanalytics.Queries{Store: store}, policy, instance, "")
}

var queryStores sync.Map

// observedReader counts viewer reads, can swap the published reader and can
// intercept a read to simulate failure or a blocked backend.
type observedReader struct {
	mu        sync.Mutex
	reader    queryclient.Reader
	reads     atomic.Int64
	intercept func(context.Context) error
}

func (r *observedReader) set(reader queryclient.Reader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reader = reader
}

func (r *observedReader) current(ctx context.Context) (queryclient.Reader, error) {
	r.reads.Add(1)
	if r.intercept != nil {
		if err := r.intercept(ctx); err != nil {
			return nil, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reader, nil
}

func (r *observedReader) Instance(ctx context.Context) (application.Instance, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return application.Instance{}, err
	}
	return reader.Instance(ctx)
}

func (r *observedReader) Usage(ctx context.Context, params analytics.Query) (application.Usage, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return application.Usage{}, err
	}
	return reader.Usage(ctx, params)
}

func (r *observedReader) AllUsage(ctx context.Context, params analytics.Query, limit int) (application.Usage, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return application.Usage{}, err
	}
	return reader.AllUsage(ctx, params, limit)
}

func (r *observedReader) Facets(ctx context.Context, params application.FacetRequest) (application.Facets, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return application.Facets{}, err
	}
	return reader.Facets(ctx, params)
}

func (r *observedReader) Status(ctx context.Context) (application.Status, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return application.Status{}, err
	}
	return reader.Status(ctx)
}

// staticReader returns fixed snapshots and honors cancellation like a backend.
type staticReader struct {
	instance application.Instance
	usage    application.Usage
}

func (r staticReader) Instance(ctx context.Context) (application.Instance, error) {
	return r.instance, ctx.Err()
}

func (r staticReader) Usage(ctx context.Context, _ analytics.Query) (application.Usage, error) {
	return r.usage, ctx.Err()
}

func (r staticReader) AllUsage(ctx context.Context, _ analytics.Query, _ int) (application.Usage, error) {
	return r.usage, ctx.Err()
}

func (r staticReader) Facets(ctx context.Context, _ application.FacetRequest) (application.Facets, error) {
	return application.Facets{}, ctx.Err()
}

func (r staticReader) Status(ctx context.Context) (application.Status, error) {
	return application.Status{}, ctx.Err()
}
