package processor

import (
	"context"
	"encoding/json"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"strconv"
	"strings"
	"time"
)

type claudeCodeParser struct{}

func (a claudeCodeParser) factFromRecord(ctx context.Context, source NativeSource, options ParseOptions, fallbackSessionID string, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
	if StringValue(record, "", "type") != "assistant" {
		return RawTokenFact{}, nil, false
	}
	message := Nested(record, "message")
	if message == nil || StringValue(message, "", "role") != "assistant" {
		return RawTokenFact{}, nil, false
	}
	usage := Nested(message, "usage")
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
	if sourceSessionID := StringField(record, "sessionId", "session_id"); sourceSessionID != nil {
		sessionID = *sourceSessionID
	}
	if strings.TrimSpace(sessionID) == "" {
		return RawTokenFact{}, []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_missing_session", "skipped Claude Code assistant token row with no stable session id", "warning")}, false
	}

	nowMs := options.ObservedAtMs
	messageID := StringField(message, "id")
	if messageID == nil {
		messageID = StringField(record, "uuid")
	}
	return RawTokenFact{
		Harness:          HarnessClaudeCode,
		SourceID:         StableHash("claude-code-session:" + sessionID),
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     nowMs,
		OccurredAtMs:     occurredAt,
		SessionID:        &sessionID,
		MessageID:        messageID,
		Provider:         StringField(message, "provider", "provider_id", "providerID"),
		Model:            StringField(message, "model", "model_id", "modelID"),
		UsageScope:       "message",
		Quality:          "derived",
		InputTokens:      tokens.input,
		OutputTokens:     tokens.output,
		ReasoningTokens:  tokens.reasoning,
		CacheReadTokens:  tokens.cacheRead,
		CacheWriteTokens: tokens.cacheWrite,
		TotalTokens:      tokens.total,
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
	outputDetails := Nested(usage, "output_tokens_details")
	if HasInvalidIntegerField(usage, "input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "total_tokens") ||
		HasInvalidIntegerField(outputDetails, "thinking_tokens", "reasoning_tokens") {
		return claudeCodeTokenCounts{}, []Diagnostic{claudeCodeDiagnostic("claude_code_jsonl_invalid_tokens", "skipped Claude Code assistant token row with non-integer token components", "warning")}, false
	}
	counts := claudeCodeTokenCounts{
		input:      IntField(usage, "input_tokens"),
		output:     IntField(usage, "output_tokens"),
		reasoning:  IntField(outputDetails, "thinking_tokens", "reasoning_tokens"),
		cacheRead:  IntField(usage, "cache_read_input_tokens"),
		cacheWrite: IntField(usage, "cache_creation_input_tokens"),
		total:      IntField(usage, "total_tokens"),
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
	if StringField(record, "sessionId", "session_id") != nil {
		sessionSource = "native"
	}
	return SourceIdentityJSON(sessionSource, StringField(record, "requestId", "request_id"))
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
	componentTotal, ok := TokenComponentSum(fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens)
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
	fact.DedupeKey = NativeTupleHash(fact.DedupeKey, SourceSessionIdentity(fact.MetadataJSON))
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
	return NativeTupleHash(parts...)
}

func int64ValueOrZero(value *int64) string {
	if value == nil {
		return "0"
	}
	return strconv.FormatInt(*value, 10)
}

func claudeCodeTimestampString(record map[string]interface{}, name string) *int64 {
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

func claudeCodeDiagnostic(code string, message string, severity string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessClaudeCode,
		Severity: severity,
		Code:     code,
		Message:  message,
	}
}
