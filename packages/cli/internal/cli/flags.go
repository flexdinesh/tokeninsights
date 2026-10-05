package cli

import (
	"flag"
	"fmt"
	"io"

	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

type period string

const (
	periodToday     period = "today"
	periodYesterday period = "yesterday"
	periodWeek      period = "week"
	periodMonth     period = "month"
	periodYear      period = "year"
	periodAllTime   period = "all"
)

type timeBucket string

const (
	bucketDay   timeBucket = "day"
	bucketWeek  timeBucket = "week"
	bucketMonth timeBucket = "month"
	bucketYear  timeBucket = "year"
)

type sortMode string

const (
	sortDate           sortMode = "date"
	sortTokens         sortMode = "tokens"
	sortInput          sortMode = "input"
	sortOutput         sortMode = "output"
	sortCacheRead      sortMode = "cache read"
	sortName           sortMode = "name"
	sortAverageContext sortMode = "avg ctx"
	sortMedianContext  sortMode = "median ctx"
	sortMaxContext     sortMode = "max ctx"
	sortSessions       sortMode = "sessions"
	sortHarness        sortMode = "harness"
	sortProvider       sortMode = "provider"
	sortModel          sortMode = "model"
)

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			*values = append(*values, trimmed)
		}
	}
	return nil
}

type filters struct {
	sessionIDs   stringList
	providers    stringList
	models       stringList
	harnesses    stringList
	repositories stringList
	directories  stringList
	dayFrom      string
	dayTo        string
}

type tableOptions struct {
	dbPath          string
	serverURL       string
	token           string
	collectorDBPath string
	syncBeforeView  bool
	period          period
	bucket          timeBucket
	sort            sortMode
	repoGroup       db.RepoGroup
	filters         filters
}

func parseTableOptions(args []string, stderr io.Writer, requirePeriod bool, defaultPeriod period) (tableOptions, error) {
	return parseViewerOptions(args, stderr, requirePeriod, defaultPeriod, nil)
}

