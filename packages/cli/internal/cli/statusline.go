package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type statuslineItemID int

const (
	statuslineBrand statuslineItemID = iota
	statuslineDateRange
	statuslineHostname
	statuslineLastSynced
)

const statuslineSeparator = " · "

type statuslineItem struct {
	id    statuslineItemID
	label string
	value string
}

func (item statuslineItem) text() string {
	if item.label == "" {
		return item.value
	}
	return fmt.Sprintf("%s: %s", item.label, item.value)
}

type statuslineModel struct {
	items []statuslineItem
}

func newStatuslineModel(dateRange string, hostname string, lastSyncMs int64) statuslineModel {
	return statuslineModel{items: []statuslineItem{
		{id: statuslineBrand, value: "TokenInsights"},
		{id: statuslineDateRange, label: "daterange", value: dateRange},
		{id: statuslineHostname, label: "hostname", value: hostname},
		{id: statuslineLastSynced, label: "lastsynced", value: formatLastSync(lastSyncMs)},
	}}
}

func (s statuslineModel) withValue(id statuslineItemID, value string) statuslineModel {
	items := append([]statuslineItem(nil), s.items...)
	for i := range items {
		if items[i].id == id {
			items[i].value = value
			break
		}
	}
	s.items = items
	return s
}

func (s statuslineModel) value(id statuslineItemID) string {
	for _, item := range s.items {
		if item.id == id {
			return item.value
		}
	}
	return ""
}

func (s statuslineModel) View(width int) string {
	if width <= 0 {
		return ""
	}

	items := append([]statuslineItem(nil), s.items...)
	items = fitStatuslineItems(items, width)
	line := renderStatuslineItems(items)
	return statuslineSurfaceStyle.Width(width).MaxWidth(width).Render(line)
}

func fitStatuslineItems(items []statuslineItem, width int) []statuslineItem {
	if statuslineItemsWidth(items) <= width {
		return items
	}

	items = shrinkStatuslineValue(items, statuslineHostname, width)
	if statuslineItemsWidth(items) <= width {
		return items
	}
	items = removeStatuslineItem(items, statuslineHostname)
	if statuslineItemsWidth(items) <= width {
		return items
	}

	items = shrinkStatuslineValue(items, statuslineLastSynced, width)
	if statuslineItemsWidth(items) <= width {
		return items
	}
	items = removeStatuslineItem(items, statuslineLastSynced)
	if statuslineItemsWidth(items) <= width {
		return items
	}

	brandIndex := statuslineItemIndex(items, statuslineBrand)
	dateIndex := statuslineItemIndex(items, statuslineDateRange)
	if brandIndex >= 0 && dateIndex >= 0 {
		brandWidth := ansi.StringWidth(items[brandIndex].text())
		dateWidth := width - brandWidth - ansi.StringWidth(statuslineSeparator)
		if dateWidth > 0 {
			dateText := items[dateIndex].text()
			items[dateIndex].label = ""
			items[dateIndex].value = truncateCell(dateText, dateWidth)
			if statuslineItemsWidth(items) <= width {
				return items
			}
		}
		items = removeStatuslineItem(items, statuslineDateRange)
	}

	if index := statuslineItemIndex(items, statuslineBrand); index >= 0 {
		items[index].value = truncateCell(items[index].text(), width)
	}
	return items
}

func shrinkStatuslineValue(items []statuslineItem, id statuslineItemID, width int) []statuslineItem {
	index := statuslineItemIndex(items, id)
	if index < 0 {
		return items
	}
	overflow := statuslineItemsWidth(items) - width
	valueWidth := ansi.StringWidth(items[index].value)
	targetWidth := max(ansi.StringWidth(truncationMarker), valueWidth-overflow)
	if targetWidth >= valueWidth {
		return items
	}
	items[index].value = truncateCell(items[index].value, targetWidth)
	return items
}

func removeStatuslineItem(items []statuslineItem, id statuslineItemID) []statuslineItem {
	index := statuslineItemIndex(items, id)
	if index < 0 {
		return items
	}
	return append(items[:index:index], items[index+1:]...)
}

func statuslineItemIndex(items []statuslineItem, id statuslineItemID) int {
	for i, item := range items {
		if item.id == id {
			return i
		}
	}
	return -1
}

func statuslineItemsWidth(items []statuslineItem) int {
	if len(items) == 0 {
		return 0
	}
	width := (len(items) - 1) * ansi.StringWidth(statuslineSeparator)
	for _, item := range items {
		width += ansi.StringWidth(item.text())
	}
	return width
}

func renderStatuslineItems(items []statuslineItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, statuslineStyle(item.id).Render(item.text()))
	}
	return strings.Join(parts, statuslineSeparatorStyle.Render(statuslineSeparator))
}

func statuslineStyle(id statuslineItemID) lipgloss.Style {
	switch id {
	case statuslineBrand:
		return statuslineBrandStyle
	case statuslineDateRange:
		return statuslineDateRangeStyle
	case statuslineHostname:
		return statuslineHostnameStyle
	case statuslineLastSynced:
		return statuslineLastSyncedStyle
	default:
		return statuslineSeparatorStyle
	}
}

func statuslineDateRangeLabel(options tableOptions) string {
	from := strings.TrimSpace(options.filters.dayFrom)
	to := strings.TrimSpace(options.filters.dayTo)
	switch {
	case from != "" && to != "":
		return from + ".." + to
	case from != "":
		return from + ".."
	case to != "":
		return ".." + to
	default:
		label := periodLabel(options.period)
		if label == "" {
			return "unknown"
		}
		return label
	}
}

func normalizeHostname(value string, err error) string {
	if err != nil || strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(value)
}

func formatLastSync(value int64) string {
	if value <= 0 {
		return "never"
	}
	return formatLatest(value)
}

var (
	statuslineSurfaceStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(appBackgroundColor))
	statuslineBrandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				Background(lipgloss.Color(appBackgroundColor))
	statuslineDateRangeStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("86")).
					Background(lipgloss.Color(appBackgroundColor))
	statuslineHostnameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("113")).
				Background(lipgloss.Color(appBackgroundColor))
	statuslineLastSyncedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("179")).
					Background(lipgloss.Color(appBackgroundColor))
	statuslineSeparatorStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("241")).
					Background(lipgloss.Color(appBackgroundColor))
)
