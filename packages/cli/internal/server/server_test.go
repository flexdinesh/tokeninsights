package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
	_ "modernc.org/sqlite"
)

func fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	store, err := datastore.Open(t.Context(), path)
	database := store.SQL()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	facts := []struct {
		day, harness, session, provider, model                 string
		input, output, cacheRead, cacheWrite, total, countable int64
	}{
		{"2026-09-01", "pi", "a", "openai", "model-a", 100, 10, 20, 5, 135, 1},
		{"2026-09-02", "pi", "a", "openai", "model-a", 200, 20, 20, 5, 245, 1},
		{"2026-09-02", "pi", "b", "openai", "model-a", 50, 5, 0, 0, 55, 1},
		{"2026-09-02", "claude-code", "a", "maybe-anthropic", "unknown", 10, 1, 5, 2, 18, 1},
		{"2026-08-01", "pi", "old", "openai", "model-a", 9000, 0, 0, 0, 9000, 1},
		{"2026-09-02", "pi", "suppressed", "openai", "model-a", 8000, 0, 0, 0, 8000, 0},
	}
	for i, f := range facts {
		day, err := time.ParseInLocation(time.DateOnly, f.day, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		key := fmt.Sprintf("fact-%d", i)
		sessionKey := f.harness + ":" + f.session
		_, err = database.Exec("INSERT INTO analytics_facts VALUES ('default',?,'fixture',?,?,?,'','',?,?,'explicit',?,'message','exact',?,?,?,0,?,?,?,'','','','','','{}',1,0)", key, f.harness, sessionKey, f.session, day.UnixMilli(), f.provider, f.model, f.countable != 0, f.input, f.output, f.cacheRead, f.cacheWrite, f.total)
		if err != nil {
			t.Fatal(err)
		}

	}
	return path
}

