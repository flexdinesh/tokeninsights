package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestHorizontalViewportSlicesAndPadsStyledContent(t *testing.T) {
	output := horizontalViewport("\x1b[31mabcdef\x1b[0m\n", 2, 3)
	line := strings.TrimSuffix(output, "\n")
	if !strings.Contains(line, "cde") {
		t.Fatalf("got %q, want viewport to include cde", output)
	}
	if width := ansi.StringWidth(line); width != 3 {
		t.Fatalf("got width %d, want 3", width)
	}
}

func TestRenderTableTokensUsesBucketAndSessionColumns(t *testing.T) {
	output := ansi.Strip(renderTable([]renderRow{{
		bucket:           "2026-04-24",
		sessions:         "2",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}}, groupByNone, tabTokens))

	for _, expected := range []string{"bucket", "sessions", "2026-04-24", "394"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "provider") || strings.Contains(output, "╭") {
		t.Fatalf("output contains legacy table surface:\n%s", output)
	}
}

func TestRenderTableModelSummaries(t *testing.T) {
	output := ansi.Strip(renderTable([]renderRow{{
		model:            "gpt-5",
		providers:        "azure, openai",
		harnesses:        "opencode, pi",
		sessions:         "2",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}}, groupByNone, tabModels))

	for _, expected := range []string{"model", "providers", "harnesses", "azure, openai", "opencode, pi"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}

func TestRenderTableEmptyState(t *testing.T) {
	output := ansi.Strip(renderTable(nil, groupByNone, tabTokens))
	if !strings.Contains(output, "No rows match the current scope.") {
		t.Fatalf("missing empty state:\n%s", output)
	}
}

func TestRenderTableViewportUsesConsistentBackground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	output := renderTableViewportWithSort([]renderRow{
		{bucket: "2026-06-14", sessions: "1", inputTokens: "35K", outputTokens: "1K", totalTokens: "114K"},
		{bucket: "2026-06-13", sessions: "38", inputTokens: "6M", outputTokens: "510K", totalTokens: "79M"},
	}, nil, groupByNone, tabTokens, sortDate, 80, 0, 4)

	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if width := ansi.StringWidth(line); width != 80 {
			t.Fatalf("got line width %d, want 80 for %q", width, line)
		}
	}

	if strings.Contains(output, "\x1b[48;2;36;36;44m") {
		t.Fatalf("table viewport contains full-width stripe background:\n%q", output)
	}
	if !strings.Contains(output, "\x1b[48;2;27;27;42m") {
		t.Fatalf("table viewport missing app background:\n%q", output)
	}
}

func TestProvidersTableInitialViewportIncludesTotalColumn(t *testing.T) {
	output := ansi.Strip(renderTableViewportWithSort([]renderRow{
		{
			provider:         "fireworks-ai",
			models:           "accounts/fireworks/models/kimi-k2p7-code",
			harnesses:        "opencode",
			sessions:         "1",
			inputTokens:      "10K",
			outputTokens:     "138",
			reasoningTokens:  "",
			cacheReadTokens:  "",
			cacheWriteTokens: "",
			totalTokens:      "10K",
		},
	}, nil, groupByNone, tabProviders, sortTokens, 150, 0, 4))

	header := tableHeaderLine(output, "provider")
	if !strings.Contains(header, "total") {
		t.Fatalf("providers table initial viewport missing total column:\n%s", output)
	}
}

func TestProvidersTableTruncatesLongListsBeforeHorizontalOverflow(t *testing.T) {
	output := ansi.Strip(renderTableViewportWithSort([]renderRow{
		{
			provider:         "openai",
			models:           "gpt-5.5, gpt-5.4, gpt-5.3-codex-spark, gpt-5.3-codex, gpt-5.2-codex",
			harnesses:        "codex, opencode, pi",
			sessions:         "260",
			inputTokens:      "31M",
			outputTokens:     "2M",
			reasoningTokens:  "919K",
			cacheReadTokens:  "330M",
			cacheWriteTokens: "1M",
			totalTokens:      "366M",
		},
	}, nil, groupByNone, tabProviders, sortTokens, 120, 0, 4))

	header := tableHeaderLine(output, "provider")
	if !strings.Contains(header, "total") {
		t.Fatalf("providers table initial viewport missing total column:\n%s", output)
	}
	if !strings.Contains(output, "...") {
		t.Fatalf("providers table did not show truncated long list:\n%s", output)
	}
}
