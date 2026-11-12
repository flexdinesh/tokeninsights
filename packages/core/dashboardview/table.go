package dashboardview

import (
	"slices"
	"strings"
	"unicode"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
)

// Column is one results-table column. Its ID is also the sort it requests.
type Column struct {
	ID      Sort
	Label   string
	Numeric bool
}

// Column widths in device-independent pixels.
const (
	nameColumnWidth     = 224
	repoNameColumnWidth = 256
	numericColumnWidth  = 128
	textColumnWidth     = 160
	nameColumnMinWidth  = 176
	numericColumnMin    = 112
	textColumnMin       = 128
	// ColumnMaxWidth bounds every resizable column.
	ColumnMaxWidth = 480
	// ColumnResizeStep is the keyboard resize increment.
	ColumnResizeStep = 16
)

// ColumnWidth is a column's default and minimum width; ColumnMaxWidth bounds all.
type ColumnWidth struct {
	Default, Min int
}

func tokenColumns() []Column {
	return []Column{
		{SortTotal, "Total", true}, {SortInput, "Input", true}, {SortOutput, "Output", true},
		{SortCacheRead, "Cache R", true}, {SortCacheWrite, "Cache W", true}, {SortReasoning, "Reasoning", true},
	}
}

// Columns lists a tab's table columns in display order. Name, when present, is
// first and cannot be hidden.
func Columns(tab Tab) []Column {
	switch tab {
	case TabContext:
		return []Column{
			{SortHarness, "Harness", false}, {SortProvider, "Provider", false}, {SortModel, "Model", false},
			{SortSessions, "Sessions", true}, {SortAverageContext, "Avg ctx", true},
			{SortMedianContext, "Median ctx", true}, {SortMaxContext, "Max ctx", true},
		}
	case TabRepo:
		return append([]Column{
			{SortName, "Location", false}, {SortProvider, "Providers", false}, {SortHarness, "Harnesses", false},
			{SortModel, "Models", false}, {SortSessions, "Sessions", true},
		}, tokenColumns()...)
	}
	columns := []Column{{SortName, firstColumnLabel(tab), false}}
	if tab == TabSessions {
		columns = append(columns, Column{SortDate, "Last active", false})
	} else {
		columns = append(columns, Column{SortSessions, "Sessions", true})
	}
	columns = append(columns, tokenColumns()...)
	if tab == TabSessions {
		columns = append(columns, Column{SortContext, "Ctx used", true})
	}
	return columns
}

func firstColumnLabel(tab Tab) string {
	switch tab {
	case TabTokens:
		return "Period"
	case TabSessions:
		return "Session"
	case TabHarnesses:
		return "Harness"
	}
	label := TabLabel(tab) // Models and Providers drop their plural "s".
	if label == "" {
		return ""
	}
	return label[:len(label)-1]
}

// ColumnWidths returns a column's default and minimum widths on tab.
func ColumnWidths(tab Tab, column Column) ColumnWidth {
	switch {
	case column.ID == SortName && tab == TabRepo:
		return ColumnWidth{repoNameColumnWidth, nameColumnMinWidth}
	case column.ID == SortName:
		return ColumnWidth{nameColumnWidth, nameColumnMinWidth}
	case column.Numeric:
		return ColumnWidth{numericColumnWidth, numericColumnMin}
	}
	return ColumnWidth{textColumnWidth, textColumnMin}
}

// ColumnVisible reports whether a column shows given the hidden set; Name always shows.
func ColumnVisible(column Sort, hidden []Sort) bool {
	return column == SortName || !slices.Contains(hidden, column)
}

// SortOptions lists the sort selector's choices for tab.
func SortOptions(tab Tab) []Column {
	if tab == TabContext {
		return Columns(tab)
	}
	options := append([]Column{{SortDate, "Date", false}, {SortName, "Name", false}}, tokenColumns()...)
	return append(options, Column{SortSessions, "Sessions", false}, Column{SortContext, "Ctx used", false})
}

// HeaderSort is the sort a column header requests: Tokens periods sort by date.
func HeaderSort(tab Tab, column Sort) Sort {
	if column == SortName && tab == TabTokens {
		return SortDate
	}
	return column
}

// TableTitle titles the results table, prefixed for excluded usage.
func TableTitle(tab Tab, quality Quality) string {
	title := TabLabel(tab) + " breakdown"
	if tab == TabRepo {
		title = "Repo totals"
	}
	if quality == QualityEstimated {
		return "Excluded " + strings.ToLower(title)
	}
	return title
}

// SummaryLines splits a comma-joined dimension summary into trimmed, nonempty lines.
func SummaryLines(value string) []string {
	lines := []string{}
	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimFunc(part, jsWhitespace); part != "" {
			lines = append(lines, part)
		}
	}
	return lines
}

// jsWhitespace matches String.prototype.trim: Unicode White_Space and BOM, not NEL.
func jsWhitespace(r rune) bool {
	return r == '\uFEFF' || r != '\u0085' && unicode.IsSpace(r)
}

// IdentityDetails are the secondary lines under a row name: the dimensions the
// tab does not group by (deduplicated, then split), or a Repo row's repository
// when it differs from the location.
func IdentityDetails(row analytics.Row, tab Tab) []string {
	switch tab {
	case TabTokens:
		return []string{}
	case TabRepo:
		if row.RepositoryName != "unknown" && row.RepositoryName != row.LocationName {
			return []string{row.RepositoryName}
		}
		return []string{}
	}
	candidates := []string{}
	if tab != TabHarnesses {
		candidates = append(candidates, row.Harness)
	}
	if tab != TabProviders {
		candidates = append(candidates, row.Provider)
	}
	if tab != TabModels {
		candidates = append(candidates, row.Model)
	}
	details := []string{}
	var seen []string
	for _, candidate := range candidates {
		if candidate == "" || slices.Contains(seen, candidate) {
			continue
		}
		seen = append(seen, candidate)
		details = append(details, SummaryLines(candidate)...)
	}
	return details
}

// RowValue is a row's count in the numeric column column, as the web reads
// row[column]; ok is false for a text column.
func RowValue(row analytics.Row, column Sort) (value int64, ok bool) {
	switch column {
	case SortTotal:
		return row.Total, true
	case SortInput:
		return row.Input, true
	case SortOutput:
		return row.Output, true
	case SortReasoning:
		return row.Reasoning, true
	case SortCacheRead:
		return row.CacheRead, true
	case SortCacheWrite:
		return row.CacheWrite, true
	case SortSessions:
		return row.Sessions, true
	case SortContext:
		return row.Context, true
	case SortAverageContext:
		return row.AverageContext, true
	case SortMedianContext:
		return row.MedianContext, true
	case SortMaxContext:
		return row.MaxContext, true
	}
	return 0, false
}

// RowText is a row's text in the text column column: its name or one of
// its dimensions, "" for another column.
func RowText(row analytics.Row, column Sort) string {
	switch column {
	case SortName:
		return row.Name
	case SortHarness:
		return row.Harness
	case SortProvider:
		return row.Provider
	case SortModel:
		return row.Model
	}
	return ""
}
