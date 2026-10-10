package queryclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/duckdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	_ "modernc.org/sqlite"
)

func pointer[T any](value T) *T { return &value }

func newClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c, err := New(s.URL, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRealServerPaginationAndComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.duckdb")
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	database := store.SQL()
	const sessions = 225
	for i := 0; i < sessions; i++ {
		key := fmt.Sprintf("synthetic-%03d", i)
		if _, err := database.Exec("INSERT INTO analytics.facts VALUES ('default',?,'fixture','pi',?,?,'','',1000,'synthetic-provider','explicit','synthetic-model','message','exact',true,10,2,3,4,5,24,'','','','','','{}',1,0)", key, key, key); err != nil {
			t.Fatal(err)
		}

	}
	c := newClient(t, server.NewDataHandler(context.Background(), duckdb.Source{Store: store}, io.Discard, "127.0.0.1", "", false))
	instance, err := c.Instance(t.Context())
	if err != nil || instance.ApiVersion != api.V2 {
		t.Fatalf("instance = %#v, %v", instance, err)
	}
	params := api.GetUsageParams{Period: pointer(api.Period("all")), Tab: pointer(api.UsageTab("sessions")), Page: pointer(7), PageSize: pointer(1)}
	result, err := c.AllUsage(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != sessions || len(result.Rows) != sessions || result.Page != 1 || result.PageSize != pageSize {
		t.Fatalf("pagination = %d/%d page%d size%d", len(result.Rows), result.RowCount, result.Page, result.PageSize)
	}
	wantSummary := api.UsageSummary{Input: 10 * sessions, Output: 2 * sessions, Reasoning: 3 * sessions, CacheRead: 4 * sessions, CacheWrite: 5 * sessions, Total: 24 * sessions, Sessions: sessions, SyncedSessions: sessions}
	if result.Summary != wantSummary {
		t.Fatalf("summary = %#v, want %#v", result.Summary, wantSummary)
	}
	for _, row := range result.Rows {
		if row.Input != 10 || row.Output != 2 || row.Reasoning != 3 || row.CacheRead != 4 || row.CacheWrite != 5 || row.Total != 24 || row.Context != 19 {
			t.Fatalf("components = %#v", row)
		}
	}
	params.Tab = pointer(api.UsageTab("context"))
	contextRows, err := c.AllUsage(t.Context(), params)
	if err != nil || len(contextRows.Rows) != 1 {
		t.Fatalf("context rows = %#v, %v", contextRows.Rows, err)
	}
	row := contextRows.Rows[0]
	if row.AverageContext != 19 || row.MedianContext != 19 || row.MaxContext != 19 || row.Sessions != sessions {
		t.Fatalf("context metrics = %#v", row)
	}
	facets, err := c.Facets(t.Context(), api.GetUsageFacetsParams{Period: pointer(api.Period("all")), Search: pointer("synthetic-224")})
	if err != nil || !reflect.DeepEqual(facets.Sessions, []string{"synthetic-224"}) || !reflect.DeepEqual(facets.Models, []string{"synthetic-model"}) {
		t.Fatalf("facets = %#v, %v", facets, err)
	}
}

func serveJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func instanceResponse(identity string) api.InstanceResponseV2 {
	return api.InstanceResponseV2{DatasetId: "default", ServerKind: "personal", Capabilities: []string{"usage", "facets"}, Permissions: []api.InstanceResponseV2Permissions{"read"}, ApiVersion: api.V2, InstanceId: identity, DataEpoch: "epoch", DataReadiness: api.InstanceResponseV2DataReadinessReady}
}

func usagePage(page int, revision int64, identity string) api.UsageResponseV2 {
	response := api.UsageResponseV2{DatasetId: "default", Page: page, PageSize: pageSize, RowCount: pageSize + 1, Revision: revision, InstanceId: identity, DataEpoch: "epoch", Summary: api.UsageSummary{Total: 12345}}
	for i := (page - 1) * pageSize; i < min(page*pageSize, pageSize+1); i++ {
		response.Rows = append(response.Rows, api.UsageRow{Key: strconv.Itoa(i), Total: int64(i)})
	}
	return response
}

func TestAllUsageRetriesWholeSnapshot(t *testing.T) {
	for _, changed := range []string{"revision", "instance", "epoch", "dataset"} {
		t.Run(changed, func(t *testing.T) {
			calls := 0
			c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/instance" {
					serveJSON(w, instanceResponse("stable"))
					return
				}
				calls++
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				response := usagePage(page, 1, "stable")
				if calls == 2 {
					switch changed {
					case "revision":
						response.Revision = 2
					case "instance":
						response.InstanceId = "restarted"
					case "epoch":
						response.DataEpoch = "reset"
					case "dataset":
						response.DatasetId = "other-user"
					}
				}
				serveJSON(w, response)
			}))
			response, err := c.AllUsage(t.Context(), api.GetUsageParams{})
			if err != nil || calls != 4 || len(response.Rows) != pageSize+1 || response.Summary.Total != 12345 {
				t.Fatalf("snapshot rows%d calls%d err%v", len(response.Rows), calls, err)
			}
		})
	}
}

