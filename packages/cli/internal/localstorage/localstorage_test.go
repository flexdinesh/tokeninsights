package localstorage_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectordb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localstorage"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecompat"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
)

const visibilityTimeout = 30 * time.Second

type paths struct{ root, collector, server, app, source string }

func newPaths(t *testing.T) paths {
	root := t.TempDir()
	return paths{root: root, collector: filepath.Join(root, "collector.sqlite"), server: filepath.Join(root, "server.sqlite"), app: filepath.Join(root, "app.sqlite"), source: filepath.Join(root, "source")}
}

// copySources materializes the collector-rebuild harness logs, including OpenCode SQLite.
func copySources(t *testing.T, target string) {
	t.Helper()
	if err := os.CopyFS(target, os.DirFS(filepath.Join("..", "..", "testdata", "conformance", "collector-rebuild", "source"))); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(target, "opencode", "source.sql"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := sql.Open("sqlite", filepath.Join(target, "opencode", "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
}

type usage struct {
	Summary api.UsageSummary
	Facts   api.Count
}

// capture runs real local collection and returns all-time usage once visible.
func capture(t *testing.T, p paths) usage {
	t.Helper()
	runtime, err := localruntime.OpenWithApp(t.Context(), p.collector, p.server, p.app)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if _, err := collector.Run(t.Context(), collector.Options{CollectorDBPath: p.collector, ServerDBPath: p.server, Destination: runtime.Destination,
		SyncOptions: pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, SourceDir: p.source, Now: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), visibilityTimeout)
	defer cancel()
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	period := api.PeriodFilter("all")
	result, err := runtime.Query.AllUsage(ctx, api.GetUsageParams{Period: &period})
	if err != nil || result.FactCount == nil {
		t.Fatal(result, err)
	}
	return usage{Summary: result.Summary, Facts: *result.FactCount}
}

func setPragma(t *testing.T, path, statement string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func status(t *testing.T, report localstorage.Report, role localstorage.Role) localstorage.Store {
	t.Helper()
	for _, store := range report.Stores {
		if store.Role == role {
			return store
		}
	}
	t.Fatalf("role %d missing", role)
	return localstorage.Store{}
}

func TestInspectClassifiesEveryLocalRole(t *testing.T) {
	type role struct {
		name    string
		path    func(paths) string
		version int
		inspect func(context.Context, paths) (storagecompat.Result, error)
	}
	roles := []role{
		{"collector", func(p paths) string { return p.collector }, collectordb.SupportedSchemaVersion, func(ctx context.Context, p paths) (storagecompat.Result, error) {
			return collectordb.InspectCompatibility(ctx, p.collector)
		}},
		{"server", func(p paths) string { return p.server }, datastore.SchemaVersion, func(ctx context.Context, p paths) (storagecompat.Result, error) {
			return datastore.InspectCompatibility(ctx, p.server)
		}},
		{"app", func(p paths) string { return p.app }, appstore.SchemaVersion, func(ctx context.Context, p paths) (storagecompat.Result, error) {
			return appstore.InspectCompatibility(ctx, p.app, datastore.KindPersonal)
		}},
		{"jobs", func(p paths) string { return p.collector + ".jobs.sqlite" }, syncjob.SchemaVersion, func(ctx context.Context, p paths) (storagecompat.Result, error) {
			_, result, err := syncjob.InspectCompatibility(ctx, p.collector)
			return result, err
		}},
	}
	cases := []struct {
		name   string
		mutate func(version int) string
		want   storagecompat.Status
	}{
		{"compatible", func(int) string { return "" }, storagecompat.Compatible},
		{"older", func(version int) string { return "PRAGMA user_version=" + strconv.Itoa(version-1) }, storagecompat.Older},
		{"newer", func(version int) string { return "PRAGMA user_version=" + strconv.Itoa(version+1) }, storagecompat.Newer},
		{"foreign", func(int) string { return "PRAGMA application_id=0" }, storagecompat.Foreign},
		{"mismatch", func(int) string { return "CREATE TABLE unexpected(id INTEGER)" }, storagecompat.Mismatch},
	}
	for _, role := range roles {
		t.Run(role.name+"/missing", func(t *testing.T) {
			result, err := role.inspect(t.Context(), newPaths(t))
			if err != nil || result.Status != storagecompat.Missing {
				t.Fatal(result, err)
			}
		})
		for _, test := range cases {
			t.Run(role.name+"/"+test.name, func(t *testing.T) {
				p := newPaths(t)
				createStores(t, p)
				if statement := test.mutate(role.version); statement != "" {
					setPragma(t, role.path(p), statement)
				}
				before := snapshot(t, p.root)
				result, err := role.inspect(t.Context(), p)
				if err != nil || result.Status != test.want || result.Supported != role.version {
					t.Fatalf("got %+v %v, want %v", result, err, test.want)
				}
				if after := snapshot(t, p.root); !reflect.DeepEqual(before, after) {
					t.Fatal("inspection changed files")
				}
			})
		}
	}
}

func createStores(t *testing.T, p paths) {
	t.Helper()
	runtime, err := localruntime.OpenWithApp(t.Context(), p.collector, p.server, p.app)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	database, _, err := collectordb.CreateIfMissing(p.collector)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	jobs, err := syncjob.Open(t.Context(), p.collector)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Close(); err != nil {
		t.Fatal(err)
	}
}

// snapshot records file names and contents, excluding lock sidecars, the
// SQLite WAL index and empty WAL files, which any WAL reader may create.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) == ".lock" || strings.HasSuffix(entry.Name(), "-shm") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if len(body) == 0 && strings.HasSuffix(entry.Name(), "-wal") {
			continue
		}
		files[entry.Name()] = string(body)
	}
	return files
}

func TestServerResetReplaysSavedCollectorEvidence(t *testing.T) {
	p := newPaths(t)
	copySources(t, p.source)
	expected := capture(t, p)
	if expected.Facts == 0 || expected.Summary.Total == 0 {
		t.Fatal("fixture captured nothing", expected)
	}
	setPragma(t, p.server, "PRAGMA user_version=0")
	report, err := localstorage.InspectLocal(t.Context(), p.collector, p.server, p.app)
	if err != nil {
		t.Fatal(err)
	}
	if !status(t, report, localstorage.Server).Reset || !status(t, report, localstorage.Application).Reset || status(t, report, localstorage.Collector).Reset || report.LosesHistory() {
		t.Fatalf("unexpected plan %+v", report)
	}
	moved, err := localstorage.Reset(t.Context(), report, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.server + ".v0.bak", p.app + ".v2.bak", p.server + ".application.json.v0.bak"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("backup missing", path, moved)
		}
	}
	// Harness logs are gone; history must come from retained collector evidence.
	if err := os.RemoveAll(p.source); err != nil {
		t.Fatal(err)
	}
	if got := capture(t, p); !reflect.DeepEqual(got, expected) {
		t.Fatalf("replayed usage %+v, want %+v", got, expected)
	}
}

