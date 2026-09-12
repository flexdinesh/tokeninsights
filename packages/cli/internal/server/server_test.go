package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
	_ "modernc.org/sqlite"
)

func fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	database, _, err := db.CreateIfMissing(path)
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
		_, err = database.Exec(`INSERT OR IGNORE INTO canonical_sessions (semantic_key,harness,session_id,first_seen_at_ms,last_seen_at_ms) VALUES (?,?,?,?,?)`, sessionKey, f.harness, f.session, day.UnixMilli(), day.UnixMilli())
		if err != nil {
			t.Fatal(err)
		}
		result, err := database.Exec(`INSERT INTO raw_token_usage (raw_fact_key,harness,source_id,source_kind,collector,parser,observed_at_ms,session_id,usage_scope,quality) VALUES (?,?,?,'test','test','test',?,?,'message','exact')`, key, f.harness, "test", day.UnixMilli(), f.session)
		if err != nil {
			t.Fatal(err)
		}
		rawID, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		_, err = database.Exec(`INSERT INTO canonical_token_usage (semantic_key,recorded_at_ms,harness,session_id,provider,model,usage_scope,quality,is_countable,input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,total_tokens,primary_raw_fact_id)
			VALUES (?,?,?,(SELECT id FROM canonical_sessions WHERE semantic_key=?),?,?,'message','exact',?,?,?,?,?,?,?)`, key, day.UnixMilli(), f.harness, sessionKey, f.provider, f.model, f.countable, f.input, f.output, f.cacheRead, f.cacheWrite, f.total, rawID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`INSERT INTO canonical_sessions (semantic_key,harness,session_id,first_seen_at_ms,last_seen_at_ms) VALUES ('empty','pi','empty',0,0)`); err != nil {
		t.Fatal(err)
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
	database, _, err := db.CreateIfMissing(path)
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
	a := newApp(context.Background(), Options{DBPath: fixture(t), Defaults: viewer.Selection{Period: "week", Bucket: "day", Providers: []string{}, Models: []string{}, Harnesses: []string{}, Sessions: []string{}}}, io.Discard)
	handler := a.handler()
	for _, query := range []string{"period=bad", "bucket=hour", "tab=tps", "from=2026-02-30", "from=2026-10-01&to=2026-09-01", "page=0", "pageSize=201", "direction=bad", "tab=context&sort=total", "sort=sql", "harness=bad"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/dashboard?"+query, nil))
		if w.Code != 400 {
			t.Errorf("%s: %d", query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/filters?from=2026-09-01&to=2026-09-30&model=absent", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var facets map[string][]string
	if err := json.Unmarshal(w.Body.Bytes(), &facets); err != nil {
		t.Fatal(err)
	}
	if len(facets["models"]) != 2 || len(facets["providers"]) != 0 {
		t.Fatalf("facets must ignore own selection only: %v", facets)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/filters?period=all&search=b", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &facets); err != nil {
		t.Fatal(err)
	}
	if len(facets["sessions"]) != 1 || facets["sessions"][0] != "b" {
		t.Fatalf("session search: %v", facets)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/bootstrap", nil))
	if !strings.Contains(w.Body.String(), `"period":"week"`) || strings.Contains(w.Body.String(), a.options.DBPath) {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "/assets/") {
		t.Fatalf("embedded app missing: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/missing", nil))
	if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("unknown API route must not return HTML")
	}
}

func TestSyncSharedAcrossClientsAndFailureRecovery(t *testing.T) {
	var log bytes.Buffer
	a := newApp(context.Background(), Options{Defaults: viewer.Selection{Harnesses: []string{"pi"}}}, &log)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	a.syncer = func(ctx context.Context, opts pipeline.SyncOptions) (pipeline.Summary, error) {
		calls.Add(1)
		if len(opts.Harnesses) != len(pipeline.SupportedHarnesses) || !opts.Normalize {
			t.Error("sync must normalize all harnesses")
		}
		opts.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
		close(started)
		<-release
		return pipeline.Summary{}, errors.New("failed at /private/source.jsonl")
	}
	a.startSync()
	<-started
	var requests sync.WaitGroup
	for range 10 {
		requests.Add(1)
		go func() {
			defer requests.Done()
			w := httptest.NewRecorder()
			a.handler().ServeHTTP(w, httptest.NewRequest("POST", "/api/sync", nil))
			if w.Code != http.StatusAccepted {
				t.Errorf("status %d", w.Code)
			}
		}()
	}
	requests.Wait()
	if calls.Load() != 1 || !a.status().Running || a.status().Harnesses["pi"] != "syncing" {
		t.Fatal("duplicate sync or missing progress")
	}
	close(release)
	a.jobs.Wait()
	status := a.status()
	if status.Running || status.Revision != 1 || status.Error == "" || strings.Contains(status.Error, "private") {
		t.Fatalf("status: %+v", status)
	}
	if !strings.Contains(log.String(), "/private/source.jsonl") {
		t.Fatal("terminal should retain diagnostic detail")
	}
	a.syncer = func(context.Context, pipeline.SyncOptions) (pipeline.Summary, error) { return pipeline.Summary{}, nil }
	a.startSync()
	a.jobs.Wait()
	if a.status().Error != "" || a.status().Revision != 2 {
		t.Fatal("retry must recover")
	}
}

func TestSyncRecoveryProgressAndRetry(t *testing.T) {
	for _, recoveryErr := range []error{db.ErrRebuildPending, db.ErrRecoveryRequired} {
		a := newApp(context.Background(), Options{}, io.Discard)
		a.syncer = func(ctx context.Context, opts pipeline.SyncOptions) (pipeline.Summary, error) {
			for _, phase := range []pipeline.SyncProgressStatus{pipeline.SyncProgressResetting, pipeline.SyncProgressRebuilding} {
				opts.Progress(pipeline.SyncProgressEvent{Status: phase})
				if status := a.status(); !status.Running || status.Phase != string(phase) {
					t.Errorf("missing recovery progress: %+v", status)
				}
			}
			return pipeline.Summary{}, fmt.Errorf("/private/source.jsonl: %w", recoveryErr)
		}
		a.startSync()
		a.jobs.Wait()
		status := a.status()
		if status.Running || status.Phase != "rebuild_failed" || !strings.Contains(status.Error, "Retry sync") || strings.Contains(status.Error, "inspect") || strings.Contains(status.Error, "/private") {
			t.Fatalf("unsafe recovery status: %+v", status)
		}
		a.syncer = func(context.Context, pipeline.SyncOptions) (pipeline.Summary, error) { return pipeline.Summary{}, nil }
		a.startSync()
		a.jobs.Wait()
		if status := a.status(); status.Phase != "ready" || status.Error != "" || status.Revision != 2 {
			t.Fatalf("retry did not recover: %+v", status)
		}
	}
}

func TestSyncKeepsRebuildingPhaseDuringHarnessProgress(t *testing.T) {
	a := newApp(context.Background(), Options{}, io.Discard)
	started, release := make(chan struct{}), make(chan struct{})
	a.syncer = func(ctx context.Context, opts pipeline.SyncOptions) (pipeline.Summary, error) {
		opts.Progress(pipeline.SyncProgressEvent{Status: pipeline.SyncProgressResetting})
		opts.Progress(pipeline.SyncProgressEvent{Status: pipeline.SyncProgressRebuilding})
		for _, event := range []pipeline.SyncProgressEvent{
			{Harness: pipeline.HarnessOpenCode, Status: pipeline.SyncProgressDiscovering},
			{Harness: pipeline.HarnessOpenCode, Status: pipeline.SyncProgressSkipped},
			{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressDiscovering},
			{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing},
			{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSynced},
			{Harness: pipeline.HarnessCodex, Status: pipeline.SyncProgressDiscovering},
			{Harness: pipeline.HarnessCodex, Status: pipeline.SyncProgressSyncing},
		} {
			opts.Progress(event)
			status := a.status()
			if status.Phase != "rebuilding" || status.Harnesses[string(event.Harness)] != string(event.Status) {
				t.Errorf("harness event lost rebuilding state: %+v", status)
			}
		}
		close(started)
		<-release
		opts.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessCodex, Status: pipeline.SyncProgressSynced})
		opts.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessClaudeCode, Status: pipeline.SyncProgressSkipped})
		opts.Progress(pipeline.SyncProgressEvent{Status: pipeline.SyncProgressNormalizing})
		if status := a.status(); status.Phase != "normalizing" {
			t.Errorf("global normalization did not advance phase: %+v", status)
		}
		return pipeline.Summary{}, nil
	}
	a.startSync()
	<-started
	status := a.status()
	if !status.Running || status.Phase != "rebuilding" || status.Harnesses["codex"] != "syncing" || status.Harnesses["pi"] != "synced" {
		t.Errorf("blocked rebuild status: %+v", status)
	}
	close(release)
	a.jobs.Wait()
	if status := a.status(); status.Running || status.Phase != "ready" {
		t.Fatalf("completed rebuild status: %+v", status)
	}
}