func TestAllUsageChurnIsBounded(t *testing.T) {
	calls := 0
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/instance" {
			serveJSON(w, instanceResponse("stable"))
			return
		}
		calls++
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		serveJSON(w, usagePage(page, int64(calls), "stable"))
	}))
	response, err := c.AllUsage(t.Context(), api.GetUsageParams{})
	if !errors.Is(err, ErrSnapshotChanged) || calls != snapshotTries*2 || len(response.Rows) != 0 {
		t.Fatalf("churn rows%d calls%d err%v", len(response.Rows), calls, err)
	}
}

func TestAllUsageRejectsResetAfterFinalPage(t *testing.T) {
	instanceCalls := 0
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/instance" {
			instanceCalls++
			identity := "stable"
			if instanceCalls%2 == 0 {
				identity = "restarted"
			}
			serveJSON(w, instanceResponse(identity))
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		serveJSON(w, usagePage(page, 1, "stable"))
	}))
	response, err := c.AllUsage(t.Context(), api.GetUsageParams{})
	if !errors.Is(err, ErrSnapshotChanged) || instanceCalls != snapshotTries*2 || len(response.Rows) != 0 {
		t.Fatalf("reset rows%d instanceCalls%d err%v", len(response.Rows), instanceCalls, err)
	}
}

func TestAllUsageRejectsInvalidPagination(t *testing.T) {
	for _, invalid := range []string{"empty-page", "duplicate-key", "missing-key", "page", "size", "count-limit", "revision"} {
		t.Run(invalid, func(t *testing.T) {
			c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/instance" {
					serveJSON(w, instanceResponse("stable"))
					return
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				response := usagePage(page, 1, "stable")
				switch invalid {
				case "empty-page":
					response.Rows = nil
				case "duplicate-key":
					if page == 2 {
						response.Rows[0].Key = "0"
					}
				case "missing-key":
					response.Rows[0].Key = ""
				case "page":
					response.Page = 9
				case "size":
					response.PageSize = 1
				case "count-limit":
					response.RowCount = maxRows + 1
				case "revision":
					response.Revision = -1
				}
				serveJSON(w, response)
			}))
			response, err := c.AllUsage(t.Context(), api.GetUsageParams{})
			if err == nil || len(response.Rows) != 0 {
				t.Fatalf("invalid snapshot published: %d rows, %v", len(response.Rows), err)
			}
		})
	}
}

func TestAllUsageCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/instance" {
			serveJSON(w, instanceResponse("stable"))
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 2 {
			cancel()
			<-r.Context().Done()
			return
		}
		serveJSON(w, usagePage(page, 1, "stable"))
	}))
	response, err := c.AllUsage(ctx, api.GetUsageParams{})
	if !errors.Is(err, context.Canceled) || len(response.Rows) != 0 {
		t.Fatalf("cancellation = %v, rows%d", err, len(response.Rows))
	}
}

