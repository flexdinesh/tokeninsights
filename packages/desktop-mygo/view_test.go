package main

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func fixtureModel() *viewModel {
	return &viewModel{
		worker:  &loader{requests: make(chan loadRequest, 1), results: make(chan loadResult, 1)},
		filters: filters{Period: "month"}, loaded: true, generation: 1,
		data: snapshot{
			Totals: totals{Total: 123456, Input: 100000, Output: 20000, Reasoning: 3456, CacheRead: 0, CacheWrite: 0, Sessions: 42},
			Range:  "1 November 2026 – 30 November 2026", Collection: "Collection complete", Processing: "Processing complete",
			Rows:      []usageRow{{Provider: "openai", Model: "gpt-6", Harness: "codex", Tokens: totals{Total: 123456, Input: 100000, Output: 20000, Reasoning: 3456}}},
			Providers: []string{"openai", "All Providers"}, Models: []string{"gpt-6"}, Harnesses: []string{"codex"},
		},
	}
}

func TestNativeViewShowsUsageAcrossThemesAndSizes(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{{"desktop", 1180, 780}, {"compact", 720, 520}} {
		for _, dark := range []bool{false, true} {
			t.Run(size.name+map[bool]string{false: "-light", true: "-dark"}[dark], func(t *testing.T) {
				m := fixtureModel()
				tester := ui.NewTester(m.view, size.width, size.height)
				tester.SetDark(dark)
				for _, text := range []string{"Local usage", "123,456", "42 sessions", "Date range", "Provider", "Model", "Harness", "gpt-6", "Collection complete"} {
					if !tester.HasText(text) {
						t.Errorf("missing %q in %v", text, tester.Texts())
					}
				}
				if dir := os.Getenv("TOKENINSIGHTS_NATIVE_CAPTURE_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(dir, t.Name()[len("TestNativeViewShowsUsageAcrossThemesAndSizes/"):]+".png"))
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = file.Close() }()
					if err := png.Encode(file, tester.Image()); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestNativeFiltersSubmitQueryAndCustomDates(t *testing.T) {
	m := fixtureModel()
	tester := ui.NewTester(m.view, 1180, 780)
	if err := tester.Click("All providers"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Provider: openai"); err != nil {
		t.Fatal(err)
	}
	request := <-m.worker.requests
	if request.Retry || request.Filters.Provider != "openai" {
		t.Fatalf("filter request: %+v", request)
	}
	if err := tester.Click("month"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("custom"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Apply dates"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Enter both dates") {
		t.Fatal("missing invalid-date feedback")
	}
	select {
	case request := <-m.worker.requests:
		t.Fatalf("invalid dates requested %+v", request)
	default:
	}
	m.filters.From, m.filters.To = "2026-03-01", "2026-03-31"
	tester.Frame()
	if err := tester.Click("Apply dates"); err != nil {
		t.Fatal(err)
	}
	request = <-m.worker.requests
	if request.Retry || request.Filters.From != "2026-03-01" || request.Filters.To != "2026-03-31" {
		t.Fatalf("date request: %+v", request)
	}
}

func TestNativeErrorRetainsSavedDataAndRetries(t *testing.T) {
	m := fixtureModel()
	m.worker.results <- loadResult{Generation: 1, Err: errors.New("connection lost")}
	tester := ui.NewTester(m.view, 1180, 780)
	if !tester.HasText("connection lost") || !tester.HasText("123,456") {
		t.Fatal("failure removed saved usage")
	}
	if err := tester.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if request := <-m.worker.requests; request.Retry {
		t.Fatal("query retry collected")
	}
	m.loading, m.data.CollectionFailed = false, true
	tester.Frame()
	if err := tester.Click("Retry collection"); err != nil {
		t.Fatal(err)
	}
	if request := <-m.worker.requests; !request.Retry {
		t.Fatal("collection retry did not request capture")
	}
}

func TestSupersededResultCannotOverwriteCurrentFilters(t *testing.T) {
	m := fixtureModel()
	m.generation = 3
	m.worker.results <- loadResult{Generation: 2, Data: snapshot{Totals: totals{Total: 999}}}
	m.receive()
	if m.data.Totals.Total != 123456 {
		t.Fatal("obsolete filter result replaced current usage")
	}
}

func TestNativeViewAcceptsInputWhileConnectionIsBlocked(t *testing.T) {
	started := make(chan struct{})
	m := newViewModel(func(ctx context.Context, _ string, _ bool) (dataSource, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}, "")
	defer func() { m.close(); waitClosed(t, m.worker.done) }()
	waitClosed(t, started)
	tester := ui.NewTester(m.view, 1180, 780)
	if !tester.HasText("Connecting to local usage") {
		t.Fatal("missing startup feedback")
	}
	if err := tester.Click("Clear filters"); err != nil {
		t.Fatal(err)
	}
	if m.filters.Period != "all" {
		t.Fatal("blocked connection blocked GUI input")
	}
}
