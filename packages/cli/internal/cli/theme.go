package cli

import (
	"github.com/charmbracelet/lipgloss"
)

// TUI visual tokens. Semantic roles only; no feature-specific colors.
// Backgrounds stay transparent so light/dark terminals own the canvas.

const (
	tuiColGap          = 2
	tuiChromeHeight    = 12
	tuiMinVisibleLines = 5
)

var (
	themeBrand  = lipgloss.AdaptiveColor{Light: "5", Dark: "212"}
	themeAccent = lipgloss.AdaptiveColor{Light: "4", Dark: "81"}
	themeMuted  = lipgloss.AdaptiveColor{Light: "242", Dark: "241"}
	themeFaint  = lipgloss.AdaptiveColor{Light: "249", Dark: "238"}
	themeText   = lipgloss.AdaptiveColor{Light: "0", Dark: "252"}
	themeTotal  = lipgloss.AdaptiveColor{Light: "28", Dark: "157"}
	themeDanger = lipgloss.AdaptiveColor{Light: "1", Dark: "203"}

	themeTabActiveBg = lipgloss.AdaptiveColor{Light: "5", Dark: "212"}
	themeTabActiveFg = lipgloss.AdaptiveColor{Light: "255", Dark: "232"}

	themeSelectedBg = lipgloss.AdaptiveColor{Light: "4", Dark: "63"}
	themeSelectedFg = lipgloss.AdaptiveColor{Light: "255", Dark: "230"}
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(themeBrand)
	hintStyle   = lipgloss.NewStyle().Foreground(themeMuted)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(themeAccent)

	dimensionStyle   = lipgloss.NewStyle().Foreground(themeText)
	textCellStyle    = lipgloss.NewStyle().Foreground(themeText)
	mutedCellStyle   = lipgloss.NewStyle().Foreground(themeMuted)
	metricStyle      = lipgloss.NewStyle().Foreground(themeText)
	contextUsedStyle = lipgloss.NewStyle().Bold(true).Foreground(themeText)
	totalStyle       = lipgloss.NewStyle().Bold(true).Foreground(themeTotal)

	appSurfaceStyle = lipgloss.NewStyle()
	rowOddStyle     = lipgloss.NewStyle()
	selectedRowStyle = lipgloss.NewStyle().
				Foreground(themeSelectedFg).
				Background(themeSelectedBg)

	dividerStyle = lipgloss.NewStyle().Foreground(themeFaint)
	footerStyle   = lipgloss.NewStyle().Foreground(themeMuted)
	footerDimStyle = lipgloss.NewStyle().Foreground(themeMuted)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(themeTabActiveFg).
			Background(themeTabActiveBg).
			Padding(0, 2)
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(themeText).
				Padding(0, 2)
	tabBarStyle = lipgloss.NewStyle().
			Padding(0, 1)
	tabGapStyle = lipgloss.NewStyle().
			Padding(0, 1)

	tableSectionStyle = lipgloss.NewStyle().Padding(0, 1)

	popupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(themeFaint).
			Padding(1, 2)
	popupTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(themeText)
	popupCursorStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(themeAccent)
	popupItemStyle = lipgloss.NewStyle().Foreground(themeText)

	statuslineSurfaceStyle  = lipgloss.NewStyle().Bold(false)
	statuslineBrandStyle    = lipgloss.NewStyle().Bold(true).Foreground(themeBrand)
	statuslineItemStyle     = lipgloss.NewStyle().Foreground(themeMuted)
	statuslineSeparatorStyle = lipgloss.NewStyle().Foreground(themeFaint)

	tableSummarySurfaceStyle   = lipgloss.NewStyle()
	tableSummaryLabelStyle     = lipgloss.NewStyle().Foreground(themeMuted)
	tableSummaryTotalStyle     = lipgloss.NewStyle().Bold(true).Foreground(themeTotal)
	tableSummarySeparatorStyle = lipgloss.NewStyle().Foreground(themeFaint)

	syncOKStyle   = lipgloss.NewStyle().Foreground(themeTotal)
	syncFailStyle = lipgloss.NewStyle().Foreground(themeDanger)
	syncBusyStyle = lipgloss.NewStyle().Foreground(themeAccent)
	syncSkipStyle = lipgloss.NewStyle().Foreground(themeMuted)
)
