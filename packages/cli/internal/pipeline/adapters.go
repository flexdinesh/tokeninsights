package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
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
		codexJSONLAdapter{},
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
		case float64:
			result := int64(typed)
			return &result
		case int64:
			result := typed
			return &result
		case string:
			var result int64
			if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &result); err == nil {
				return &result
			}
		}
	}
	return nil
}

func stableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
