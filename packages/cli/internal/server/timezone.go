package server

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"time"
)

func reportingTimezone(location *time.Location, now time.Time) string {
	return analytics.ReportingTimezone(location, now)
}
