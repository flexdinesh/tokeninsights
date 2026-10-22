package cli

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
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
	return server.NewDirectQuery(sqlanalytics.Queries{Store: store}, policy, instance, "")
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

func (r *observedReader) Instance(ctx context.Context) (api.InstanceResponseV2, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return api.InstanceResponseV2{}, err
	}
	return reader.Instance(ctx)
}

func (r *observedReader) Usage(ctx context.Context, params api.GetUsageParams) (api.UsageResponseV2, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return api.UsageResponseV2{}, err
	}
	return reader.Usage(ctx, params)
}

func (r *observedReader) AllUsage(ctx context.Context, params api.GetUsageParams, limit int) (api.UsageResponseV2, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return api.UsageResponseV2{}, err
	}
	return reader.AllUsage(ctx, params, limit)
}

func (r *observedReader) Facets(ctx context.Context, params api.GetUsageFacetsParams) (api.UsageFacetsResponseV2, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return api.UsageFacetsResponseV2{}, err
	}
	return reader.Facets(ctx, params)
}

func (r *observedReader) Status(ctx context.Context) (api.StatusResponseV2, error) {
	reader, err := r.current(ctx)
	if err != nil {
		return api.StatusResponseV2{}, err
	}
	return reader.Status(ctx)
}

// staticReader returns fixed snapshots and honors cancellation like a backend.
type staticReader struct {
	instance api.InstanceResponseV2
	usage    api.UsageResponseV2
}

func (r staticReader) Instance(ctx context.Context) (api.InstanceResponseV2, error) {
	return r.instance, ctx.Err()
}

func (r staticReader) Usage(ctx context.Context, _ api.GetUsageParams) (api.UsageResponseV2, error) {
	return r.usage, ctx.Err()
}

func (r staticReader) AllUsage(ctx context.Context, _ api.GetUsageParams, _ int) (api.UsageResponseV2, error) {
	return r.usage, ctx.Err()
}

func (r staticReader) Facets(ctx context.Context, _ api.GetUsageFacetsParams) (api.UsageFacetsResponseV2, error) {
	return api.UsageFacetsResponseV2{}, ctx.Err()
}

func (r staticReader) Status(ctx context.Context) (api.StatusResponseV2, error) {
	return api.StatusResponseV2{}, ctx.Err()
}
