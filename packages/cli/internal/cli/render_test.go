package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

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

	for _, expected := range []string{"bucket", "sessions", "cache R", "cache W", "2026-04-24", "394"} {
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

	for _, expected := range []string{"model", "providers", "harnesses", "azure", "openai", "opencode", "pi"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
	for _, unexpected := range []string{"azure, openai", "opencode, pi"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("output should stack summary values instead of rendering %q:\n%s", unexpected, output)
		}
	}
}

func TestRenderTableSessionsIncludesContextUsed(t *testing.T) {
	output := ansi.Strip(renderTable([]renderRow{{
		latest:            "2026-04-24 12:00",
		sessionID:         "ses_12345678",
		harness:           "opencode",
		providers:         "openai",
		models:            "gpt-5",
		contextUsedTokens: "10k",
		inputTokens:       "300",
		outputTokens:      "30",
		reasoningTokens:   "11",
		cacheReadTokens:   "50",
		cacheWriteTokens:  "3",
		totalTokens:       "394",
	}}, groupByNone, tabSessions))

	for _, expected := range []string{"latest", "session", "ctx used", "cache R", "cache W", "10k"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}

func TestRenderTableSessionSummariesStackValues(t *testing.T) {
	output := ansi.Strip(renderTable([]renderRow{{
		latest:            "2026-04-24 12:00",
		sessionID:         "ses_12345678",
		harness:           "opencode",
		providers:         "anthropic, azure, openai",
		models:            "claude, gpt-5, o4-mini",
		contextUsedTokens: "10k",
		inputTokens:       "300",
		totalTokens:       "394",
	}}, groupByNone, tabSessions))

	for _, expected := range []string{"anthropic", "azure", "openai", "claude", "gpt-5", "o4-mini"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
	for _, unexpected := range []string{"anthropic, azure", "claude, gpt-5"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("output should stack session summary values instead of rendering %q:\n%s", unexpected, output)
		}
	}
}

func TestRenderTableEmptyState(t *testing.T) {
	output := ansi.Strip(renderTable(nil, groupByNone, tabTokens))
	if !strings.Contains(output, "No rows match the current scope.") {
		t.Fatalf("missing empty state:\n%s", output)
	}
}

func TestRenderTableViewportUsesStripedDataBackgrounds(t *testing.T) {
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

	if !strings.Contains(output, "\x1b[48;2;27;27;42m") {
		t.Fatalf("table viewport missing app background:\n%q", output)
	}
	if !strings.Contains(output, "\x1b[48;2;36;36;44m") {
		t.Fatalf("table viewport missing alternate row background:\n%q", output)
	}
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	for _, line := range lines {
		switch {
		case strings.Contains(ansi.Strip(line), "2026-06-14"):
			assertLineCellsHaveBackground(t, line, "48;2;27;27;42")
		case strings.Contains(ansi.Strip(line), "2026-06-13"):
			assertLineCellsHaveBackground(t, line, "48;2;36;36;44")
		}
	}
}

func TestViewFrameAndSeparatorsPaintAppBackground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	m := interactiveModel{
		rows: []renderRow{
			{bucket: "2026-06-14", sessions: "1", inputTokens: "35K", outputTokens: "1K", totalTokens: "114K"},
		},
		activeTab: tabTokens,
		period:    periodMonth,
		width:     100,
		height:    24,
		options:   tableOptions{period: periodMonth},
	}
	m = m.measureHeights()

	output := m.View()
	for _, line := range strings.Split(output, "\n") {
		assertStyledRunesHaveBackground(t, line, "╭╮╰╯│─", "48;2;27;27;42")
	}
}

func TestViewTitleRowPaintsFullAppBackground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	m := interactiveModel{
		rows: []renderRow{
			{bucket: "2026-06-14", sessions: "1", inputTokens: "35K", outputTokens: "1K", totalTokens: "114K"},
		},
		activeTab: tabTokens,
		period:    periodAllTime,
		width:     100,
		height:    24,
		options:   tableOptions{period: periodAllTime},
	}
	m = m.measureHeights()

	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(ansi.Strip(line), "TokenInsights") {
			assertLineCellsHaveBackground(t, line, "48;2;27;27;42")
			return
		}
	}
	t.Fatal("title row not found")
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
	if !strings.Contains(header, "cache R") || !strings.Contains(header, "cache W") || !strings.Contains(header, "total") {
		t.Fatalf("providers table initial viewport missing cache R, cache W, or total column:\n%s", output)
	}
}

func assertLineCellsHaveBackground(t *testing.T, line string, background string) {
	t.Helper()

	hasBackground := false
	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			end := strings.IndexByte(line[i:], 'm')
			if end < 0 {
				i++
				continue
			}
			sequence := line[i+2 : i+end]
			if sequence == "0" {
				hasBackground = false
			} else if strings.Contains(sequence, background) {
				hasBackground = true
			} else if strings.Contains(sequence, "49") {
				hasBackground = false
			}
			i += end + 1
			continue
		}

		_, size := rune(line[i]), 1
		if line[i] >= 0x80 {
			_, size = utf8.DecodeRuneInString(line[i:])
		}
		if !hasBackground {
			t.Fatalf("cell is missing app background in line %q", line)
		}
		i += size
	}
}

func TestProvidersTableStacksLongListsBeforeHorizontalOverflow(t *testing.T) {
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
	if !strings.Contains(header, "cache R") || !strings.Contains(header, "cache W") || !strings.Contains(header, "total") {
		t.Fatalf("providers table initial viewport missing cache R, cache W, or total column:\n%s", output)
	}
	for _, expected := range []string{"gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark", "gpt-5.2-codex"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("providers table did not show stacked list value %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "gpt-5.5, gpt-5.4") {
		t.Fatalf("providers table rendered long list on one line:\n%s", output)
	}
}

func assertStyledRunesHaveBackground(t *testing.T, line string, runes string, background string) {
	t.Helper()

	hasBackground := false
	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			end := strings.IndexByte(line[i:], 'm')
			if end < 0 {
				i++
				continue
			}
			sequence := line[i+2 : i+end]
			if sequence == "0" {
				hasBackground = false
			} else if strings.Contains(sequence, background) {
				hasBackground = true
			} else if strings.Contains(sequence, "49") {
				hasBackground = false
			}
			i += end + 1
			continue
		}

		r, size := rune(line[i]), 1
		if r >= 0x80 {
			r, size = utf8.DecodeRuneInString(line[i:])
		}
		if strings.ContainsRune(runes, r) && !hasBackground {
			t.Fatalf("rune %q is missing app background in line %q", r, line)
		}
		i += size
	}
}
