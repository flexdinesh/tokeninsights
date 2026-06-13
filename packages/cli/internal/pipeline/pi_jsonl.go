package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	piSessionJSONLSourceKind = "pi-session-jsonl"
	maxPiJSONLLineBytes      = 16 * 1024 * 1024
)

type piJSONLAdapter struct{}

type piJSONLSessionFile struct {
	sessionID         string
	filenameSessionID string
	hasHeader         bool
}

func (a piJSONLAdapter) Harness() Harness {
	return HarnessPi
}

func (a piJSONLAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	var roots []string
	sourceDir := strings.TrimSpace(options.SourceDir)
	if sourceDir != "" {
		harnessDir := filepath.Join(sourceDir, string(HarnessPi))
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
		roots = append(roots, filepath.Join(home, ".pi", "agent", "sessions"))
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
		rootSources, err := a.sourcesInRoot(ctx, root)
		if err != nil {
			return nil, err
		}
		sources = append(sources, rootSources...)
	}
	return sources, nil
}

func (a piJSONLAdapter) sourcesInRoot(ctx context.Context, root string) ([]Source, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var sources []Source
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		path := filepath.Join(root, entry.Name())
		if !entry.IsDir() {
			if isCandidateSource(path) {
				sources = append(sources, a.source(path, root))
			}
			continue
		}
		childEntries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, childEntry := range childEntries {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			childPath := filepath.Join(path, childEntry.Name())
			if !childEntry.IsDir() && isCandidateSource(childPath) {
				sources = append(sources, a.source(childPath, root))
			}
		}
	}
	return sources, nil
}

func (a piJSONLAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness: HarnessPi,
		ID:      stableHash("pi-source:" + filepath.ToSlash(rel)),
		Kind:    piSessionJSONLSourceKind,
		Path:    path,
	}
}

func (a piJSONLAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	session := piJSONLSessionFile{filenameSessionID: piSessionIDFromFilename(source.Path)}
	session.sessionID = session.filenameSessionID
	var facts []RawTokenFact
	var diagnostics []Diagnostic
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxPiJSONLLineBytes)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_parse_error", "skipped unparsable Pi JSONL line"))
			continue
		}
		if stringValue(record, "", "type") == "session" {
			if sessionID := stringField(record, "id"); sessionID != nil {
				session.hasHeader = true
				session.sessionID = *sessionID
				if session.filenameSessionID != "" && session.filenameSessionID != session.sessionID {
					diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_session_id_mismatch", "Pi session header id differs from filename session id"))
				}
			}
			continue
		}
		fact, rowDiagnostics, ok := a.factFromRecord(source, options, session, record)
		diagnostics = append(diagnostics, rowDiagnostics...)
		if ok {
			facts = append(facts, fact)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if !session.hasHeader && session.filenameSessionID != "" && len(facts) > 0 {
		diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_missing_session_header", "used Pi filename session id because the session header was missing"))
	}
	return facts, diagnostics, nil
}

