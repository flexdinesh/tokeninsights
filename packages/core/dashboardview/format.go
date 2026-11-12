package dashboardview

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Number formats mirror Intl.NumberFormat('en') as used by the web dashboard.
const (
	groupSize = 3
	// Compact notation only groups integer parts with at least this many digits
	// (CLDR minimumGroupingDigits 2), e.g. "1000T" but "10,000T".
	compactGroupingMinDigits = 5
	decimalBase              = 10
	// Values smaller than this share render as "<0.1%" so tiny usage stays distinguishable.
	minimumDisplayedShare = 0.001
	percentScale          = 2 // Percent moves the decimal point two places.
	shareFractionDigits   = 1
	roundingDigit         = 5 // Half-expand: a first dropped digit of 5 or more rounds away from zero.
)

var compactUnits = []struct {
	exponent int
	suffix   string
}{{3, "K"}, {6, "M"}, {9, "B"}, {12, "T"}}

// CompactCount formats like Intl en compact notation with at most one fraction
// digit: 999, 1.2K, 3.4M, 1B, 1000T. Rounding is half-expand and a rounded value
// that reaches the next unit moves to it (999,950 is "1M").
func CompactCount(n int64) string {
	sign, magnitude := splitSign(n)
	unit := -1
	for index, candidate := range compactUnits {
		if magnitude >= pow10(candidate.exponent) {
			unit = index
		}
	}
	if unit < 0 {
		return sign + strconv.FormatUint(magnitude, decimalBase)
	}
	tenths := roundedTenths(magnitude, compactUnits[unit].exponent)
	if tenths >= pow10(groupSize+1) && unit+1 < len(compactUnits) {
		unit++
		tenths = roundedTenths(magnitude, compactUnits[unit].exponent)
	}
	whole := strconv.FormatUint(tenths/decimalBase, decimalBase)
	if len(whole) >= compactGroupingMinDigits {
		whole = group(whole)
	}
	if fraction := tenths % decimalBase; fraction != 0 {
		whole += "." + strconv.FormatUint(fraction, decimalBase)
	}
	return sign + whole + compactUnits[unit].suffix
}

// ExactCount formats an integer with en grouping separators, e.g. 1,234,567.
func ExactCount(n int64) string {
	sign, magnitude := splitSign(n)
	return sign + group(strconv.FormatUint(magnitude, decimalBase))
}

// TokenShare formats tokens/total as an en percentage with at most one fraction
// digit. A zero total is 0%; a positive share below 0.1% is "<0.1%".
func TokenShare(tokens, total int64) string {
	share := 0.0
	if total > 0 {
		share = float64(tokens) / float64(total)
	}
	if share > 0 && share < minimumDisplayedShare {
		return "<" + formatPercent(minimumDisplayedShare)
	}
	return formatPercent(share)
}

// formatPercent rounds the shortest round-trip decimal of value, as ICU does,
// rather than its exact binary expansion.
func formatPercent(value float64) string {
	sign := ""
	if value < 0 {
		sign, value = "-", -value
	}
	mantissa, exponentText, _ := strings.Cut(strconv.FormatFloat(value, 'e', -1, 64), "e")
	exponent, _ := strconv.Atoi(exponentText)
	digits := strings.Replace(mantissa, ".", "", 1)
	// value = 0.digits × 10^(exponent+1); keep shareFractionDigits after scaling.
	keep := exponent + 1 + percentScale + shareFractionDigits
	scaled := new(big.Int)
	switch {
	case keep >= len(digits):
		scaled.SetString(digits+strings.Repeat("0", keep-len(digits)), decimalBase)
	case keep >= 0:
		if keep > 0 {
			scaled.SetString(digits[:keep], decimalBase)
		}
		if digits[keep]-'0' >= roundingDigit {
			scaled.Add(scaled, big.NewInt(1))
		}
	}
	text := scaled.Text(decimalBase)
	if len(text) <= shareFractionDigits {
		text = strings.Repeat("0", shareFractionDigits+1-len(text)) + text
	}
	whole, fraction := text[:len(text)-shareFractionDigits], strings.TrimRight(text[len(text)-shareFractionDigits:], "0")
	whole = group(whole)
	if fraction != "" {
		whole += "." + fraction
	}
	return sign + whole + "%"
}

func splitSign(n int64) (string, uint64) {
	if n < 0 {
		return "-", uint64(-(n + 1)) + 1
	}
	return "", uint64(n)
}

func pow10(exponent int) uint64 {
	value := uint64(1)
	for range exponent {
		value *= decimalBase
	}
	return value
}

