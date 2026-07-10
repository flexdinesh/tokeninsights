package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestTableSummaryRendersFullResultTotals(t *testing.T) {
	summary := newTableSummaryModel([]renderRow{
		{totalValue: 4_000_000},
		{totalValue: 9_000_000},
	}, tabTokens, false)
	output := summary.View(40)
	plain := ansi.Strip(output)

	if ansi.StringWidth(output) != 40 {
		t.Fatalf("summary width = %d, want 40", ansi.StringWidth(output))
	}
	if strings.Contains(output, "\n") {
		t.Fatalf("summary wrapped: %q", output)
	}
	if !strings.HasPrefix(plain, "rows 2 · total 13M") {
		t.Fatalf("unexpected summary: %q", plain)
	}
}

func TestTableSummaryLoadedEmptyStateShowsZeroes(t *testing.T) {
	output := ansi.Strip(newTableSummaryModel(nil, tabTokens, false).View(32))
	if !strings.HasPrefix(output, "rows 0 · total 0") {
		t.Fatalf("unexpected empty summary: %q", output)
	}
}

func TestTableSummaryContextShowsRowsOnly(t *testing.T) {
	output := ansi.Strip(newTableSummaryModel([]renderRow{{}, {}}, tabContext, false).View(32))
	if !strings.HasPrefix(output, "rows 2") {
		t.Fatalf("context summary missing rows: %q", output)
	}
	if strings.Contains(output, "total") {
		t.Fatalf("context summary should not render total: %q", output)
	}
}

func TestTableSummaryLoadingReservesBlankRow(t *testing.T) {
	output := newTableSummaryModel([]renderRow{{totalValue: 10}}, tabTokens, true).View(32)
	if ansi.StringWidth(output) != 32 {
		t.Fatalf("loading summary width = %d, want 32", ansi.StringWidth(output))
	}
	if strings.TrimSpace(ansi.Strip(output)) != "" {
		t.Fatalf("loading summary should be blank: %q", ansi.Strip(output))
	}
	if strings.Contains(output, "\n") {
		t.Fatalf("loading summary wrapped: %q", output)
	}
}

func TestTableSummaryStaysWithinNarrowWidths(t *testing.T) {
	summary := newTableSummaryModel([]renderRow{{totalValue: 13_000_000}}, tabTokens, false)
	for _, width := range []int{20, 10, 3, 1} {
		output := summary.View(width)
		if got := ansi.StringWidth(output); got != width {
			t.Fatalf("width %d rendered %d cells", width, got)
		}
		if strings.Contains(output, "\n") {
			t.Fatalf("width %d wrapped: %q", width, output)
		}
	}
}

func TestTableSummaryUsesExistingPalette(t *testing.T) {
	if got := tableSummaryLabelStyle.GetForeground(); got != lipgloss.Color("245") {
		t.Fatalf("row label foreground = %v, want 245", got)
	}
	if got := tableSummaryTotalStyle.GetForeground(); got != lipgloss.Color("157") {
		t.Fatalf("total foreground = %v, want 157", got)
	}
	if got := tableSummarySurfaceStyle.GetBackground(); got != lipgloss.Color(rowStripeColor) {
		t.Fatalf("summary background = %v, want %s", got, rowStripeColor)
	}
}

func TestTableSummaryPaintsFullStripeBackground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	output := newTableSummaryModel([]renderRow{{totalValue: 136}}, tabTokens, false).View(40)
	assertLineCellsHaveBackground(t, output, "48;2;36;36;44")
}
