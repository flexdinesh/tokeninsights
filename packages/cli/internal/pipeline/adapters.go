package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
	"io"
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
	return processor.Nested(record, key)
}
func stringField(record map[string]interface{}, names ...string) *string {
	return processor.StringField(record, names...)
}
func stringValue(record map[string]interface{}, fallback string, names ...string) string {
	return processor.StringValue(record, fallback, names...)
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

func tokenComponentSum(values ...*int64) (int64, bool) { return processor.TokenComponentSum(values...) }
func stableHash(value string) string                   { return processor.StableHash(value) }
