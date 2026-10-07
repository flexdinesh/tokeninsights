package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type codexParser struct{}
type codexJSONLState struct {
	sessionID      string
	sawSessionMeta bool
	provider       *string
	model          *string
	turnID         *string
	pending        []codexPendingFact
}

type codexTokenCounts struct {
	input     *int64
	output    *int64
	reasoning *int64
	cacheRead *int64
	total     *int64
}

type codexPendingFact struct {
	candidate   codexCandidate
	diagnostics []Diagnostic
}

func (state *codexJSONLState) applySessionMeta(record map[string]interface{}) []Diagnostic {
	payload := Nested(record, "payload")
	if payload == nil {
		return nil
	}
	sessionID := StringField(payload, "id")
	if sessionID == nil {
		return nil
	}
	var diagnostics []Diagnostic
	if !state.sawSessionMeta {
		state.sawSessionMeta = true
		state.sessionID = *sessionID
		if state.provider == nil {
			state.provider = StringField(payload, "model_provider")
		}
		return diagnostics
	} else if state.sessionID != *sessionID {
		diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_multiple_session_meta", "Codex session file contains multiple session metadata ids"))
		return diagnostics
	}
	if state.provider == nil {
		state.provider = StringField(payload, "model_provider")
	}
	return diagnostics
}

func (state *codexJSONLState) applyTurnContext(record map[string]interface{}) {
	payload := Nested(record, "payload")
	if payload == nil {
		return
	}
	if turnID := StringField(payload, "turn_id"); turnID != nil {
		state.turnID = turnID
	}
	if model := StringField(payload, "model"); model != nil {
		state.model = model
	}
}

func (state *codexJSONLState) flushPending(resolved bool) ([]codexCandidate, []Diagnostic) {
	if len(state.pending) == 0 {
		return nil, nil
	}
	pending := state.pending
	state.pending = nil
	facts := make([]codexCandidate, 0, len(pending))
	var diagnostics []Diagnostic
	for _, item := range pending {
		diagnostics = append(diagnostics, item.diagnostics...)
		candidate := item.candidate
		if resolved {
			candidate.fact.Model = state.model
			if candidate.turnID == nil {
				candidate.turnID = state.turnID
			}
			candidate.setMessageID()
		} else {
			diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_missing_model", "ingested Codex token-count row before model state was resolved"))
		}
		facts = append(facts, candidate)
	}
	return facts, diagnostics
}

