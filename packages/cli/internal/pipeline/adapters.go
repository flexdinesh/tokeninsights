package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

type Adapter interface {
	Harness() Harness
	Discover(context.Context, DiscoverOptions) ([]Source, error)
	Parse(context.Context, Source, SyncOptions) ([]RawTokenFact, []Diagnostic, error)
}

func Adapters() []Adapter {
	return []Adapter{
		opencodeSQLiteAdapter{},
		piJSONLAdapter{},
		&codexJSONLAdapter{},
		claudeCodeJSONLAdapter{},
	}
}

func AdapterFor(h Harness) (Adapter, bool) {
	for _, adapter := range Adapters() {
		if adapter.Harness() == h {
			return adapter, true
		}
	}
	return nil, false
}

func isCandidateSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jsonl", ".ndjson":
		return true
	default:
		return false
	}
}

func nested(record map[string]interface{}, key string) map[string]interface{} {
	value, ok := record[key]
	if !ok {
		return nil
	}
	nestedValue, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return nestedValue
}

func stringField(record map[string]interface{}, names ...string) *string {
	for _, name := range names {
		value, ok := record[name]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			trimmed := strings.TrimSpace(text)
			return &trimmed
		}
	}
	return nil
}

func stringValue(record map[string]interface{}, fallback string, names ...string) string {
	value := stringField(record, names...)
	if value == nil {
		return fallback
	}
	return *value
}

func intField(record map[string]interface{}, names ...string) *int64 {
	for _, name := range names {
		value, ok := record[name]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case json.Number:
			result, err := typed.Int64()
			if err == nil {
				return &result
			}
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed >= math.MaxInt64 || typed < math.MinInt64 {
				return nil
			}
			result := int64(typed)
			return &result
		case int64:
			result := typed
			return &result
		case string:
			result, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
			if err == nil {
				return &result
			}
		}
		return nil
	}
	return nil
}

func decodeJSONRecord(line string, record *map[string]interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	if err := decoder.Decode(record); err != nil {
		return err
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("multiple JSON values")
	}
	return nil
}

func tokenComponentSum(values ...*int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value == nil {
			continue
		}
		if *value < 0 {
			return 0, false
		}
		if *value > math.MaxInt64-total {
			return 0, false
		}
		total += *value
	}
	return total, true
}

func hasInvalidIntegerField(record map[string]interface{}, names ...string) bool {
	for _, name := range names {
		value, ok := record[name]
		if ok && value != nil && intField(record, name) == nil {
			return true
		}
	}
	return false
}

func stableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
