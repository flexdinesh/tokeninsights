package pipeline

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	claudeCodeJSONLSourceKind   = "claude-code-session-jsonl"
	maxClaudeCodeJSONLLineBytes = 16 * 1024 * 1024
)

type claudeCodeJSONLAdapter struct{}

func (a claudeCodeJSONLAdapter) Harness() Harness {
	return HarnessClaudeCode
}

func (a claudeCodeJSONLAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	var roots []string
	sourceDir := strings.TrimSpace(options.SourceDir)
	if sourceDir != "" {
		harnessDir := filepath.Join(sourceDir, string(HarnessClaudeCode))
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
		root := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
		if root == "" {
			home := strings.TrimSpace(os.Getenv("HOME"))
			if home == "" {
				return nil, nil
			}
			root = filepath.Join(home, ".claude")
		}
		roots = append(roots, filepath.Join(root, "projects"))
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
	sort.Slice(sources, func(i int, j int) bool {
		return sources[i].Path < sources[j].Path
	})
	return sources, nil
}

func (a claudeCodeJSONLAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness: HarnessClaudeCode,
		ID:      stableHash("claude-code-session-file:" + filepath.ToSlash(rel)),
		Kind:    claudeCodeJSONLSourceKind,
		Path:    path,
	}
}

func (a claudeCodeJSONLAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()

	sessionID := claudeCodeSessionIDFromFilename(source.Path)
	var facts []RawTokenFact
	var requestIDs []*string
	var diagnostics []Diagnostic
	mergedFactIndexes := map[string]int{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxClaudeCodeJSONLLineBytes)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]interface{}
		if err := decodeJSONRecord(line, &record); err != nil {
			diagnostics = append(diagnostics, claudeCodeDiagnostic("claude_code_jsonl_parse_error", "skipped unparsable Claude Code JSONL line", "warning"))
			continue
		}
		fact, rowDiagnostics, ok := a.factFromRecord(source, options, sessionID, record)
		diagnostics = append(diagnostics, rowDiagnostics...)
		if ok {
			mergeKey := claudeCodeStreamingMergeKey(record)
			if mergeKey == "" {
				facts = append(facts, fact)
				requestIDs = append(requestIDs, stringField(record, "requestId", "request_id"))
				continue
			}
			if index, exists := mergedFactIndexes[mergeKey]; exists {
				mergeClaudeCodeStreamingFact(&facts[index], fact)
				continue
			}
			mergedFactIndexes[mergeKey] = len(facts)
			facts = append(facts, fact)
			requestIDs = append(requestIDs, stringField(record, "requestId", "request_id"))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	finalFacts := make([]RawTokenFact, 0, len(facts))
	for index := range facts {
		factDiagnostics, ok := finalizeClaudeCodeFact(&facts[index], requestIDs[index])
		diagnostics = append(diagnostics, factDiagnostics...)
		if ok {
			finalFacts = append(finalFacts, facts[index])
		}
	}
	facts = finalFacts
	if len(facts) > 0 {
		diagnostics = append(diagnostics, claudeCodeDiagnostic("claude_code_jsonl_transcript_derived", "Claude Code token usage was derived from local JSONL transcript data", "info"))
	}
	return facts, diagnostics, nil
}

