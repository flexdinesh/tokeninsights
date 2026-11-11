package main

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

func testDashboard(choice *appearance) *dashboard {
	info := endpoint.Info{Transport: "ssh", Target: "ssh://alice@host:2222", State: "connected", Hostname: "studio"}
	return newDashboard(info, choice, func(context.Context) error { return nil })
}

func TestShellShowsConnectionIdentity(t *testing.T) {
	choice := appearanceSystem
	tt := ui.NewTester(testDashboard(&choice).view, windowWidth, windowHeight)
	for _, text := range []string{"TokenInsights", "studio", "SSH · ssh://alice@host:2222 · connected"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestRefreshCoalescesAndReportsFailure(t *testing.T) {
	choice := appearanceSystem
	d := testDashboard(&choice)
	tt := ui.NewTester(d.view, windowWidth, windowHeight)
	if !d.beginRefresh() || d.beginRefresh() {
		t.Fatal("overlapping refresh was not coalesced")
	}
	tt.Frame()
	if !tt.HasText("Refreshing…") {
		t.Fatal(tt.Texts())
	}
	d.endRefresh(errors.New("backend busy"))
	tt.Frame()
	if !tt.HasText("Refresh failed: backend busy") || d.refreshing {
		t.Fatal(tt.Texts())
	}
	if !d.beginRefresh() || d.refreshErr != "" {
		t.Fatal("a new refresh kept the old failure")
	}
}

func TestAppearanceChoiceOverridesSystem(t *testing.T) {
	choice := appearanceSystem
	d := testDashboard(&choice)
	for _, test := range []struct {
		choice     appearance
		systemDark bool
		want       *theme.Tokens
	}{
		{appearanceSystem, false, theme.For(false)},
		{appearanceSystem, true, theme.For(true)},
		{appearanceLight, true, theme.For(false)},
		{appearanceDark, false, theme.For(true)},
	} {
		choice = test.choice
		if got := d.tokens(test.systemDark); got != test.want {
			t.Fatal(test.choice, test.systemDark)
		}
	}
}

func TestStandaloneArgumentsCannotSilentlySelectLocalData(t *testing.T) {
	for _, args := range [][]string{{"--connection", "ssh://host"}, {"tokeninsights-desktop://launch?id=0123456789abcdef0123456789abcdef"}} {
		if err := validateArguments(args); err == nil {
			t.Fatal("unsupported request fell through to standalone local capture", args)
		}
	}
	if err := validateArguments([]string{"tokeninsights-native://launch?id=0123456789abcdef0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
}