func TestDashboardCanonicalParityAndPagination(t *testing.T) {
	path := fixture(t)
	for tab, wantRows := range map[string]int{"tokens": 2, "models": 2, "providers": 2, "harnesses": 2, "sessions": 3, "context": 2} {
		t.Run(tab, func(t *testing.T) {
			q, err := parseQuery(url.Values{"tab": {tab}, "from": {"2026-09-01"}, "to": {"2026-09-30"}, "pageSize": {"1"}, "page": {"999"}})
			if err != nil {
				t.Fatal(err)
			}
			data, err := loadDashboard(context.Background(), path, q, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if data.RowCount != wantRows || data.Page != wantRows || len(data.Rows) != 1 {
				t.Fatalf("pagination: %+v", data)
			}
			if data.Summary.TotalTokens != 453 || data.Summary.SessionCount != 3 || data.Summary.InputTokens != 360 || data.Summary.OutputTokens != 36 || data.Summary.CacheReadTokens != 45 || data.Summary.CacheWriteTokens != 12 {
				t.Fatalf("summary: %+v", data.Summary)
			}
			if data.Summary.SyncedSessions != 4 {
				t.Fatalf("synced count must include out-of-range sessions: %+v", data.Summary)
			}
			if data.LastSynced != 0 {
				t.Fatal("empty sync history should be never")
			}
			if tab == "context" {
				peak := data.Chart[0]
				if peak.Sessions != 2 || peak.AverageContext != 137 || peak.MedianContext != 137 || peak.MaxContext != 225 {
					t.Fatalf("context: %+v", peak)
				}
			}
			if tab == "tokens" && data.Chart[0].Name != "2026-09-01" {
				t.Fatal("chart must stay chronological independently of table sort/page")
			}
		})
	}
}

func TestDashboardCombinedFiltersAndEmptyState(t *testing.T) {
	path := fixture(t)
	q, err := parseQuery(url.Values{"tab": {"sessions"}, "period": {"all"}, "from": {"2026-09-02"}, "to": {"2026-09-02"}, "provider": {"openai"}, "model": {"model-a"}, "harness": {"pi"}, "session": {"a"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := loadDashboard(context.Background(), path, q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Rows) != 1 || data.Rows[0].Context != 225 || data.Summary.TotalTokens != 245 {
		t.Fatalf("filtered: %+v", data)
	}
	if data.Summary.SessionCount != 1 || data.Summary.SyncedSessions != 4 {
		t.Fatalf("combined filters must only narrow shown sessions: %+v", data.Summary)
	}
	q.Selection.Models = []string{"absent"}
	data, err = loadDashboard(context.Background(), path, q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if data.Rows == nil || data.Chart == nil || len(data.Rows) != 0 || data.Summary.TotalTokens != 0 || data.Page != 1 {
		t.Fatalf("empty: %+v", data)
	}
	if data.Summary.SessionCount != 0 || data.Summary.SyncedSessions != 4 {
		t.Fatalf("empty filtered results must retain synced count: %+v", data.Summary)
	}
}

func TestRepoUnknownAndDirectoryGrouping(t *testing.T) {
	path := fixture(t)
	q, err := parseQuery(url.Values{"tab": {"repo"}, "from": {"2026-09-01"}, "to": {"2026-09-30"}, "repository": {"unknown"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := loadDashboard(context.Background(), path, q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Rows) != 1 || data.Rows[0].LocationKey != "unknown" || data.Rows[0].Total != 453 || data.Summary.TotalTokens != 453 || len(data.Rows[0].DirectoryNames) != 0 || !data.Rows[0].HasUnknownDirectory {
		t.Fatalf("unknown repo attribution: %+v", data)
	}
	transport := apiUsageRows(data.Rows)
	if len(transport) != 1 || transport[0].DirectoryNames == nil || !transport[0].HasUnknownDirectory {
		t.Fatalf("unknown directory transport: %+v", transport)
	}
	q.LocationGroup = querymodel.RepoGroupDirectory
	data, err = loadDashboard(context.Background(), path, q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Rows) != 1 || data.Rows[0].LocationKey != "unknown" || data.Summary.TotalTokens != 453 {
		t.Fatalf("directory grouping: %+v", data)
	}
}

func TestDashboardSessionCoverageAcrossBucketsAndDates(t *testing.T) {
	path := fixture(t)
	for _, bucket := range []string{"day", "week", "month", "year"} {
		for _, test := range []struct {
			name    string
			filters url.Values
			shown   int64
		}{
			{"all time", url.Values{}, 4},
			{"harness", url.Values{"harness": {"pi"}}, 3},
			{"shared source ID across harnesses", url.Values{"session": {"a"}}, 2},
			{"provider", url.Values{"provider": {"maybe-anthropic"}}, 1},
			{"model", url.Values{"model": {"unknown"}}, 1},
			{"custom dates", url.Values{"from": {"2026-09-01"}, "to": {"2026-09-30"}}, 3},
		} {
			t.Run(bucket+"/"+test.name, func(t *testing.T) {
				values := test.filters
				values.Set("period", "all")
				values.Set("bucket", bucket)
				q, err := parseQuery(values)
				if err != nil {
					t.Fatal(err)
				}
				data, err := loadDashboard(context.Background(), path, q, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if data.Summary.SessionCount != test.shown || data.Summary.SyncedSessions != 4 {
					t.Fatalf("coverage: %+v", data.Summary)
				}
			})
		}
	}
	path = filepath.Join(t.TempDir(), "empty.sqlite")
	store, err := datastore.Open(t.Context(), path)
	database := store.SQL()
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	q, err := parseQuery(url.Values{"period": {"all"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := loadDashboard(context.Background(), path, q, time.Now())
	if err != nil || data.Summary.SessionCount != 0 || data.Summary.SyncedSessions != 0 {
		t.Fatalf("empty database: %+v, %v", data.Summary, err)
	}
}

func TestAPIValidationFacetsAndAssets(t *testing.T) {
	a := fixtureApp(t, fixture(t), Options{Defaults: viewer.Selection{Period: "week", Bucket: "day", Providers: []string{}, Models: []string{}, Harnesses: []string{}, Sessions: []string{}}})
	handler := a.handler()
	for _, query := range []string{"period=bad", "bucket=hour", "tab=invalid", "from=2026-02-30", "from=2026-10-01&to=2026-09-01", "page=0", "pageSize=201", "direction=bad", "tab=context&sort=total", "sort=sql", "harness=bad", "tab=repo&locationGroup=bad", "tab=repo&breakdown=provider", "tab=repo&worktree=old", "tab=repo&branch=main", "repository=unknown"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v2/usage?"+query, nil))
		var responseError serverapi.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &responseError); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if w.Code != http.StatusBadRequest || responseError.Code != serverapi.ErrorCodeInvalidRequest || responseError.Message == "" {
			t.Errorf("%s: %d %+v", query, w.Code, responseError)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v2/usage/facets?from=2026-09-01&to=2026-09-30&model=absent", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var facets serverapi.UsageFacetsResponseV2
	if err := json.Unmarshal(w.Body.Bytes(), &facets); err != nil {
		t.Fatal(err)
	}
	if len(facets.Models) != 2 || len(facets.Providers) != 0 {
		t.Fatalf("facets must ignore own selection only: %v", facets)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v2/usage/facets?tab=repo&period=all", nil))
	var locationFacets serverapi.UsageFacetsResponseV2
	if err := json.Unmarshal(w.Body.Bytes(), &locationFacets); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(locationFacets.Repositories) != 1 || locationFacets.Repositories[0].Key != "unknown" || locationFacets.Repositories[0].Name != "unknown" {
		t.Fatalf("unknown repository facet: %d %+v", w.Code, locationFacets)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v2/usage/facets?period=all&search=b", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &facets); err != nil {
		t.Fatal(err)
	}
	if len(facets.Sessions) != 1 || facets.Sessions[0] != "b" {
		t.Fatalf("session search: %v", facets)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v2/instance", nil))
	var instance serverapi.InstanceResponseV2
	if err := json.Unmarshal(w.Body.Bytes(), &instance); err != nil {
		t.Fatal(err)
	}
	if instance.ApiVersion != "v2" || instance.ServerVersion != version.Version || instance.Defaults.Period != serverapi.PeriodWeek || len(instance.Capabilities) == 0 {
		t.Fatalf("instance: %+v", instance)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "/assets/") {
		t.Fatalf("embedded app missing: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/missing", nil))
	var responseError serverapi.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &responseError); err != nil {
		t.Fatal(err)
	}
	if w.Code != 404 || responseError.Code != serverapi.ErrorCodeNotFound || responseError.Message == "" || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("unknown API route must not return HTML")
	}
}

func TestDashboardRoutesServeEmbeddedApp(t *testing.T) {
	handler := newApp(context.Background(), Options{}, io.Discard).handler()

	for _, path := range []string{"/tokens", "/models", "/providers", "/harnesses", "/sessions", "/context", "/repo"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+"?period=all", nil))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "/assets/") {
				t.Fatalf("embedded app missing: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestListenerURLAndShutdown(t *testing.T) {
	listener, err := listen("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" || port == "0" {
		t.Fatalf("must bind only the selected address and actual port: %s", listener.Addr())
	}
	conflict, err := net.Listen("tcp4", listener.Addr().String())
	if err == nil {
		_ = conflict.Close()
		t.Fatal("expected occupied port")
	}
	_ = listener.Close()
}

func loadDashboard(ctx context.Context, path string, q query, now time.Time) (dashboard, error) {
	store, err := datastore.Open(ctx, path)
	if err != nil {
		return dashboard{}, err
	}
	defer func() { _ = store.Close() }()
	return sqlanalytics.LoadDashboard(ctx, store, q, now)
}
func fixtureApp(t *testing.T, path string, options Options) *app {
	t.Helper()
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a := newApp(t.Context(), options, io.Discard)
	a.data = store
	a.queries = sqlanalytics.Queries{Store: store}
	return a
}
