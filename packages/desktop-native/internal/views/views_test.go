package views

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
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

// fakeBackend answers with fixed results.
type fakeBackend struct {
	instance    application.Instance
	instanceErr error
	captureErr  error
	captures    int
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

func (r *workers) run() {
	pending := r.pending
	r.pending = nil
	for _, fn := range pending {
		fn()
	}
}

var (
	studioConnection = endpoint.Info{Transport: "ssh", Target: "ssh://alice@studio.lan:2222", State: "connected", Hostname: "studio.lan"}
	studioInstance   = application.Instance{
		InstanceID: "4f1c9a0e-7b2d-4c55-9a61-0d3e8b7f2c10", Hostname: "studio", Timezone: "Europe/London",
		Defaults: analytics.Selection{Period: "month", From: "2026-11-01", To: "2026-11-30", Bucket: "day"},
	}
)

// fixture is a window over a fake backend and the app-wide appearance.
type fixture struct {
	backend    *fakeBackend
	workers    *workers
	window     *model.Window
	appearance theme.Appearance
}

func newFixture() *fixture {
	f := &fixture{backend: &fakeBackend{instance: studioInstance}, workers: &workers{}, appearance: theme.AppearanceSystem}
	rt := model.Runtime{Context: context.Background(), Go: f.workers.Go, Update: func(fn func()) { fn() }}
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

func (f *fixture) tester(width, height int, dark bool) *ui.Tester {
	return ui.NewTesterWith(f.view, width, height, ui.TesterOptions{Dark: dark})
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
	tt := f.tester(windowWidth, windowHeight, false)
	assertTexts(t, tt, "TokenInsights", "Personal workspace", "Usage metadata only")
	for _, tab := range dashboardview.Tabs() {
		label := dashboardview.TabLabel(tab)
		a, ok := tt.AccessByRole(ui.RoleButton, label)
		if !ok {
			t.Fatalf("no %s tab in %q", label, tt.Texts())
		}
		current := tab == model.DefaultTab
		if a.Disabled == current || (a.Current == ui.CurrentPage) != current {
			t.Errorf("%s tab: disabled %v, current %v", label, a.Disabled, a.Current)
		}
	}
	_ = tt.Click("Models")
	if f.window.Tab() != model.DefaultTab {
		t.Fatal("a disabled tab was selected")
	}
	assertTexts(t, tt, "/ Tokens", placeholderText)
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
	tt := f.tester(windowWidth, windowHeight, false)
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

func TestInstanceFailureShowsQuietDangerLine(t *testing.T) {
	f := newFixture()
	f.backend.instanceErr = errors.New("backend busy")
	f.window.Load()
	f.workers.run()
	tt := f.tester(windowWidth, windowHeight, false)
	assertTexts(t, tt, instanceFailed, "studio.lan")
}

func TestThemeButtonCyclesAppearance(t *testing.T) {
	f := loaded()
	tt := f.tester(windowWidth, windowHeight, false)
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
	tt := f.tester(minWidth, minHeight, false)
	// A point of the page between the placeholder and the footer.
	got := tt.Image().RGBAAt(minWidth*3/4, minHeight*3/4)
	want := theme.For(true).Surface
	if got.R != want.R || got.G != want.G || got.B != want.B {
		t.Fatalf("dark choice on a light system painted %v, want %v", got, want)
	}
}

func TestEveryControlIsNamed(t *testing.T) {
	for _, size := range [][2]int{{windowWidth, windowHeight}, {minWidth, minHeight}} {
		tt := loaded().tester(size[0], size[1], false)
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
				tt := f.tester(shot.width, shot.height, dark)
				if shot.hover {
					r, ok := tt.Find("studio")
					if !ok {
						t.Fatal("no hostname")
					}
					tt.Move(r.X+r.W/2, r.Y+r.H/2)
				}
				tt.Advance(settle)
				assertTexts(t, tt, "/ Tokens", "Refresh", placeholderText, refreshFailed)
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
