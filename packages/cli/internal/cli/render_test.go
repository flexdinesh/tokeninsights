package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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