func parseViewerOptions(args []string, stderr io.Writer, requirePeriod bool, defaultPeriod period, extra func(*flag.FlagSet)) (tableOptions, error) {
	return parseViewerOptionsWithDefaults(args, stderr, requirePeriod, defaultPeriod, extra, (commandInvocation{}).defaults())
}
func parseViewerOptionsWithDefaults(args []string, stderr io.Writer, requirePeriod bool, defaultPeriod period, extra func(*flag.FlagSet), settings config.Settings) (tableOptions, error) {
	flags := flag.NewFlagSet("tokeninsights tui", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var dbPath string
	var today bool
	var yesterday bool
	var week bool
	var month bool
	var year bool
	var allTime bool
	var syncBeforeView bool
	var serverURL, token, collectorDBPath string
	var bucket string
	var queryFilters filters
	flags.StringVar(&dbPath, "server-db-path", settings.ServerDBPath, "local query server database path")
	flags.StringVar(&collectorDBPath, "collector-db-path", settings.CollectorDBPath, "collector database used by startup sync")
	flags.StringVar(&serverURL, "server-url", settings.ServerURL, "query server; empty selects local")
	flags.BoolVar(&syncBeforeView, "sync", true, "collect and publish before viewing; --sync=false reads saved data only")
	flags.BoolVar(&today, "today", false, "show today")
	flags.BoolVar(&yesterday, "yesterday", false, "show yesterday")
	flags.BoolVar(&week, "week", false, "show current calendar week (Mon-Sun)")
	flags.BoolVar(&month, "month", false, "show current calendar month")
	flags.BoolVar(&year, "year", false, "show current calendar year")
	flags.BoolVar(&allTime, "all-time", false, "show all time")
	flags.StringVar(&bucket, "bucket", string(bucketDay), "time bucket: day, week, month, or year")
	flags.Var(&queryFilters.sessionIDs, "session-id", "filter by session id; repeat or comma-separate")
	flags.Var(&queryFilters.providers, "provider", "filter by provider; repeat or comma-separate")
	flags.Var(&queryFilters.models, "model", "filter by model; repeat or comma-separate")
	flags.Var(&queryFilters.harnesses, "harness", "filter by harness; repeat or comma-separate")
	flags.StringVar(&queryFilters.dayFrom, "filter-day-from", "", "filter from local day YYYY-MM-DD")
	flags.StringVar(&queryFilters.dayTo, "filter-day-to", "", "filter to local day YYYY-MM-DD")
	if extra != nil {
		extra(flags)
	}

	if err := flags.Parse(args); err != nil {
		return tableOptions{}, fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return tableOptions{}, fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	selectedDBPath := strings.TrimSpace(dbPath)
	if selectedDBPath == "" {
		selectedDBPath = defaultServerDBPath()
	}

	selected, err := selectedPeriod(today, yesterday, week, month, year, allTime, requirePeriod, defaultPeriod)
	if err != nil {
		return tableOptions{}, err
	}
	selectedBucket, err := selectedBucket(bucket)
	if err != nil {
		return tableOptions{}, err
	}

	if err := validateHarnesses(queryFilters.harnesses); err != nil {
		return tableOptions{}, err
	}

	// Validate date range filters
	if queryFilters.dayFrom != "" {
		if _, err := time.Parse("2006-01-02", queryFilters.dayFrom); err != nil {
			return tableOptions{}, fmt.Errorf("invalid --filter-day-from %q: must be YYYY-MM-DD\n%w", queryFilters.dayFrom, ErrUsage)
		}
	}
	if queryFilters.dayTo != "" {
		if _, err := time.Parse("2006-01-02", queryFilters.dayTo); err != nil {
			return tableOptions{}, fmt.Errorf("invalid --filter-day-to %q: must be YYYY-MM-DD\n%w", queryFilters.dayTo, ErrUsage)
		}
	}
	if queryFilters.dayFrom != "" && queryFilters.dayTo != "" {
		from, _ := time.Parse("2006-01-02", queryFilters.dayFrom)
		to, _ := time.Parse("2006-01-02", queryFilters.dayTo)
		if from.After(to) {
			return tableOptions{}, fmt.Errorf("--filter-day-from must not be after --filter-day-to\n%w", ErrUsage)
		}
	}

	return tableOptions{dbPath: selectedDBPath, serverURL: strings.TrimSpace(serverURL), token: token, collectorDBPath: collectorDBPath, syncBeforeView: syncBeforeView, period: selected, bucket: selectedBucket, filters: queryFilters}, nil
}

func selectedPeriod(today bool, yesterday bool, week bool, month bool, year bool, allTime bool, required bool, fallback period) (period, error) {
	selected := 0
	if today {
		selected++
	}
	if yesterday {
		selected++
	}
	if week {
		selected++
	}
	if month {
		selected++
	}
	if year {
		selected++
	}
	if allTime {
		selected++
	}
	if selected == 0 && !required {
		return fallback, nil
	}
	if selected != 1 {
		return "", fmt.Errorf("choose exactly one of --today, --yesterday, --week, --month, --year, --all-time\n%w", ErrUsage)
	}

	switch {
	case today:
		return periodToday, nil
	case yesterday:
		return periodYesterday, nil
	case week:
		return periodWeek, nil
	case month:
		return periodMonth, nil
	case year:
		return periodYear, nil
	default:
		return periodAllTime, nil
	}
}

func selectedBucket(value string) (timeBucket, error) {
	switch timeBucket(strings.TrimSpace(value)) {
	case bucketDay:
		return bucketDay, nil
	case bucketWeek:
		return bucketWeek, nil
	case bucketMonth:
		return bucketMonth, nil
	case bucketYear:
		return bucketYear, nil
	default:
		return "", fmt.Errorf("invalid --bucket %q: must be day, week, month, or year\n%w", value, ErrUsage)
	}
}

func periodStart(now time.Time, selected period) time.Time {
	return viewer.PeriodStart(now, string(selected))
}

func validateHarnesses(values stringList) error {
	for _, value := range values {
		if value != "opencode" && value != "pi" && value != "codex" && value != "claude-code" {
			return fmt.Errorf("invalid --harness %q: must be opencode, pi, codex, or claude-code\n%w", value, ErrUsage)
		}
	}
	return nil
}

func harnessList(values stringList) []pipeline.Harness {
	result := make([]pipeline.Harness, 0, len(values))
	for _, value := range values {
		result = append(result, pipeline.Harness(value))
	}
	return result
}
