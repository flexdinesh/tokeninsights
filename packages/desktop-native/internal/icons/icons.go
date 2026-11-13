// Package icons embeds the Lucide icons the web dashboard uses, as SVGs that
// ui.Icon draws in the color of the text. Each is parsed once, as the package
// loads; the accessors share the parsed icon across windows.
package icons

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// files holds the Lucide sources (lucide-react 1.59.0, ISC; see lucide/LICENSE).
//
//go:embed lucide/*.svg
var files embed.FS

// load parses one embedded icon by its file name.
func load(name string) *ui.SVG {
	data, err := files.ReadFile("lucide/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

var (
	activitySVG            = load("activity")
	arrowDownSVG           = load("arrow-down")
	arrowLeftSVG           = load("arrow-left")
	arrowRightSVG          = load("arrow-right")
	arrowUpSVG             = load("arrow-up")
	arrowUpDownSVG         = load("arrow-up-down")
	boxSVG                 = load("box")
	calendarDaysSVG        = load("calendar-days")
	chartNoAxesCombinedSVG = load("chart-no-axes-combined")
	checkSVG               = load("check")
	chevronDownSVG         = load("chevron-down")
	chevronLeftSVG         = load("chevron-left")
	chevronRightSVG        = load("chevron-right")
	chevronUpSVG           = load("chevron-up")
	circleAlertSVG         = load("circle-alert")
	columns3SVG            = load("columns-3")
	commandSVG             = load("command")
	copySVG                = load("copy")
	filterSVG              = load("filter")
	gaugeSVG               = load("gauge")
	gitBranchSVG           = load("git-branch")
	layers3SVG             = load("layers-3")
	loaderCircleSVG        = load("loader-circle")
	monitorSVG             = load("monitor")
	moonSVG                = load("moon")
	rotateCwSVG            = load("rotate-cw")
	searchSVG              = load("search")
	searchXSVG             = load("search-x")
	sunSVG                 = load("sun")
	xSVG                   = load("x")
)

// Activity is Lucide activity.
func Activity() *ui.SVG { return activitySVG }

// ArrowDown is Lucide arrow-down.
func ArrowDown() *ui.SVG { return arrowDownSVG }

// ArrowLeft is Lucide arrow-left.
func ArrowLeft() *ui.SVG { return arrowLeftSVG }

// ArrowRight is Lucide arrow-right.
func ArrowRight() *ui.SVG { return arrowRightSVG }

// ArrowUp is Lucide arrow-up.
func ArrowUp() *ui.SVG { return arrowUpSVG }

// ArrowUpDown is Lucide arrow-up-down.
func ArrowUpDown() *ui.SVG { return arrowUpDownSVG }

// Box is Lucide box.
func Box() *ui.SVG { return boxSVG }

// CalendarDays is Lucide calendar-days.
func CalendarDays() *ui.SVG { return calendarDaysSVG }

// ChartNoAxesCombined is Lucide chart-no-axes-combined.
func ChartNoAxesCombined() *ui.SVG { return chartNoAxesCombinedSVG }

// Check is Lucide check.
func Check() *ui.SVG { return checkSVG }

// ChevronDown is Lucide chevron-down.
func ChevronDown() *ui.SVG { return chevronDownSVG }

// ChevronLeft is Lucide chevron-left.
func ChevronLeft() *ui.SVG { return chevronLeftSVG }

// ChevronRight is Lucide chevron-right.
func ChevronRight() *ui.SVG { return chevronRightSVG }

// ChevronUp is Lucide chevron-up.
func ChevronUp() *ui.SVG { return chevronUpSVG }

// CircleAlert is Lucide circle-alert.
func CircleAlert() *ui.SVG { return circleAlertSVG }

// Columns3 is Lucide columns-3.
func Columns3() *ui.SVG { return columns3SVG }

// Command is Lucide command.
func Command() *ui.SVG { return commandSVG }

// Copy is Lucide copy.
func Copy() *ui.SVG { return copySVG }

// Filter is Lucide filter.
func Filter() *ui.SVG { return filterSVG }

// Gauge is Lucide gauge.
func Gauge() *ui.SVG { return gaugeSVG }

// GitBranch is Lucide git-branch.
func GitBranch() *ui.SVG { return gitBranchSVG }

// Layers3 is Lucide layers-3.
func Layers3() *ui.SVG { return layers3SVG }

// LoaderCircle is Lucide loader-circle.
func LoaderCircle() *ui.SVG { return loaderCircleSVG }

// Monitor is Lucide monitor.
func Monitor() *ui.SVG { return monitorSVG }

// Moon is Lucide moon.
func Moon() *ui.SVG { return moonSVG }

// RotateCw is Lucide rotate-cw.
func RotateCw() *ui.SVG { return rotateCwSVG }

// Search is Lucide search.
func Search() *ui.SVG { return searchSVG }

// SearchX is Lucide search-x.
func SearchX() *ui.SVG { return searchXSVG }

// Sun is Lucide sun.
func Sun() *ui.SVG { return sunSVG }

// X is Lucide x.
func X() *ui.SVG { return xSVG }

// byFile maps each embedded file name to its accessor, for the tests.
var byFile = map[string]func() *ui.SVG{
	"activity":               Activity,
	"arrow-down":             ArrowDown,
	"arrow-left":             ArrowLeft,
	"arrow-right":            ArrowRight,
	"arrow-up":               ArrowUp,
	"arrow-up-down":          ArrowUpDown,
	"box":                    Box,
	"calendar-days":          CalendarDays,
	"chart-no-axes-combined": ChartNoAxesCombined,
	"check":                  Check,
	"chevron-down":           ChevronDown,
	"chevron-left":           ChevronLeft,
	"chevron-right":          ChevronRight,
	"chevron-up":             ChevronUp,
	"circle-alert":           CircleAlert,
	"columns-3":              Columns3,
	"command":                Command,
	"copy":                   Copy,
	"filter":                 Filter,
	"gauge":                  Gauge,
	"git-branch":             GitBranch,
	"layers-3":               Layers3,
	"loader-circle":          LoaderCircle,
	"monitor":                Monitor,
	"moon":                   Moon,
	"rotate-cw":              RotateCw,
	"search":                 Search,
	"search-x":               SearchX,
	"sun":                    Sun,
	"x":                      X,
}