func (a piJSONLAdapter) factFromRecord(source Source, options SyncOptions, session piJSONLSessionFile, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
	if stringValue(record, "", "type") != "message" {
		return RawTokenFact{}, nil, false
	}
	message := nested(record, "message")
	if message == nil || stringValue(message, "", "role") != "assistant" {
		return RawTokenFact{}, nil, false
	}
	usage := nested(message, "usage")
	if usage == nil {
		return RawTokenFact{}, nil, false
	}

	tokens, tokenDiagnostics, ok := piTokensFromUsage(usage)
	if !ok {
		return RawTokenFact{}, tokenDiagnostics, false
	}
	occurredAt := intField(message, "timestamp")
	if occurredAt == nil {
		occurredAt = piTimestampString(record, "timestamp")
	}
	if occurredAt == nil {
		return RawTokenFact{}, []Diagnostic{piDiagnostic("pi_jsonl_missing_time", "skipped Pi assistant token row with no usable timestamp")}, false
	}

	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, tokenDiagnostics...)
	messageID := stringField(record, "id")
	if messageID == nil {
		diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_missing_message_id", "ingested Pi assistant token row without a message id"))
	}
	if strings.TrimSpace(session.sessionID) == "" {
		return RawTokenFact{}, []Diagnostic{piDiagnostic("pi_jsonl_missing_session", "skipped Pi assistant token row with no stable session id")}, false
	}

	nowMs := options.Now.UnixMilli()
	if options.Now.IsZero() {
		nowMs = time.Now().UnixMilli()
	}
	sourceID := stableHash("pi-session:" + session.sessionID)
	return RawTokenFact{
		Harness:          HarnessPi,
		SourceID:         sourceID,
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     nowMs,
		OccurredAtMs:     occurredAt,
		SessionID:        &session.sessionID,
		MessageID:        messageID,
		Provider:         stringField(message, "provider"),
		Model:            stringField(message, "model"),
		UsageScope:       "message",
		Quality:          "exact",
		InputTokens:      tokens.input,
		OutputTokens:     tokens.output,
		ReasoningTokens:  nil,
		CacheReadTokens:  tokens.cacheRead,
		CacheWriteTokens: tokens.cacheWrite,
		TotalTokens:      tokens.total,
	}, diagnostics, true
}

type piTokenCounts struct {
	input      *int64
	output     *int64
	cacheRead  *int64
	cacheWrite *int64
	total      *int64
}

func piTokensFromUsage(usage map[string]interface{}) (piTokenCounts, []Diagnostic, bool) {
	counts := piTokenCounts{
		input:      intField(usage, "input"),
		output:     intField(usage, "output"),
		cacheRead:  intField(usage, "cacheRead"),
		cacheWrite: intField(usage, "cacheWrite"),
		total:      intField(usage, "totalTokens"),
	}
	if counts.input == nil && counts.output == nil && counts.cacheRead == nil && counts.cacheWrite == nil && counts.total == nil {
		return piTokenCounts{}, []Diagnostic{piDiagnostic("pi_jsonl_missing_tokens", "skipped Pi assistant token row with no usable token components")}, false
	}
	if hasInvalidPiToken(usage, "input", "output", "cacheRead", "cacheWrite", "totalTokens") {
		return piTokenCounts{}, []Diagnostic{piDiagnostic("pi_jsonl_invalid_tokens", "skipped Pi assistant token row with non-numeric token components")}, false
	}
	clamped := false
	clampToken(counts.input, &clamped)
	clampToken(counts.output, &clamped)
	clampToken(counts.cacheRead, &clamped)
	clampToken(counts.cacheWrite, &clamped)
	clampToken(counts.total, &clamped)
	if clamped {
		return counts, []Diagnostic{piDiagnostic("pi_jsonl_negative_tokens", "clamped negative Pi token components to zero")}, true
	}
	return counts, nil, true
}

func hasInvalidPiToken(usage map[string]interface{}, names ...string) bool {
	for _, name := range names {
		value, ok := usage[name]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) {
				return true
			}
		case string:
			if intField(usage, name) == nil {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func clampToken(value *int64, clamped *bool) {
	if value == nil || *value >= 0 {
		return
	}
	*value = 0
	*clamped = true
}

func piTimestampString(record map[string]interface{}, name string) *int64 {
	value := stringField(record, name)
	if value == nil {
		return nil
	}
	timestamp, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil
	}
	result := timestamp.UnixMilli()
	return &result
}

func piSessionIDFromFilename(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	separator := strings.Index(name, "_")
	if separator < 0 || separator == len(name)-1 {
		return ""
	}
	return strings.TrimSpace(name[separator+1:])
}

func piDiagnostic(code string, message string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessPi,
		Severity: "warning",
		Code:     code,
		Message:  message,
	}
}