func TestQueryEncodingPreservesRepeatedFilters(t *testing.T) {
	params := api.GetUsageParams{Period: pointer(api.Period("all")), Bucket: pointer(api.Bucket("week")), From: pointer("2026-01-01"), To: pointer("2026-02-01"), Provider: pointer([]string{"a+b", "a&b"}), Model: pointer([]string{"m/1", "m 2"}), Harness: pointer([]api.Harness{"pi", "codex"}), Session: pointer([]string{"s?1", "s#2"}), Repository: pointer([]string{"repo"}), Directory: pointer([]string{"dir"}), Tab: pointer(api.UsageTab("repo")), LocationGroup: pointer(api.LocationGroup("directory")), Sort: pointer(api.SortField("input")), Direction: pointer(api.SortDirection("asc")), Page: pointer(2), PageSize: pointer(1)}
	want := url.Values{"period": {"all"}, "bucket": {"week"}, "from": {"2026-01-01"}, "to": {"2026-02-01"}, "provider": {"a+b", "a&b"}, "model": {"m/1", "m 2"}, "harness": {"pi", "codex"}, "session": {"s?1", "s#2"}, "repository": {"repo"}, "directory": {"dir"}, "tab": {"repo"}, "locationGroup": {"directory"}, "sort": {"input"}, "direction": {"asc"}, "page": {"2"}, "pageSize": {"1"}}
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/usage" || !reflect.DeepEqual(r.URL.Query(), want) {
			t.Errorf("unexpected query %s %s", r.Method, r.URL)
		}
		serveJSON(w, api.UsageResponseV2{DatasetId: "default", InstanceId: "stable", DataEpoch: "epoch", Revision: 0})
	}))
	if _, err := c.Usage(t.Context(), params); err != nil {
		t.Fatal(err)
	}
}

func TestURLValidation(t *testing.T) {
	for _, raw := range []string{"", "/relative", "ftp://example.test", "http://user:secret@example.test", "http://example.test?secret=value", "http://example.test#secret", "http://example.test:65536", "http://example.test:bad"} {
		if _, err := New(raw, nil); err == nil {
			t.Errorf("accepted invalid URL %q", raw)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("error leaked URL credentials: %v", err)
		}
	}
}

func TestReadOnlyStatusAndBearerClientCopy(t *testing.T) {
	requests := make(chan string, 3)
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("query mutated server: %s", r.Method)
		}
		requests <- r.Header.Get("Authorization")
		if r.URL.Path == "/api/v2/status" {
			serveJSON(w, api.StatusResponseV2{DatasetId: "default", Revision: 7, InstanceId: "stable", DataEpoch: "epoch", DataReadiness: api.StatusResponseV2DataReadinessReady})
			return
		}
		serveJSON(w, instanceResponse("stable"))
	}))
	authenticated := c.WithToken("synthetic-token")
	status, err := authenticated.Status(t.Context())
	if err != nil || status.Revision != 7 {
		t.Fatalf("status=%#v error=%v", status, err)
	}
	if _, err := authenticated.Instance(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Instance(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Bearer synthetic-token", "Bearer synthetic-token", ""} {
		if got := <-requests; got != want {
			t.Fatalf("authorization=%q want%q", got, want)
		}
	}
}

func TestResponseBoundsAndSafeErrors(t *testing.T) {
	for _, invalid := range []string{"status", "redirect", "type", "json", "trailing", "oversized", "version"} {
		t.Run(invalid, func(t *testing.T) {
			c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch invalid {
				case "status":
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"message":"synthetic-secret"}`)
				case "redirect":
					w.Header().Set("Location", "/synthetic-secret")
					w.WriteHeader(http.StatusFound)
				case "type":
					w.Header().Set("Content-Type", "text/html")
				case "json":
					_, _ = io.WriteString(w, "synthetic-secret")
				case "trailing":
					_, _ = io.WriteString(w, `{"apiVersion":"v1"} {"synthetic-secret":true}`)
				case "oversized":
					_, _ = io.WriteString(w, strings.Repeat(" ", maxBodyBytes+1))
				case "version":
					response := instanceResponse("stable")
					response.ApiVersion = "v999"
					serveJSON(w, response)
				}
			}))
			_, err := c.Instance(t.Context())
			if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatalf("unsafe or missing error: %v", err)
			}
		})
	}
}
