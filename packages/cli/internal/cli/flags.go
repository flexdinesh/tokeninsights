package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type period string

const (
	periodToday     period = "today"
	periodYesterday period = "yesterday"
	periodWeek      period = "week"
	periodMonth     period = "month"
	periodYear      period = "year"
	periodAllTime   period = "all"
	defaultDBName          = "tokeninsights.sqlite"
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
	sortDate      sortMode = "date"
	sortTokens    sortMode = "tokens"
	sortInput     sortMode = "input"
	sortOutput    sortMode = "output"
	sortCacheRead sortMode = "cache read"
	sortName      sortMode = "name"
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
	sessionIDs stringList
	providers  stringList
	models     stringList
	harnesses  stringList
	dayFrom    string
	dayTo      string
}

type tableOptions struct {
	dbPath  string
	period  period
	bucket  timeBucket
	sort    sortMode
	filters filters
}

func parseTableOptions(args []string, stderr io.Writer, requirePeriod bool, defaultPeriod period) (tableOptions, error) {
	flags := flag.NewFlagSet("tokeninsights", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var dbPath string
	var today bool
	var yesterday bool
	var week bool
	var month bool
	var year bool
	var allTime bool
	var bucket string
	var queryFilters filters
	flags.StringVar(&dbPath, "db-path", defaultDBPath(), "path to tokeninsights sqlite db")
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

	if err := flags.Parse(args); err != nil {
		return tableOptions{}, fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return tableOptions{}, fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	selectedDBPath := strings.TrimSpace(dbPath)
	if selectedDBPath == "" {
		selectedDBPath = defaultDBPath()
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

	return tableOptions{dbPath: selectedDBPath, period: selected, bucket: selectedBucket, filters: queryFilters}, nil
}

func defaultDBPath() string {
	envPath := strings.TrimSpace(os.Getenv("TOKENINSIGHTS_DB_PATH"))
	if envPath != "" {
		return envPath
	}
	return filepath.Join(defaultDataPath(), defaultDBName)
}

func defaultDataPath() string {
	xdgDataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "tokeninsights")
	}

	home := strings.TrimSpace(os.Getenv("HOME"))
	if home != "" {
		return filepath.Join(home, ".local", "share", "tokeninsights")
	}

	cwd, err := os.Getwd()
	if err == nil && strings.TrimSpace(cwd) != "" {
		return filepath.Join(cwd, ".tokeninsights-data")
	}

	return filepath.Join(".", ".tokeninsights-data")
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
	local := now.Local()

	switch selected {
	case periodToday:
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	case periodYesterday:
		yesterday := local.AddDate(0, 0, -1)
		return time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, local.Location())
	case periodWeek:
		// Go weekday: Sunday=0, Monday=1, ..., Saturday=6
		// We want Monday as start of week
		offset := int(local.Weekday() - time.Monday)
		if offset < 0 {
			offset += 7
		}
		monday := local.AddDate(0, 0, -offset)
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, local.Location())
	case periodMonth:
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	case periodYear:
		return time.Date(local.Year(), 1, 1, 0, 0, 0, 0, local.Location())
	case periodAllTime:
		return time.Time{}
	default:
		return time.Time{}
	}
}

func periodEnd(now time.Time, selected period) time.Time {
	start := periodStart(now, selected)
	if start.IsZero() {
		return time.Time{}
	}

	switch selected {
	case periodToday, periodYesterday:
		return start.AddDate(0, 0, 1)
	case periodWeek:
		return start.AddDate(0, 0, 7)
	case periodMonth:
		return start.AddDate(0, 1, 0)
	case periodYear:
		return start.AddDate(1, 0, 0)
	default:
		return time.Time{}
	}
}

func validateHarnesses(values stringList) error {
	for _, value := range values {
		if value != "opencode" && value != "pi" && value != "codex" && value != "claude-code" {
			return fmt.Errorf("invalid --harness %q: must be opencode, pi, codex, or claude-code\n%w", value, ErrUsage)
		}
	}
	return nil
}

var ErrUsage = errors.New("usage: tokeninsights <sync|normalize|reset-canonical|reset-all|view> [options]")
