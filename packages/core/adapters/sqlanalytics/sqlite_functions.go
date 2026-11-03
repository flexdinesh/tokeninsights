package sqlanalytics

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"modernc.org/sqlite"
)

// Locations are immutable. Reporting zones come from server configuration, so
// loading IANA files once avoids filesystem work for each SQL row.
var locations sync.Map

func location(zone string) (*time.Location, error) {
	if cached, ok := locations.Load(zone); ok {
		return cached.(*time.Location), nil
	}
	loc, err := loadLocation(zone)
	if err == nil {
		locations.Store(zone, loc)
	}
	return loc, err
}

func loadLocation(zone string) (*time.Location, error) {
	if strings.HasPrefix(zone, "@") {
		offset, err := strconv.Atoi(zone[1:])
		if err != nil {
			return nil, err
		}
		return time.FixedZone(zone, offset), nil
	}
	return time.LoadLocation(zone)
}

func calendarBucket(ms int64, zone, bucket string) (string, error) {
	loc, err := location(zone)
	if err != nil {
		return "", err
	}
	at := time.UnixMilli(ms).In(loc)
	switch bucket {
	case "day":
		return at.Format("2006-01-02"), nil
	case "week":
		day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, loc)
		return day.AddDate(0, 0, -(int(day.Weekday())+6)%7).Format("2006-01-02"), nil
	case "month":
		return at.Format("2006-01"), nil
	case "year":
		return at.Format("2006"), nil
	default:
		return "", fmt.Errorf("invalid bucket")
	}
}

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("ti_bucket", 3, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		ms, ok := args[0].(int64)
		if !ok {
			return nil, fmt.Errorf("invalid timestamp")
		}
		zone, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("invalid timezone")
		}
		bucket, ok := args[2].(string)
		if !ok {
			return nil, fmt.Errorf("invalid bucket")
		}
		return calendarBucket(ms, zone, bucket)
	})
	for _, spec := range []struct {
		name string
		json bool
	}{{"ti_dimensions", false}, {"ti_directories", true}} {
		sqlite.MustRegisterFunction(spec.name, &sqlite.FunctionImpl{NArgs: 1, Deterministic: true, MakeAggregate: func(sqlite.FunctionContext) (sqlite.AggregateFunction, error) {
			return &distinctStrings{values: map[string]int{}, json: spec.json}, nil
		}})
	}
}

type distinctStrings struct {
	values map[string]int
	json   bool
}

func (a *distinctStrings) Step(_ *sqlite.FunctionContext, args []driver.Value) error {
	if v, ok := args[0].(string); ok {
		a.values[v]++
	}
	return nil
}
func (a *distinctStrings) WindowInverse(_ *sqlite.FunctionContext, args []driver.Value) error {
	if v, ok := args[0].(string); ok {
		a.values[v]--
		if a.values[v] == 0 {
			delete(a.values, v)
		}
	}
	return nil
}
func (a *distinctStrings) WindowValue(*sqlite.FunctionContext) (driver.Value, error) {
	values := make([]string, 0, len(a.values))
	for v := range a.values {
		values = append(values, v)
	}
	slices.Sort(values)
	if a.json {
		body, err := json.Marshal(values)
		return string(body), err
	}
	return strings.Join(values, ", "), nil
}
func (*distinctStrings) Final(*sqlite.FunctionContext) {}
