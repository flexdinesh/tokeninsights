package pipeline

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

type Adapter interface {
	Harness() Harness
	Discover(context.Context, DiscoverOptions) ([]Source, error)
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
	value, _ := record[key].(map[string]interface{})
	return value
}
func stringValue(record map[string]interface{}, fallback string, names ...string) string {
	for _, name := range names {
		if value, ok := record[name].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return fallback
}

func stableHash(value string) string { return evidence.Hash([]byte(value)) }
