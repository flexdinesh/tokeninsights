package pipeline

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Adapter interface {
	Harness() Harness
	Discover(context.Context, DiscoverOptions) ([]Source, error)
	Parse(context.Context, Source, SyncOptions) ([]RawTokenFact, []Diagnostic, error)
}

func Adapters() []Adapter {
	return []Adapter{
		jsonAdapter{harness: HarnessOpenCode, defaultDirs: []string{".local/share/opencode", ".config/opencode", ".cache/opencode"}},
		jsonAdapter{harness: HarnessPi, defaultDirs: []string{".pi"}},
		jsonAdapter{harness: HarnessCodex, defaultDirs: []string{".codex", ".local/share/codex"}},
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

type jsonAdapter struct {
	harness     Harness
	defaultDirs []string
}

func (a jsonAdapter) Harness() Harness {
	return a.harness
}

func (a jsonAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	var roots []string
	sourceDir := strings.TrimSpace(options.SourceDir)
	if sourceDir != "" {
		harnessDir := filepath.Join(sourceDir, string(a.harness))
		if info, err := os.Stat(harnessDir); err == nil && info.IsDir() {
			roots = append(roots, harnessDir)
		} else if options.HarnessSubdirOnly {
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			return nil, nil
		} else {
			roots = append(roots, sourceDir)
		}
	} else {
		home := strings.TrimSpace(os.Getenv("HOME"))
		if home == "" {
			return nil, nil
		}
		for _, dir := range a.defaultDirs {
			roots = append(roots, filepath.Join(home, dir))
		}
	}

	var sources []Source
	for _, root := range roots {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			if isCandidateSource(root) {
				sources = append(sources, a.source(root, filepath.Dir(root)))
			}
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir() {
				name := entry.Name()
				if name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if isCandidateSource(path) {
				sources = append(sources, a.source(path, root))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return sources, nil
}

func (a jsonAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness: a.harness,
		ID:      stableHash(string(a.harness) + ":" + filepath.ToSlash(rel)),
		Kind:    "local-json",
		Path:    path,
	}
}

func (a jsonAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	var facts []RawTokenFact
	var diagnostics []Diagnostic
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Harness:  a.harness,
				Severity: "warning",
				Code:     "source_parse_error",
				Message:  "skipped unparsable JSON line",
			})
			continue
		}
		fact, ok := a.factFromRecord(source, options, record, lineNumber)
		if !ok {
			continue
		}
		facts = append(facts, fact)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return facts, diagnostics, nil
}

func isCandidateSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jsonl", ".ndjson":
		return true
	default:
		return false
	}
}

func (a jsonAdapter) factFromRecord(source Source, options SyncOptions, record map[string]interface{}, lineNumber int) (RawTokenFact, bool) {
	usage := nested(record, "usage")
	if usage == nil {
		usage = record
	}

	input := intField(usage, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens", "input")
	output := intField(usage, "output_tokens", "outputTokens", "completion_tokens", "completionTokens", "output")
	reasoning := intField(usage, "reasoning_tokens", "reasoningTokens")
	cacheRead := intField(usage, "cache_read_tokens", "cacheReadTokens", "cache_read")
	cacheWrite := intField(usage, "cache_write_tokens", "cacheWriteTokens", "cache_write")
	total := intField(usage, "total_tokens", "totalTokens", "total")

	if input == nil && output == nil && reasoning == nil && cacheRead == nil && cacheWrite == nil && total == nil {
		return RawTokenFact{}, false
	}

	nowMs := options.Now.UnixMilli()
	if options.Now.IsZero() {
		nowMs = time.Now().UnixMilli()
	}
	occurredAt := intField(record, "recorded_at_ms", "recordedAtMs", "occurred_at_ms", "occurredAtMs", "timestamp_ms", "timestampMs")
	if occurredAt == nil {
		occurredAt = intField(usage, "recorded_at_ms", "recordedAtMs", "occurred_at_ms", "occurredAtMs")
	}
	metadata := fmt.Sprintf(`{"line":%d}`, lineNumber)

	return RawTokenFact{
		Harness:          a.harness,
		SourceID:         source.ID,
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     nowMs,
		OccurredAtMs:     occurredAt,
		SessionID:        stringField(record, "session_id", "sessionID", "sessionId", "session"),
		MessageID:        stringField(record, "message_id", "messageID", "messageId", "turn_id", "turnId"),
		Provider:         stringField(record, "provider"),
		Model:            stringField(record, "model"),
		UsageScope:       stringValue(record, "message", "usage_scope", "usageScope"),
		Quality:          stringValue(record, "exact", "quality"),
		InputTokens:      input,
		OutputTokens:     output,
		ReasoningTokens:  reasoning,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
		TotalTokens:      total,
		MetadataJSON:     &metadata,
	}, true
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
