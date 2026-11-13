package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/preferences"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/views"
)

func TestMain(m *testing.M) {
	if err := theme.RegisterFonts(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// idleBackend answers a window that never reads in these tests.
type idleBackend struct{}

func (idleBackend) Instance(context.Context) (application.Instance, error) {
	return application.Instance{Hostname: "studio"}, nil
}
func (idleBackend) Capture(context.Context) error { return nil }
func (idleBackend) Status(context.Context) (application.Status, error) {
	return application.Status{}, nil
}
func (idleBackend) Progress(context.Context) (collectorprogress.Snapshot, error) {
	return collectorprogress.Snapshot{}, nil
}
func (idleBackend) Usage(context.Context, analytics.Query) (application.Usage, error) {
	return application.Usage{}, nil
}
func (idleBackend) Facets(context.Context, application.FacetRequest) (application.Facets, error) {
	return application.Facets{}, nil
}

// testAppearance is the app's appearance over a preferences file in dir,
// with its View menu built and its flushes joined by flushes.
func testAppearance(t *testing.T, dir string, flushes *sync.WaitGroup, applied *[]theme.Appearance) *appearance {
	t.Helper()
	store, saved, err := preferences.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &appearance{choice: saved.Appearance, store: store, spawn: flushes.Go,
		applied: func(choice theme.Appearance) { *applied = append(*applied, choice) }}
	a.menu = mygo.NewMenu([]*mygo.MenuItem{{Label: "View", Submenu: a.menuItems()}})
	return a
}

func checkedItems(a *appearance) []theme.Appearance {
	var checked []theme.Appearance
	for _, choice := range theme.Appearances() {
		if a.menu.ItemByID(appearanceMenuIDs[choice]).IsChecked() {
			checked = append(checked, choice)
		}
	}
	return checked
}

func TestHeaderThemeCycleSyncsMenuAndPersists(t *testing.T) {
	dir := t.TempDir()
	var flushes sync.WaitGroup
	var applied []theme.Appearance
	a := testAppearance(t, dir, &flushes, &applied)
	if got := checkedItems(a); len(got) != 1 || got[0] != theme.AppearanceSystem {
		t.Fatalf("first run checks %v", got)
	}
	window := model.NewWindow(endpoint.Info{Transport: "local", Target: "local", State: "connected"}, idleBackend{},
		model.Runtime{Context: context.Background(), Go: func(func()) {}, Update: func(fn func()) { fn() }, Now: time.Now})
	tt := ui.NewTester(func(c *ui.Context) {
		views.AppShell(c, views.Shell{Window: window, Appearance: a.choice, SetAppearance: a.set})
	}, windowWidth, windowHeight)
	if err := tt.Click("Theme: system. Change theme"); err != nil {
		t.Fatal(err)
	}
	flushes.Wait()
	if a.choice != theme.AppearanceLight || len(applied) != 1 || applied[0] != theme.AppearanceLight {
		t.Fatalf("choice %s, applied %v", a.choice, applied)
	}
	if got := checkedItems(a); len(got) != 1 || got[0] != theme.AppearanceLight {
		t.Fatalf("menu checks %v after the header chose light", got)
	}
	if !tt.HasText("Theme: light. Change theme") {
		t.Fatal(tt.Texts())
	}
	if _, saved, err := preferences.Open(dir); err != nil || saved.Appearance != theme.AppearanceLight {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	// A new run starts from the saved choice with its radio checked.
	restarted := testAppearance(t, dir, &flushes, &applied)
	if got := checkedItems(restarted); restarted.choice != theme.AppearanceLight || len(got) != 1 || got[0] != theme.AppearanceLight {
		t.Fatalf("restart chose %s, checks %v", restarted.choice, got)
	}
}

func TestMenuRadioChoosesAppearance(t *testing.T) {
	var flushes sync.WaitGroup
	var applied []theme.Appearance
	a := testAppearance(t, t.TempDir(), &flushes, &applied)
	item := a.menu.ItemByID(appearanceMenuIDs[theme.AppearanceDark])
	item.Click(item, nil)
	flushes.Wait()
	if got := checkedItems(a); a.choice != theme.AppearanceDark || len(got) != 1 || got[0] != theme.AppearanceDark {
		t.Fatalf("choice %s, checks %v", a.choice, got)
	}
}

func TestAppearanceWithoutStoreStillApplies(t *testing.T) {
	var applied []theme.Appearance
	a := &appearance{choice: theme.AppearanceSystem, applied: func(choice theme.Appearance) { applied = append(applied, choice) },
		spawn: func(func()) { t.Fatal("persisted without a store") }}
	a.set(theme.AppearanceDark)
	a.set("sepia")
	if a.choice != theme.AppearanceDark || len(applied) != 1 {
		t.Fatalf("choice %s, applied %v", a.choice, applied)
	}
}