func TestCollectorResetKeepsLocalUsageWithoutDoubleCounting(t *testing.T) {
	p := newPaths(t)
	copySources(t, p.source)
	expected := capture(t, p)
	setPragma(t, p.collector, "PRAGMA user_version=19")
	report, err := localstorage.InspectLocal(t.Context(), p.collector, p.server, p.app)
	if err != nil {
		t.Fatal(err)
	}
	if !status(t, report, localstorage.Collector).Reset || status(t, report, localstorage.Server).Reset || status(t, report, localstorage.Application).Reset {
		t.Fatalf("unexpected plan %+v", report)
	}
	if _, err := localstorage.Reset(t.Context(), report, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.collector + ".v19.bak"); err != nil {
		t.Fatal(err)
	}
	// A fresh collector rereads the same logs; stable identities deduplicate them.
	if got := capture(t, p); !reflect.DeepEqual(got, expected) {
		t.Fatalf("usage after collector reset %+v, want %+v", got, expected)
	}
}

func TestBothResetReportsHistoryLoss(t *testing.T) {
	p := newPaths(t)
	createStores(t, p)
	setPragma(t, p.collector, "PRAGMA user_version=19")
	setPragma(t, p.server, "PRAGMA user_version=0")
	report, err := localstorage.InspectLocal(t.Context(), p.collector, p.server, p.app)
	if err != nil || !report.LosesHistory() {
		t.Fatal("history loss not reported", report, err)
	}
}

func TestResetMovesCompanionsAndKeepsEarlierBackups(t *testing.T) {
	p := newPaths(t)
	createStores(t, p)
	setPragma(t, p.collector, "PRAGMA user_version=19")
	report, err := localstorage.InspectSync(t.Context(), p.collector)
	if err != nil {
		t.Fatal(err)
	}
	earlier := p.collector + ".v19.bak"
	for path, body := range map[string]string{earlier: "earlier", p.collector + "-wal": "wal", p.collector + "-shm": "shm"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 23, 9, 30, 0, 0, time.UTC)
	if _, err := localstorage.Reset(t.Context(), report, now); err != nil {
		t.Fatal(err)
	}
	target := earlier + ".20261023T093000Z"
	for path, body := range map[string]string{earlier: "earlier", target + "-wal": "wal", target + "-shm": "shm"} {
		if got, err := os.ReadFile(path); err != nil || string(got) != body {
			t.Fatal(path, string(got), err)
		}
	}
	for _, path := range []string{p.collector, p.collector + "-wal", p.collector + "-shm"} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("left behind", path, err)
		}
	}
	if result, err := collectordb.InspectCompatibility(t.Context(), target); err != nil || result.Status != storagecompat.Older {
		t.Fatal("backup unreadable", result, err)
	}
}

func TestResetRejectsOwnedServerWithoutMoving(t *testing.T) {
	p := newPaths(t)
	createStores(t, p)
	setPragma(t, p.server, "PRAGMA user_version=0")
	report, err := localstorage.InspectLocal(t.Context(), p.collector, p.server, p.app)
	if err != nil {
		t.Fatal(err)
	}
	owner, held, err := serverownership.Lifetime(p.server, true)
	if err != nil || held {
		t.Fatal(held, err)
	}
	defer func() { _ = owner.Close() }()
	before := snapshot(t, p.root)
	if _, err := localstorage.Reset(t.Context(), report, time.Now()); !errors.Is(err, localstorage.ErrBusy) {
		t.Fatal("owned server reset", err)
	}
	if after := snapshot(t, p.root); !reflect.DeepEqual(before, after) {
		t.Fatal("busy reset moved files")
	}
}

func TestBlockedReportResetsNothing(t *testing.T) {
	p := newPaths(t)
	createStores(t, p)
	setPragma(t, p.collector, "PRAGMA user_version=21")
	setPragma(t, p.server, "PRAGMA user_version=0")
	report, err := localstorage.InspectLocal(t.Context(), p.collector, p.server, p.app)
	if err != nil {
		t.Fatal(err)
	}
	if blocked := report.Blocked(); len(blocked) != 1 || blocked[0].Role != localstorage.Collector {
		t.Fatal(blocked)
	}
	before := snapshot(t, p.root)
	if _, err := localstorage.Reset(t.Context(), report, time.Now()); err == nil {
		t.Fatal("blocked report reset")
	}
	if after := snapshot(t, p.root); !reflect.DeepEqual(before, after) {
		t.Fatal("blocked reset moved files")
	}
}
