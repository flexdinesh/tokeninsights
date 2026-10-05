package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const (
	claudeCodeJSONLSourceKind = "claude-code-session-jsonl"
)

type claudeCodeJSONLAdapter struct{}

func (a claudeCodeJSONLAdapter) Harness() Harness {
	return HarnessClaudeCode
}

func (a claudeCodeJSONLAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	var roots []string
	if options.Sources != nil {
		var err error
		roots, err = options.Sources.discoveryRoots(HarnessClaudeCode, options.HarnessSubdirOnly)
		if err != nil {
			return nil, err
		}
	} else {
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
	recordSourceParse(ctx)

	sessionID := claudeCodeSessionIDFromFilename(source.Path)
	var facts []RawTokenFact
	var diagnostics []Diagnostic
	mergedFactIndexes := map[string]int{}
	scanner := newSourceJSONLReader(ctx, file, source, options)
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
		if options.sourceSnapshot != nil {
			options.sourceSnapshot.observeRecord(record)
		}
		fact, rowDiagnostics, ok := a.factFromRecord(ctx, source, options, sessionID, record)
		diagnostics = append(diagnostics, rowDiagnostics...)
		if ok {
			mergeKey := claudeCodeStreamingMergeKey(fact)
			if mergeKey == "" {
				facts = append(facts, fact)
				continue
			}
			if index, exists := mergedFactIndexes[mergeKey]; exists {
				conflict, err := mergeClaudeCodeStreamingFact(&facts[index], fact)
				if err != nil {
					return nil, diagnostics, err
				}
				if conflict {
					diagnostics = append(diagnostics, claudeCodeDiagnostic("location_conflict", "conflicting location evidence in Claude Code streaming copies; affected grouping is unknown", "warning"))
				}
				continue
			}
			mergedFactIndexes[mergeKey] = len(facts)
			facts = append(facts, fact)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if scanner.deferred {
		diagnostics = append(diagnostics, Diagnostic{Harness: HarnessClaudeCode, Severity: "info", Code: "jsonl_incomplete_tail", Message: "unfinished final JSONL record deferred until next sync"})
	}
	finalFacts := make([]RawTokenFact, 0, len(facts))
	for index := range facts {
		factDiagnostics, ok := finalizeClaudeCodeFact(&facts[index])
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

func (a claudeCodeJSONLAdapter) factFromRecord(ctx context.Context, source Source, options SyncOptions, fallbackSessionID string, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
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
	if !publication.ValidTimestampMs(*occurredAt) {
		tokenDiagnostics = append(tokenDiagnostics, claudeCodeDiagnostic("claude_code_jsonl_invalid_time", "retained raw Claude Code assistant token row with a timestamp outside the supported range", "warning"))
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
	location, _ := resolveFactLocation(ctx, options, stringValue(record, "", "cwd"), "", "")
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
		Location:         location,
		MetadataJSON:     claudeCodeIdentityMetadata(record),
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

func claudeCodeIdentityMetadata(record map[string]interface{}) *string {
	sessionSource := "filename"
	if stringField(record, "sessionId", "session_id") != nil {
		sessionSource = "native"
	}
	return sourceIdentityJSON(sessionSource, stringField(record, "requestId", "request_id"))
}

func claudeCodeRequestID(metadata *string) string {
	if metadata == nil {
		return ""
	}
	var identity sourceIdentityMetadata
	if json.Unmarshal([]byte(*metadata), &identity) != nil {
		return ""
	}
	return stringValueOrEmpty(identity.RequestID)
}

func claudeCodeStreamingMergeKey(fact RawTokenFact) string {
	if fact.MessageID == nil || sourceSessionIdentity(fact.MetadataJSON) != "native" ||
		fact.OccurredAtMs == nil || !publication.ValidTimestampMs(*fact.OccurredAtMs) {
		return ""
	}
	return nativeTupleHash(stringValueOrEmpty(fact.SessionID), *fact.MessageID, claudeCodeRequestID(fact.MetadataJSON))
}

// Claude records for one native request are snapshots. Source time orders them;
// merging counters independently could synthesize usage never present in source.
func mergeClaudeCodeStreamingFact(existing *RawTokenFact, next RawTokenFact) (bool, error) {
	if claudeCodeRequestID(existing.MetadataJSON) == "" && *existing.OccurredAtMs != *next.OccurredAtMs {
		return false, fmt.Errorf("claude code message has no native request revision evidence")
	}
	if *existing.OccurredAtMs == *next.OccurredAtMs && !claudeCodeSameUsage(*existing, next) {
		return false, fmt.Errorf("claude code native request has conflicting usage at the same source timestamp")
	}
	location, conflicts, _, conflict := mergeLocations(existing.Location, next.Location, existing.locationConflicts)
	if *next.OccurredAtMs > *existing.OccurredAtMs {
		*existing = next
	}
	existing.Location = location
	existing.locationConflicts = conflicts
	return conflict, nil
}

func claudeCodeSameUsage(left RawTokenFact, right RawTokenFact) bool {
	leftUsage, leftValid := claudeCodeCanonicalUsage(left)
	rightUsage, rightValid := claudeCodeCanonicalUsage(right)
	return leftValid && rightValid && sameCanonicalUsage(leftUsage, rightUsage)
}

// Only unfinalized parser snapshots use this projection. Finalization mutates
// the clone's pointer fields, leaving the original inclusive output untouched.
// Retained raw facts have already split reasoning and must never pass here.
func claudeCodeCanonicalUsage(fact RawTokenFact) (canonicalTokenValues, bool) {
	if _, ok := finalizeClaudeCodeFact(&fact); !ok {
		return canonicalTokenValues{}, false
	}
	row := rawTokenRow{
		Harness: fact.Harness, UsageScope: fact.UsageScope,
		Provider: sql.NullString{String: stringValueOrEmpty(fact.Provider), Valid: fact.Provider != nil},
		Model:    sql.NullString{String: stringValueOrEmpty(fact.Model), Valid: fact.Model != nil},
	}
	provider, providerSource := canonicalProvider(row)
	total, _ := tokenComponentSum(fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens)
	return canonicalTokenValues{
		Provider: provider, ProviderSource: providerSource, Model: canonicalModel(row.Model, provider),
		Quality: fact.Quality, Countable: countable(row),
		InputTokens: intValueOrZero(fact.InputTokens), OutputTokens: intValueOrZero(fact.OutputTokens),
		ReasoningTokens: intValueOrZero(fact.ReasoningTokens), CacheReadTokens: intValueOrZero(fact.CacheReadTokens),
		CacheWriteTokens: intValueOrZero(fact.CacheWriteTokens), TotalTokens: total,
	}, true
}

func finalizeClaudeCodeFact(fact *RawTokenFact) ([]Diagnostic, bool) {
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
	fact.DedupeKey = claudeCodeFactDedupeKey(*fact.SessionID, fact.MessageID, claudeCodeRequestID(fact.MetadataJSON), fact.OccurredAtMs, claudeCodeTokenCounts{
		input: fact.InputTokens, output: fact.OutputTokens, reasoning: fact.ReasoningTokens,
		cacheRead: fact.CacheReadTokens, cacheWrite: fact.CacheWriteTokens, total: fact.TotalTokens,
	})
	fact.DedupeKey = nativeTupleHash(fact.DedupeKey, sourceSessionIdentity(fact.MetadataJSON))
	return diagnostics, true
}

func claudeCodeFactDedupeKey(sessionID string, messageID *string, requestID string, occurredAt *int64, tokens claudeCodeTokenCounts) string {
	parts := []string{
		"claude-code-token",
		sessionID,
		stringValueOrEmpty(messageID),
		requestID,
		int64ValueOrZero(occurredAt),
		int64ValueOrZero(tokens.input),
		int64ValueOrZero(tokens.output),
		int64ValueOrZero(tokens.reasoning),
		int64ValueOrZero(tokens.cacheRead),
		int64ValueOrZero(tokens.cacheWrite),
		int64ValueOrZero(tokens.total),
	}
	return nativeTupleHash(parts...)
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