func TestAnalyticsRejectPendingRecoveryWithoutMutation(t *testing.T) {
	path := fixture(t)
	database, err := db.OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec("UPDATE database_lifecycle SET rebuild_pending = 1, rebuild_source_key = 'test-scope' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	a := newApp(context.Background(), Options{DBPath: path}, io.Discard)
	for _, endpoint := range []string{"/api/dashboard", "/api/filters"} {
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, httptest.NewRequest("GET", endpoint, nil))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "Sync to rebuild") || strings.Contains(w.Body.String(), path) {
			t.Fatalf("%s: %d %s", endpoint, w.Code, w.Body.String())
		}
	}
	var pending, facts int
	if err := database.QueryRow("SELECT rebuild_pending, (SELECT COUNT(*) FROM canonical_token_usage) FROM database_lifecycle WHERE id = 1").Scan(&pending, &facts); err != nil {
		t.Fatal(err)
	}
	if pending != 1 || facts != 6 {
		t.Fatalf("read endpoints changed data: pending=%d facts=%d", pending, facts)
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
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if err := Run(ctx, Options{DBPath: fixture(t), NoSync: true, Host: "127.0.0.1", Port: 0}, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "http://127.0.0.1:") {
		t.Fatalf("expected a single bound URL: %s", output.String())
	}
	if err := Run(context.Background(), Options{DBPath: filepath.Join(t.TempDir(), "missing.sqlite"), NoSync: true}, io.Discard, io.Discard); err == nil {
		t.Fatal("--no-sync must reject a missing database")
	}
}