func (a *codexParser) factFromEvent(ctx context.Context, source NativeSource, options ParseOptions, state *codexJSONLState, record map[string]interface{}, lineNumber int) (codexCandidate, []Diagnostic, bool) {
	payload := Nested(record, "payload")
	if payload == nil {
		return codexCandidate{}, nil, false
	}
	if StringValue(payload, "", "type") == "task_started" {
		if turnID := StringField(payload, "turn_id"); turnID != nil {
			state.turnID = turnID
		}
		return codexCandidate{}, nil, false
	}
	if StringValue(payload, "", "type") != "token_count" {
		return codexCandidate{}, nil, false
	}
	if strings.TrimSpace(state.sessionID) == "" {
		return codexCandidate{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_session", "skipped Codex token-count row with no stable session id")}, false
	}
	occurredAt := codexTimestampString(record, "timestamp")
	if occurredAt == nil {
		return codexCandidate{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_time", "skipped Codex token-count row with no usable timestamp")}, false
	}
	info := Nested(payload, "info")
	if info == nil {
		return codexCandidate{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_tokens", "skipped Codex token-count row with no usable token components")}, false
	}
	lastUsage := Nested(info, "last_token_usage")
	if lastUsage == nil {
		return codexCandidate{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_tokens", "skipped Codex token-count row with no usable last token usage")}, false
	}
	tokens, tokenDiagnostics, ok := codexTokensFromUsage(lastUsage)
	if !ok {
		return codexCandidate{}, tokenDiagnostics, false
	}

	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, tokenDiagnostics...)
	totalUsage := Nested(info, "total_token_usage")
	var cumulativeCounts *codexTokenCounts
	if totalUsage != nil {
		cumulative, cumulativeDiagnostics, cumulativeOK := codexTokensFromUsage(totalUsage)
		diagnostics = append(diagnostics, cumulativeDiagnostics...)
		if !cumulativeOK {
			return codexCandidate{}, diagnostics, false
		}
		cumulativeCounts = &cumulative
	} else {
		diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_last_without_total", "ingested Codex token-count row without cumulative duplicate protection"))
	}
	nowMs := options.ObservedAtMs
	sourceID := StableHash("codex-session:" + state.sessionID)
	sessionID := state.sessionID
	fact := RawTokenFact{
		Harness:          HarnessCodex,
		SourceID:         sourceID,
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     nowMs,
		OccurredAtMs:     occurredAt,
		SessionID:        &sessionID,
		Provider:         state.provider,
		Model:            state.model,
		UsageScope:       "message",
		Quality:          "exact",
		InputTokens:      tokens.input,
		OutputTokens:     tokens.output,
		ReasoningTokens:  tokens.reasoning,
		CacheReadTokens:  tokens.cacheRead,
		CacheWriteTokens: nil,
		TotalTokens:      nil,
	}
	last, lastValid := CodexSnapshotFromUsage(lastUsage)
	total, totalValid := CodexSnapshotFromUsage(totalUsage)
	candidate := codexCandidate{
		fact: fact, turnID: state.turnID, line: lineNumber,
		snapshot:    CodexSnapshot{Last: last, Total: total},
		replayValid: lastValid && totalValid, cumulative: cumulativeCounts,
	}
	candidate.setMessageID()
	if state.model == nil {
		state.pending = append(state.pending, codexPendingFact{candidate: candidate, diagnostics: diagnostics})
		return codexCandidate{}, nil, false
	}
	return candidate, diagnostics, true
}

func codexTokensFromUsage(usage map[string]interface{}) (codexTokenCounts, []Diagnostic, bool) {
	if hasInvalidCodexToken(usage, "input_tokens", "cached_input_tokens", "cache_read_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens") {
		return codexTokenCounts{}, []Diagnostic{codexDiagnostic("codex_jsonl_invalid_tokens", "skipped Codex token-count row with non-numeric token components")}, false
	}
	rawInput := CodexIntField(usage, "input_tokens")
	cacheRead := largerInt(CodexIntField(usage, "cached_input_tokens"), CodexIntField(usage, "cache_read_input_tokens"))
	counts := codexTokenCounts{
		input:     rawInput,
		output:    CodexIntField(usage, "output_tokens"),
		reasoning: CodexIntField(usage, "reasoning_output_tokens"),
		cacheRead: cacheRead,
		total:     CodexIntField(usage, "total_tokens"),
	}
	if counts.input == nil && counts.output == nil && counts.reasoning == nil && counts.cacheRead == nil && counts.total == nil {
		return codexTokenCounts{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_tokens", "skipped Codex token-count row with no usable token components")}, false
	}
	clamped := false
	clampToken(counts.input, &clamped)
	clampToken(counts.output, &clamped)
	clampToken(counts.reasoning, &clamped)
	clampToken(counts.cacheRead, &clamped)
	clampToken(counts.total, &clamped)
	if counts.reasoning != nil {
		if counts.output == nil || *counts.reasoning > *counts.output {
			return codexTokenCounts{}, []Diagnostic{codexDiagnostic("codex_jsonl_invalid_reasoning", "skipped Codex token-count row whose reasoning tokens exceeded inclusive output tokens")}, false
		}
		*counts.output -= *counts.reasoning
	}
	if counts.input != nil && counts.cacheRead != nil {
		*counts.input -= *counts.cacheRead
		clampToken(counts.input, &clamped)
	}
	if _, ok := TokenComponentSum(counts.input, counts.output, counts.reasoning, counts.cacheRead); !ok {
		return codexTokenCounts{}, []Diagnostic{codexDiagnostic("codex_jsonl_invalid_tokens", "skipped Codex token-count row whose token total exceeds the supported range")}, false
	}
	if clamped {
		return counts, []Diagnostic{codexDiagnostic("codex_jsonl_negative_tokens", "clamped negative Codex token components to zero")}, true
	}
	return counts, nil, true
}

func hasInvalidCodexToken(usage map[string]interface{}, names ...string) bool {
	for _, name := range names {
		value, ok := usage[name]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case json.Number:
			if _, err := typed.Int64(); err != nil {
				return true
			}
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed >= math.MaxInt64 || typed < math.MinInt64 {
				return true
			}
		case string:
			if CodexIntField(usage, name) == nil {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func CodexIntField(usage map[string]interface{}, name string) *int64 {
	switch value := usage[name].(type) {
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return &parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil {
			return &parsed
		}
	case float64:
		if math.Trunc(value) == value && value < math.MaxInt64 && value >= math.MinInt64 {
			parsed := int64(value)
			return &parsed
		}
	}
	return nil
}

func (counts codexTokenCounts) equal(other codexTokenCounts) bool {
	return intPointersEqual(counts.input, other.input) &&
		intPointersEqual(counts.output, other.output) &&
		intPointersEqual(counts.reasoning, other.reasoning) &&
		intPointersEqual(counts.cacheRead, other.cacheRead) &&
		intPointersEqual(counts.total, other.total)
}

func (counts codexTokenCounts) lessThan(other codexTokenCounts) bool {
	return counts.sum() < other.sum()
}

func (counts codexTokenCounts) sum() int64 {
	return intPointerValue(counts.input) +
		intPointerValue(counts.output) +
		intPointerValue(counts.reasoning) +
		intPointerValue(counts.cacheRead)
}

func intPointersEqual(left *int64, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func intPointerValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func largerInt(left *int64, right *int64) *int64 {
	if left == nil {
		return right
	}
	if right == nil || *left >= *right {
		return left
	}
	return right
}

func codexTimestampString(record map[string]interface{}, name string) *int64 {
	value := StringField(record, name)
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

func codexMessageID(turnID *string, occurredAtMs int64, lineNumber int) string {
	if turnID != nil {
		return fmt.Sprintf("%s:%d", *turnID, occurredAtMs)
	}
	return fmt.Sprintf("line:%d:%d", lineNumber, occurredAtMs)
}

func codexDiagnostic(code string, message string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessCodex,
		Severity: "warning",
		Code:     code,
		Message:  message,
	}
}

// Snapshot fields preserve source presence and both cache aliases. Replay proof
// must compare the source counters, not normalized/clamped token components.
type CodexUsageSnapshot struct {
	Input       *int64 `json:"input"`
	CachedInput *int64 `json:"cachedInput"`
	CacheRead   *int64 `json:"cacheRead"`
	Output      *int64 `json:"output"`
	Reasoning   *int64 `json:"reasoning"`
	Total       *int64 `json:"total"`
}

type CodexSnapshot struct {
	Last  *CodexUsageSnapshot `json:"last"`
	Total *CodexUsageSnapshot `json:"total"`
}

type codexCandidate struct {
	fact        RawTokenFact
	turnID      *string
	line        int
	snapshot    CodexSnapshot
	replayValid bool
	cumulative  *codexTokenCounts
}

type codexSourceMetadata struct {
	sessionID string
	parentID  string
	fork      bool
	conflict  bool
}

func CodexSnapshotFromUsage(usage map[string]interface{}) (*CodexUsageSnapshot, bool) {
	if usage == nil {
		return nil, false
	}
	snapshot := &CodexUsageSnapshot{}
	fields := []struct {
		name string
		dest **int64
	}{
		{"input_tokens", &snapshot.Input},
		{"cached_input_tokens", &snapshot.CachedInput},
		{"cache_read_input_tokens", &snapshot.CacheRead},
		{"output_tokens", &snapshot.Output},
		{"reasoning_output_tokens", &snapshot.Reasoning},
		{"total_tokens", &snapshot.Total},
	}
	valid, present := true, false
	for _, field := range fields {
		value, exists := usage[field.name]
		if !exists {
			continue
		}
		present = true
		*field.dest = CodexIntField(usage, field.name)
		// Strings can be retained by the legacy ingestion path but do not prove
		// exact source-native integer equality.
		_, numeric := value.(json.Number)
		if !numeric || *field.dest == nil || **field.dest < 0 {
			valid = false
		}
	}
	return snapshot, valid && present
}

func (candidate *codexCandidate) setMessageID() {
	id := CodexMessageIdentity(candidate.turnID, *candidate.fact.OccurredAtMs, candidate.line, candidate.snapshot)
	candidate.fact.MessageID = &id
}
func (candidate codexCandidate) replayFingerprint() string {
	return CodexReplayFingerprint(candidate.turnID, candidate.fact.Provider, candidate.fact.Model, candidate.snapshot, candidate.replayValid)
}

// CodexMessageIdentity retains native turn/time/snapshot identity, including the
// line witness required when cumulative source usage is absent.
func CodexMessageIdentity(turnID *string, occurredAtMs int64, line int, snapshot CodexSnapshot) string {
	encoded, _ := json.Marshal(snapshot)
	id := codexMessageID(turnID, occurredAtMs, line) + ":" + StableHash(string(encoded))
	if turnID != nil && snapshot.Total == nil {
		id += fmt.Sprintf(":line:%d", line)
	}
	return id
}

// CodexReplayFingerprint is proof only for complete, native integer snapshots.
func CodexReplayFingerprint(turnID, provider, model *string, snapshot CodexSnapshot, valid bool) string {
	if !valid || turnID == nil || provider == nil || model == nil {
		return ""
	}
	encoded, _ := json.Marshal(struct {
		Turn     string        `json:"turn"`
		Provider string        `json:"provider"`
		Model    string        `json:"model"`
		Snapshot CodexSnapshot `json:"snapshot"`
	}{*turnID, *provider, *model, snapshot})
	return StableHash(string(encoded))
}

func codexMetadataFromPayload(payload map[string]interface{}, fallback string) codexSourceMetadata {
	metadata := codexSourceMetadata{sessionID: StringValue(payload, fallback, "id")}
	spawn := Nested(Nested(Nested(payload, "source"), "subagent"), "thread_spawn")
	metadata.fork = Nested(payload, "source")["subagent"] != nil
	for _, field := range []struct {
		object map[string]interface{}
		name   string
	}{
		{payload, "forked_from_id"},
		{payload, "parent_thread_id"},
		{spawn, "parent_thread_id"},
	} {
		if value, exists := field.object[field.name]; !exists || value == nil {
			continue
		}
		metadata.fork = true
		reference := StringField(field.object, field.name)
		if reference == nil {
			metadata.conflict = true
			continue
		}
		if metadata.parentID != "" && metadata.parentID != *reference {
			metadata.conflict = true
		}
		metadata.parentID = *reference
	}
	return metadata
}
