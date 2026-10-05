package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Prefer a rule-bearing IANA name so clients format historical DST correctly.
// Unnamed/local installations fall back to an explicitly fixed current offset.
func reportingTimezone(location *time.Location, now time.Time) string {
	if name := location.String(); name != "Local" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}
	if location == time.Local {
		name := strings.TrimPrefix(os.Getenv("TZ"), ":")
		if name == "" {
			if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
				name = target
			}
		}
		if _, suffix, found := strings.Cut(name, "zoneinfo/"); found {
			name = suffix
		}
		if name != "" && !filepath.IsAbs(name) {
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	_, offset := now.In(location).Zone()
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, offset%3600/60)
}
