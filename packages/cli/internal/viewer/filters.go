// Package viewer holds presentation-independent viewer semantics.
package viewer

import (
	"fmt"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

type Selection struct {
	Period    string   `json:"period"`
	Bucket    string   `json:"bucket"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Providers []string `json:"providers"`
	Models    []string `json:"models"`
	Harnesses []string `json:"harnesses"`
	Sessions  []string `json:"sessions"`
}

func (s Selection) Validate() error {
	switch s.Period {
	case "today", "yesterday", "week", "month", "year", "all":
	default:
		return fmt.Errorf("invalid period %q", s.Period)
	}
	switch s.Bucket {
	case "day", "week", "month", "year":
	default:
		return fmt.Errorf("invalid bucket %q", s.Bucket)
	}
	for _, day := range []string{s.From, s.To} {
		if day != "" {
			if _, err := time.Parse(time.DateOnly, day); err != nil {
				return fmt.Errorf("invalid date %q: use YYYY-MM-DD", day)
			}
		}
	}
	if s.From != "" && s.To != "" && s.From > s.To {
		return fmt.Errorf("from must not be after to")
	}
	for _, h := range s.Harnesses {
		switch h {
		case "opencode", "pi", "codex", "claude-code":
		default:
			return fmt.Errorf("invalid harness %q", h)
		}
	}
	return nil
}

func PeriodStart(now time.Time, period string) time.Time {
	local := now.Local()
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	switch period {
	case "today":
		return day
	case "yesterday":
		return day.AddDate(0, 0, -1)
	case "week":
		return day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	case "month":
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	case "year":
		return time.Date(local.Year(), 1, 1, 0, 0, 0, 0, local.Location())
	default:
		return time.Time{}
	}
}

func PeriodEnd(now time.Time, period string) time.Time {
	start := PeriodStart(now, period)
	switch period {
	case "today", "yesterday":
		return start.AddDate(0, 0, 1)
	case "week":
		return start.AddDate(0, 0, 7)
	case "month":
		return start.AddDate(0, 1, 0)
	case "year":
		return start.AddDate(1, 0, 0)
	default:
		return time.Time{}
	}
}

func (s Selection) Filter(now time.Time) db.Filter {
	f := db.Filter{Start: PeriodStart(now, s.Period), End: PeriodEnd(now, s.Period), DayFrom: s.From, DayTo: s.To,
		Providers: s.Providers, Models: s.Models, Harnesses: s.Harnesses, SessionIDs: s.Sessions}
	if s.From != "" || s.To != "" {
		f.Start, f.End = time.Time{}, time.Time{}
	}
	return f
}
