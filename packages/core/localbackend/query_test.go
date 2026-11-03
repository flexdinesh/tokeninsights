package localbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"path/filepath"
)

type countedRepository struct {
	analytics.Repository
	all, pages int
	limit      int
}

func (r *countedRepository) AllDashboard(ctx context.Context, q analytics.Query, now time.Time, maxRows int) (analytics.Dashboard, error) {
	r.all++
	r.limit = maxRows
	return r.Repository.AllDashboard(ctx, q, now, maxRows)
}

func (r *countedRepository) Dashboard(ctx context.Context, q analytics.Query, now time.Time) (analytics.Dashboard, error) {
	r.pages++
	return r.Repository.Dashboard(ctx, q, now)
}

func TestPrivateAllUsageCallsOneRealRepositorySnapshot(t *testing.T) {
	root := t.TempDir()
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	source := &countedRepository{Repository: sqlanalytics.Queries{Store: runtime.Store}}
	queries := application.NewQueries(source, runtime.Policy, runtime.InstanceID, runtime.Hostname)
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(localprotocol.AllUsageRequest{Query: analytics.Query{Selection: analytics.Selection{Period: "all"}, Page: 1, PageSize: 200}, MaxRows: localprotocol.MaxQueryRows}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	queryHandler(queries).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/local/v1/queries/all", &body))
	if response.Code != http.StatusOK || source.all != 1 || source.pages != 0 || source.limit != 100000 {
		t.Fatal("private read paginated or reduced row limit", response.Code, source.all, source.pages, source.limit)
	}
	// The same application validator still enforces the public page limit.
	body.Reset()
	_ = json.NewEncoder(&body).Encode(analytics.Query{PageSize: 201})
	response = httptest.NewRecorder()
	queryHandler(queries).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/local/v1/queries/usage", &body))
	if response.Code != http.StatusBadRequest || source.pages != 0 {
		t.Fatal("normal page bound changed", response.Code, source.pages)
	}
}