// roundedTenths returns magnitude / 10^exponent in tenths, rounded half-expand.
func roundedTenths(magnitude uint64, exponent int) uint64 {
	divisor := pow10(exponent - 1)
	tenths, remainder := magnitude/divisor, magnitude%divisor
	if remainder >= divisor/2 {
		tenths++
	}
	return tenths
}

func group(digits string) string {
	if len(digits) <= groupSize {
		return digits
	}
	var out strings.Builder
	head := len(digits) % groupSize
	if head > 0 {
		out.WriteString(digits[:head])
	}
	for index := head; index < len(digits); index += groupSize {
		if out.Len() > 0 {
			out.WriteByte(',')
		}
		out.WriteString(digits[index : index+groupSize])
	}
	return out.String()
}

// UnavailableZoneLabel labels an unknown server timezone. Formatting then
// uses UTC explicitly and never adopts the viewer's local timezone.
const UnavailableZoneLabel = "UTC (server timezone unavailable)"

const (
	maxOffsetHours   = 23
	maxOffsetMinutes = 59
	secondsPerMinute = 60
	minutesPerHour   = 60
)

var fixedOffset = regexp.MustCompile(`^UTC([+-])(\d{2}):(\d{2})$`)

// ServerZone is a backend reporting timezone resolved like the web dashboard:
// "UTC±HH:MM" is a fixed offset, other names are IANA zones, and anything
// unresolvable is UTC labeled UnavailableZoneLabel.
type ServerZone struct {
	// Label is shown beside server times: the advertised name or UnavailableZoneLabel.
	Label    string
	location *time.Location
}

// ResolveServerZone resolves an advertised timezone. IANA names use the host
// timezone database (or an embedded one when the program links time/tzdata).
// Unlike Intl, names match case-sensitively and bare "+HH:MM" offsets are
// unavailable; backends advertise canonical IANA names or "UTC±HH:MM" only.
func ResolveServerZone(timezone string) ServerZone {
	if match := fixedOffset.FindStringSubmatch(timezone); match != nil {
		hours, _ := strconv.Atoi(match[2])
		minutes, _ := strconv.Atoi(match[3])
		if hours <= maxOffsetHours && minutes <= maxOffsetMinutes {
			offset := (hours*minutesPerHour + minutes) * secondsPerMinute
			if match[1] == "-" {
				offset = -offset
			}
			return ServerZone{Label: timezone, location: time.FixedZone(timezone, offset)}
		}
	} else if timezone != "" && timezone != "Local" {
		if location, err := time.LoadLocation(timezone); err == nil {
			return ServerZone{Label: timezone, location: location}
		}
	}
	return ServerZone{Label: UnavailableZoneLabel, location: time.UTC}
}

// In returns t on the server's wall clock.
func (z ServerZone) In(t time.Time) time.Time {
	if z.location == nil {
		return t.UTC()
	}
	return t.In(z.location)
}

// ServerZoneLabel returns the label shown for an advertised timezone.
func ServerZoneLabel(timezone string) string { return ResolveServerZone(timezone).Label }

// FormatServerDate formats Unix milliseconds as a server-calendar date, e.g. "Jan 1, 2026".
func FormatServerDate(milliseconds int64, timezone string) string {
	return shortDate(ResolveServerZone(timezone).In(time.UnixMilli(milliseconds)))
}

// FormatServerDateTime formats Unix milliseconds on the server clock with a
// 24-hour time, e.g. "Jan 1, 2026, 00:30".
func FormatServerDateTime(milliseconds int64, timezone string) string {
	t := ResolveServerZone(timezone).In(time.UnixMilli(milliseconds))
	return shortDate(t) + ", " + twoDigits(t.Hour()) + ":" + twoDigits(t.Minute())
}

func shortDate(t time.Time) string {
	return monthAbbreviation(t.Month()) + " " + strconv.Itoa(t.Day()) + ", " + strconv.Itoa(t.Year())
}

const monthAbbreviationLength = 3

func monthAbbreviation(month time.Month) string { return month.String()[:monthAbbreviationLength] }

func twoDigits(value int) string {
	if value < decimalBase {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

// TabLabel is the navigation label of a dashboard tab, e.g. "Tokens".
func TabLabel(tab Tab) string { return tabLabels[tab] }

var tabLabels = map[Tab]string{
	TabTokens: "Tokens", TabModels: "Models", TabProviders: "Providers", TabHarnesses: "Harnesses",
	TabSessions: "Sessions", TabContext: "Context", TabRepo: "Repo",
}