func (a claudeCodeJSONLAdapter) factFromRecord(source Source, options SyncOptions, fallbackSessionID string, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
	if stringValue(record, "", "type") != "assistant" {
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
	tokens, tokenDiagnostics, ok := claudeCodeTokensFromUsage(usage)
	if !ok {
		return RawTokenFact{}, tokenDiagnostics, false
	}
	occurredAt := claudeCodeTimestampString(record, "timestamp")
	if occurredAt == nil {
		return RawTokenFact{}, []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_missing_time", "skipped Claude Code assistant token row with no usable timestamp", "warning")}, false
	}
	sessionID := fallbackSessionID
	if sourceSessionID := stringField(record, "sessionId", "session_id"); sourceSessionID != nil {
		sessionID = *sourceSessionID
	}
	if strings.TrimSpace(sessionID) == "" {
		return RawTokenFact{}, []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_missing_session", "skipped Claude Code assistant token row with no stable session id", "warning")}, false
	}

	nowMs := syncNowMs(options.Now)
	messageID := stringField(message, "id")
	if messageID == nil {
		messageID = stringField(record, "uuid")
	}
	return RawTokenFact{
		Harness:          HarnessClaudeCode,
		SourceID:         stableHash("claude-code-session:" + sessionID),
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     nowMs,
		OccurredAtMs:     occurredAt,
		SessionID:        &sessionID,
		MessageID:        messageID,
		Provider:         stringField(message, "provider", "provider_id", "providerID"),
		Model:            stringField(message, "model", "model_id", "modelID"),
		UsageScope:       "message",
		Quality:          "derived",
		InputTokens:      tokens.input,
		OutputTokens:     tokens.output,
		ReasoningTokens:  tokens.reasoning,
		CacheReadTokens:  tokens.cacheRead,
		CacheWriteTokens: tokens.cacheWrite,
		TotalTokens:      tokens.total,
	}, tokenDiagnostics, true
}

type claudeCodeTokenCounts struct {
	input      *int64
	output     *int64
	reasoning  *int64
	cacheRead  *int64
	cacheWrite *int64
	total      *int64
}

func claudeCodeTokensFromUsage(usage map[string]interface{}) (claudeCodeTokenCounts, []Diagnostic, bool) {
	outputDetails := nested(usage, "output_tokens_details")
	if hasInvalidIntegerField(usage, "input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "total_tokens") ||
		hasInvalidIntegerField(outputDetails, "thinking_tokens", "reasoning_tokens") {
		return claudeCodeTokenCounts{}, []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_invalid_tokens", "skipped Claude Code assistant token row with non-integer token components", "warning")}, false
	}
	counts := claudeCodeTokenCounts{
		input:      intField(usage, "input_tokens"),
		output:     intField(usage, "output_tokens"),
		reasoning:  intField(outputDetails, "thinking_tokens", "reasoning_tokens"),
		cacheRead:  intField(usage, "cache_read_input_tokens"),
		cacheWrite: intField(usage, "cache_creation_input_tokens"),
		total:      intField(usage, "total_tokens"),
	}
	if counts.input == nil && counts.output == nil && counts.reasoning == nil && counts.cacheRead == nil && counts.cacheWrite == nil && counts.total == nil {
		return claudeCodeTokenCounts{}, []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_missing_tokens", "skipped Claude Code assistant token row with no usable token components", "warning")}, false
	}
	clamped := false
	clampToken(counts.input, &clamped)
	clampToken(counts.output, &clamped)
	clampToken(counts.reasoning, &clamped)
	clampToken(counts.cacheRead, &clamped)
	clampToken(counts.cacheWrite, &clamped)
	clampToken(counts.total, &clamped)
	if clamped {
		return counts, []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_negative_tokens", "clamped negative Claude Code token components to zero", "warning")}, true
	}
	return counts, nil, true
}

func claudeCodeStreamingMergeKey(record map[string]interface{}) string {
	message := nested(record, "message")
	if message == nil {
		return ""
	}
	messageID := stringField(message, "id")
	if messageID == nil {
		return ""
	}
	requestID := stringField(record, "requestId", "request_id")
	if requestID == nil {
		return *messageID
	}
	return *messageID + "|" + *requestID
}

func mergeClaudeCodeStreamingFact(existing *RawTokenFact, next RawTokenFact) {
	existing.InputTokens = maxIntPointer(existing.InputTokens, next.InputTokens)
	existing.OutputTokens = maxIntPointer(existing.OutputTokens, next.OutputTokens)
	existing.ReasoningTokens = maxIntPointer(existing.ReasoningTokens, next.ReasoningTokens)
	existing.CacheReadTokens = maxIntPointer(existing.CacheReadTokens, next.CacheReadTokens)
	existing.CacheWriteTokens = maxIntPointer(existing.CacheWriteTokens, next.CacheWriteTokens)
	existing.TotalTokens = maxIntPointer(existing.TotalTokens, next.TotalTokens)
	if next.OccurredAtMs != nil && (existing.OccurredAtMs == nil || *next.OccurredAtMs > *existing.OccurredAtMs) {
		existing.OccurredAtMs = next.OccurredAtMs
	}
	if existing.Provider == nil {
		existing.Provider = next.Provider
	}
	if existing.Model == nil {
		existing.Model = next.Model
	}
}

func finalizeClaudeCodeFact(fact *RawTokenFact, requestID *string) ([]Diagnostic, bool) {
	var diagnostics []Diagnostic
	if fact.ReasoningTokens != nil {
		if fact.OutputTokens == nil || *fact.ReasoningTokens > *fact.OutputTokens {
			fact.ReasoningTokens = nil
			diagnostics = append(diagnostics, claudeCodeDiagnostic("claude_code_jsonl_invalid_reasoning", "ignored Claude Code reasoning tokens that exceeded inclusive output tokens", "warning"))
		} else {
			nonReasoningOutput := *fact.OutputTokens - *fact.ReasoningTokens
			fact.OutputTokens = &nonReasoningOutput
		}
	}
	componentTotal, ok := tokenComponentSum(fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens)
	if !ok {
		return []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_invalid_tokens", "skipped Claude Code assistant token row whose token total exceeds the supported range", "warning")}, false
	}
	if fact.TotalTokens != nil && *fact.TotalTokens != componentTotal {
		fact.TotalTokens = nil
		diagnostics = append(diagnostics, claudeCodeDiagnostic("claude_code_jsonl_inconsistent_total", "ignored Claude Code total_tokens that did not equal the token component sum", "warning"))
	}
	fact.DedupeKey = claudeCodeFactDedupeKey(*fact.SessionID, fact.MessageID, requestID, fact.OccurredAtMs, claudeCodeTokenCounts{
		input: fact.InputTokens, output: fact.OutputTokens, reasoning: fact.ReasoningTokens,
		cacheRead: fact.CacheReadTokens, cacheWrite: fact.CacheWriteTokens, total: fact.TotalTokens,
	})
	return diagnostics, true
}

func maxIntPointer(left *int64, right *int64) *int64 {
	if left == nil {
		return right
	}
	if right == nil || *left >= *right {
		return left
	}
	return right
}

func claudeCodeFactDedupeKey(sessionID string, messageID *string, requestID *string, occurredAt *int64, tokens claudeCodeTokenCounts) string {
	parts := []string{
		"claude-code-token",
		sessionID,
		stringValueOrEmpty(messageID),
		stringValueOrEmpty(requestID),
		int64ValueOrZero(occurredAt),
		int64ValueOrZero(tokens.input),
		int64ValueOrZero(tokens.output),
		int64ValueOrZero(tokens.reasoning),
		int64ValueOrZero(tokens.cacheRead),
		int64ValueOrZero(tokens.cacheWrite),
		int64ValueOrZero(tokens.total),
	}
	return stableHash(strings.Join(parts, "|"))
}

func int64ValueOrZero(value *int64) string {
	if value == nil {
		return "0"
	}
	return strconv.FormatInt(*value, 10)
}

func claudeCodeTimestampString(record map[string]interface{}, name string) *int64 {
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

func claudeCodeSessionIDFromFilename(path string) string {
	return strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

func claudeCodeDiagnostic(code string, message string, severity string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessClaudeCode,
		Severity: severity,
		Code:     code,
		Message:  message,
	}
}
