package views

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Window sizes views render at: the default window and the minimum.
const (
	windowWidth  = 1280
	windowHeight = 850
	minWidth     = 720
	minHeight    = 520
)

// galleryDirEnv names the directory view PNGs are written to, as the kit
// gallery's. Unset, the tests only render and assert.
const galleryDirEnv = "TOKENINSIGHTS_NATIVE_GALLERY_DIR"

// galleryScale renders PNGs at a Retina scale for review.
const galleryScale = 2

// settle is longer than any kit transition or tooltip delay.
const settle = time.Second

func TestMain(m *testing.M) {
	if err := theme.RegisterFonts(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// fakeBackend answers with fixed results and counts the reads.
type fakeBackend struct {
	instance    application.Instance
	instanceErr error
	captureErr  error
	captures    int
	status      application.Status
	statusErr   error
	statuses    int
	progress    collectorprogress.Snapshot
	progressErr error
	usage       application.Usage
	usageErr    error
	// rows are each tab's rows, which usage reads answer a page of;
	// pageErr fails the reads of the pages after the first.
	rows    map[dashboardview.Tab][]analytics.Row
	pageErr error
	// requests are the usage requests read.
	requests []analytics.Query
	// facets answer facets reads, sessions matching a search; facetsErr
	// fails them and searchErr fails searches; facetRequests are the
	// facets requests read.
	facets        application.Facets
	facetsErr     error
	searchErr     error
	facetRequests []application.FacetRequest
}

func (b *fakeBackend) Facets(_ context.Context, request application.FacetRequest) (application.Facets, error) {
	b.facetRequests = append(b.facetRequests, request)
	if b.facetsErr != nil {
		return application.Facets{}, b.facetsErr
	}
	facets := b.facets
	if request.Search != "" {
		if b.searchErr != nil {
			return application.Facets{}, b.searchErr
		}
		facets.Sessions = slices.DeleteFunc(slices.Clone(facets.Sessions), func(s string) bool {
			return !strings.Contains(s, request.Search)
		})
	}
	return facets, nil
}

func (b *fakeBackend) Usage(_ context.Context, query analytics.Query) (application.Usage, error) {
	b.requests = append(b.requests, query)
	if b.usageErr != nil {
		return application.Usage{}, b.usageErr
	}
	if query.Page > 1 && b.pageErr != nil {
		return application.Usage{}, b.pageErr
	}
	usage := b.usage
	rows := b.rows[dashboardview.Tab(query.Tab)]
	start := min((query.Page-1)*query.PageSize, len(rows))
	usage.Rows = rows[start:min(start+query.PageSize, len(rows))]
	usage.RowCount, usage.Page, usage.PageSize = len(rows), query.Page, query.PageSize
	return usage, nil
}

func (b *fakeBackend) Status(context.Context) (application.Status, error) {
	b.statuses++
	return b.status, b.statusErr
}

func (b *fakeBackend) Progress(context.Context) (collectorprogress.Snapshot, error) {
	return b.progress, b.progressErr
}

func (b *fakeBackend) Instance(context.Context) (application.Instance, error) {
	return b.instance, b.instanceErr
}

func (b *fakeBackend) Capture(context.Context) error {
	b.captures++
	return b.captureErr
}

// workers holds a window's workers until the test runs them; their
// updates apply at once, as the test is the main thread.
type workers struct{ pending []func() }

func (r *workers) Go(fn func()) { r.pending = append(r.pending, fn) }

// step runs the pending workers, leaving those they start pending.
func (r *workers) step() {
	pending := r.pending
	r.pending = nil
	for _, fn := range pending {
		fn()
	}
}

// run runs the pending workers and those they start.
func (r *workers) run() {
	for len(r.pending) > 0 {
		r.step()
	}
}

var (
	studioConnection = endpoint.Info{Transport: "ssh", Target: "ssh://alice@studio.lan:2222", State: "connected", Hostname: "studio.lan"}
	studioInstance   = application.Instance{
		InstanceID: "4f1c9a0e-7b2d-4c55-9a61-0d3e8b7f2c10", DatasetID: "default", Hostname: "studio", Timezone: "Europe/London",
		Capabilities: serverfeatures.Capabilities{serverfeatures.Usage, serverfeatures.Facets, serverfeatures.CollectorProgress},
		Permissions:  serverfeatures.Permissions{serverfeatures.Read},
		Defaults:     analytics.Selection{Period: "month", Bucket: "day"},
	}
	studioStatus = application.Status{InstanceID: studioInstance.InstanceID, Readiness: "ready", Metadata: application.Publication{
		DatabaseID: "db-7a2e", DatasetID: "default", Revision: 42, Generation: 3, TargetGeneration: 3, InputRevision: 42,
	}}
)

// lastIngestion is when the fixtures' usage was last ingested: 14 Sep
// 2026, 10:00 UTC, as the web mocks'.
var lastIngestion = time.Date(2026, time.September, 14, 10, 0, 0, 0, time.UTC).UnixMilli()

// studioUsage is the web mocks' usage summary, answered from studioStatus's
// snapshot.
var studioUsage = application.Usage{InstanceID: studioInstance.InstanceID, Dashboard: analytics.Dashboard{
	DatabaseID: "db-7a2e", DatasetID: "default", Revision: 42, Generation: 3,
	Summary: analytics.ViewerSummaryRow{
		TotalTokens: 647_000, InputTokens: 369_000, OutputTokens: 78_000, ReasoningTokens: 13_000,
		CacheReadTokens: 178_000, CacheWriteTokens: 9_000, SessionCount: 4, SyncedSessions: 5,
	},
	LastSynced: lastIngestion, Range: "Sep 12 – Sep 14, 2026",
}}

// studioFacets are the web mocks' filter values, answered from
// studioStatus's snapshot.
var studioFacets = application.Facets{InstanceID: studioInstance.InstanceID, Facets: analytics.Facets{
	DatabaseID: "db-7a2e", DatasetID: "default", Revision: 42, Generation: 3,
	Harnesses: []string{"claude-code", "codex", "opencode", "pi"},
	Providers: []string{"anthropic", "openai"},
	Models:    []string{"claude-sonnet-4-5", "gpt-5", "gpt-5-mini"},
	Sessions:  []string{"mock-claude-session", "mock-codex-session", "mock-opencode-session", "mock-pi-session"},
}}

// totalReadout names the total readout of studioUsage by its exact count.
const totalReadout = "Total tokens: 647,000"

// readsAt is the wall time of the fixtures' reads: 13 Nov 2026, 09:30 UTC.
var readsAt = time.Date(2026, time.November, 13, 9, 30, 0, 0, time.UTC)

// fixture is a window over a fake backend and the app-wide appearance.
type fixture struct {
	backend    *fakeBackend
	workers    *workers
	window     *model.Window
	appearance theme.Appearance
}

func newFixture() *fixture {
	backend := &fakeBackend{instance: studioInstance, status: studioStatus, progress: collectorprogress.Snapshot{InstanceID: studioInstance.InstanceID},
		usage: studioUsage, rows: mockRows(), facets: studioFacets}
	f := &fixture{backend: backend, workers: &workers{}, appearance: theme.AppearanceSystem}
	rt := model.Runtime{Context: context.Background(), Go: f.workers.Go, Update: func(fn func()) { fn() }, Now: func() time.Time { return readsAt }}
	f.window = model.NewWindow(studioConnection, f.backend, rt)
	return f
}

// loaded is a fixture whose instance has loaded.
func loaded() *fixture {
	f := newFixture()
	f.window.Load()
	f.workers.run()
	return f
}

func (f *fixture) view(c *ui.Context) {
	AppShell(c, Shell{Window: f.window, Appearance: f.appearance, SetAppearance: func(a theme.Appearance) { f.appearance = a }})
}

// tester renders the shell, whose first frame starts the status and
// progress reads; synced testers have answered them.
func (f *fixture) tester(width, height int, dark bool) *ui.Tester {
	return ui.NewTesterWith(f.view, width, height, ui.TesterOptions{Dark: dark})
}

// synced is a tester whose first reads were answered.
func (f *fixture) synced(width, height int, dark bool) *ui.Tester {
	tt := f.tester(width, height, dark)
	f.sync(tt)
	return tt
}

// sync runs the pending reads and shows their results.
func (f *fixture) sync(tt *ui.Tester) {
	f.workers.run()
	tt.Frame()
}

func assertTexts(t *testing.T, tt *ui.Tester, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestSidebarShowsEveryTabAndDisablesUnportedOnes(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, "TokenInsights", "Personal workspace", "Usage metadata only")
	for _, tab := range dashboardview.Tabs() {
		label := dashboardview.TabLabel(tab)
		a, ok := tt.AccessByRole(ui.RoleButton, label)
		if !ok {
			t.Fatalf("no %s tab in %q", label, tt.Texts())
		}
		current := tab == model.DefaultTab
		if a.Disabled == ported(tab) || (a.Current == ui.CurrentPage) != current {
			t.Errorf("%s tab: disabled %v, current %v", label, a.Disabled, a.Current)
		}
	}
	if ported(dashboardview.TabRepo) {
		t.Fatal("Repo is enabled before its views")
	}
	_ = tt.Click("Repo")
	if f.window.Tab() != model.DefaultTab {
		t.Fatal("a disabled tab was selected")
	}
	if err := tt.Click("Models"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if f.window.Tab() != dashboardview.TabModels || len(f.backend.requests) != 2 || f.backend.requests[1].Tab != string(dashboardview.TabModels) {
		t.Fatalf("models tab: %s, requests %+v", f.window.Tab(), f.backend.requests)
	}
	assertTexts(t, tt, "/ Models", totalReadout)
	if a, ok := tt.AccessByRole(ui.RoleButton, "Models"); !ok || a.Current != ui.CurrentPage {
		t.Fatalf("models tab not current: %+v", a)
	}
}

func TestHeaderShowsIdentityAndConnection(t *testing.T) {
	f := newFixture()
	tt := f.tester(windowWidth, windowHeight, false)
	assertTexts(t, tt, "/ Tokens", "studio.lan", "SSH · ssh://alice@studio.lan:2222 · connected", "studio.lan · ssh://alice@studio.lan:2222")
	if a, ok := tt.Access("studio.lan"); !ok || a.Description != "" {
		t.Fatalf("hostname before the instance loaded: %+v", a)
	}

	f.window.Load()
	f.workers.run()
	tt.Frame()
	assertTexts(t, tt, "studio", "studio · ssh://alice@studio.lan:2222 · Europe/London")
	host, ok := tt.Access("studio")
	if !ok || host.Description != "Backend hostname: studio · Identity: "+studioInstance.InstanceID {
		t.Fatalf("hostname tooltip: %+v", host)
	}
	if status, ok := tt.AccessByRole(ui.RoleStatus, "SSH · ssh://alice@studio.lan:2222 · connected"); !ok {
		t.Fatalf("connection state: %+v", status)
	}
}

func TestRefreshButtonDisablesWhilePendingAndShowsFailure(t *testing.T) {
	f := loaded()
	f.backend.captureErr = errors.New("capture failed")
	tt := f.synced(windowWidth, windowHeight, false)
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	assertTexts(t, tt, "Refreshing…")
	if a, ok := tt.AccessByRole(ui.RoleButton, "Refreshing…"); !ok || !a.Disabled {
		t.Fatalf("pending refresh button: %+v", a)
	}
	_ = tt.Click("Refreshing…")
	f.workers.run()
	tt.Frame()
	if f.backend.captures != 1 {
		t.Fatalf("captures %d", f.backend.captures)
	}
	assertTexts(t, tt, "Refresh", refreshFailed)
	if a, ok := tt.AccessByRole(ui.RoleAlert, refreshFailed); !ok {
		t.Fatalf("refresh failure: %+v", a)
	}

	f.backend.captureErr = nil
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	f.workers.run()
	tt.Frame()
	if tt.HasText(refreshFailed) {
		t.Fatal("a successful refresh kept the failure")
	}
}

func TestInstanceFailureShowsAConnectionAlertThatRetries(t *testing.T) {
	f := newFixture()
	f.backend.instanceErr = errors.New("backend busy")
	f.window.Load()
	f.workers.run()
	tt := f.tester(windowWidth, windowHeight, false)
	assertTexts(t, tt, "backend busy", "studio.lan")
	if _, ok := tt.AccessByRole(ui.RoleAlert, ""); !ok {
		t.Fatal("no connection alert")
	}
	f.backend.instanceErr = nil
	if err := tt.Click(retryRequest); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	f.sync(tt)
	if _, ok := f.window.Instance(); !ok || tt.HasText("backend busy") {
		t.Fatal("Retry Request did not load the instance")
	}
	assertTexts(t, tt, totalReadout)
}

func TestThemeButtonCyclesAppearance(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	for _, want := range []theme.Appearance{theme.AppearanceLight, theme.AppearanceDark, theme.AppearanceSystem} {
		name := "Theme: " + string(f.appearance) + ". Change theme"
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
		if f.appearance != want {
			t.Fatalf("appearance %s, want %s", f.appearance, want)
		}
	}
}

func TestShellThemesFromTheChoice(t *testing.T) {
	f := loaded()
	f.appearance = theme.AppearanceDark
	tt := f.synced(minWidth, minHeight, false)
	// A point of the page's right gutter, beside the results.
	got := tt.Image().RGBAAt(minWidth-int(theme.Gutter)/2, minHeight*3/4)
	want := theme.For(true).Surface
	if got.R != want.R || got.G != want.G || got.B != want.B {
		t.Fatalf("dark choice on a light system painted %v, want %v", got, want)
	}
}

func TestEveryControlIsNamed(t *testing.T) {
	for _, size := range [][2]int{{windowWidth, windowHeight}, {minWidth, minHeight}} {
		tt := loaded().synced(size[0], size[1], false)
		for _, a := range tt.AccessAll() {
			if a.Role == ui.RoleButton && a.Name == "" {
				t.Errorf("unnamed button at %v", a.Bounds)
			}
		}
	}
}

// TestShellImages renders the shell in both appearances at the default and
// minimum window sizes, and writes PNGs when TOKENINSIGHTS_NATIVE_GALLERY_DIR
// is set.
func TestShellImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name          string
		width, height int
		hover         bool
	}{{"default", windowWidth, windowHeight, false}, {"min", minWidth, minHeight, false}, {"tooltip", windowWidth, windowHeight, true}} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := loaded()
				f.backend.captureErr = errors.New("capture failed")
				f.window.Refresh()
				f.workers.run()
				tt := f.synced(shot.width, shot.height, dark)
				if shot.hover {
					r, ok := tt.Find("studio")
					if !ok {
						t.Fatal("no hostname")
					}
					tt.Move(r.X+r.W/2, r.Y+r.H/2)
				}
				tt.Advance(settle)
				assertTexts(t, tt, "/ Tokens", "Refresh", totalReadout, refreshFailed)
				if shot.hover {
					assertTexts(t, tt, "Backend hostname: studio · Identity: "+studioInstance.InstanceID)
				}
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, "shell-"+shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}

func writePNG(t *testing.T, path string, tt *ui.Tester) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
