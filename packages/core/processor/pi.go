package processor

import (
	"context"
	"strings"
	"time"
)

type piParser struct{}
type piJSONLSessionFile struct {
	sessionID string
	hasHeader bool
}

func (a piParser) factFromRecord(ctx context.Context, source NativeSource, options ParseOptions, session piJSONLSessionFile, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
	if StringValue(record, "", "type") != "message" {
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

	tokens, tokenDiagnostics, ok := piTokensFromUsage(usage)
	if !ok {
		return RawTokenFact{}, tokenDiagnostics, false
	}
	occurredAt := IntField(message, "timestamp")
	if occurredAt == nil {
		occurredAt = piTimestampString(record, "timestamp")
	}
	if occurredAt == nil {
		return RawTokenFact{}, []Diagnostic{piDiagnostic("pi_jsonl_missing_time", "skipped Pi assistant token row with no usable timestamp")}, false
	}

	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, tokenDiagnostics...)
	messageID := StringField(record, "id")
	if messageID == nil {
		diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_missing_message_id", "ingested Pi assistant token row without a message id"))
	}
	if strings.TrimSpace(session.sessionID) == "" {
		return RawTokenFact{}, []Diagnostic{piDiagnostic("pi_jsonl_missing_session", "skipped Pi assistant token row with no stable session id")}, false
	}

	nowMs := options.ObservedAtMs
	sessionSource := "filename"
	if session.hasHeader {
		sessionSource = "native"
	}
	sourceID := StableHash("pi-session:" + session.sessionID)
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
		Provider:         StringField(message, "provider"),
		Model:            StringField(message, "model"),
		UsageScope:       "message",
		Quality:          "exact",
		InputTokens:      tokens.input,
		OutputTokens:     tokens.output,
		ReasoningTokens:  tokens.reasoning,
		CacheReadTokens:  tokens.cacheRead,
		CacheWriteTokens: tokens.cacheWrite,
		TotalTokens:      tokens.total,
		MetadataJSON:     SourceIdentityJSON(sessionSource, nil),
	}, diagnostics, true
}

type piTokenCounts struct {
	input      *int64
	output     *int64
	reasoning  *int64
	cacheRead  *int64
	cacheWrite *int64
	total      *int64
}

func piTokensFromUsage(usage map[string]interface{}) (piTokenCounts, []Diagnostic, bool) {
	if HasInvalidIntegerField(usage, "input", "output", "reasoning", "cacheRead", "cacheWrite", "totalTokens") {
		return piTokenCounts{}, []Diagnostic{piDiagnostic("pi_jsonl_invalid_tokens", "skipped Pi assistant token row with non-integer token components")}, false
	}
	counts := piTokenCounts{
		input:      IntField(usage, "input"),
		output:     IntField(usage, "output"),
		reasoning:  IntField(usage, "reasoning"),
		cacheRead:  IntField(usage, "cacheRead"),
		cacheWrite: IntField(usage, "cacheWrite"),
		total:      IntField(usage, "totalTokens"),
	}
	if counts.input == nil && counts.output == nil && counts.reasoning == nil && counts.cacheRead == nil && counts.cacheWrite == nil && counts.total == nil {
		return piTokenCounts{}, []Diagnostic{piDiagnostic("pi_jsonl_missing_tokens", "skipped Pi assistant token row with no usable token components")}, false
	}
	clamped := false
	clampToken(counts.input, &clamped)
	clampToken(counts.output, &clamped)
	clampToken(counts.reasoning, &clamped)
	clampToken(counts.cacheRead, &clamped)
	clampToken(counts.cacheWrite, &clamped)
	clampToken(counts.total, &clamped)
	var diagnostics []Diagnostic
	if clamped {
		diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_negative_tokens", "clamped negative Pi token components to zero"))
	}

	componentTotal, totalOK := TokenComponentSum(counts.input, counts.output, counts.cacheRead, counts.cacheWrite)
	if !totalOK {
		return piTokenCounts{}, []Diagnostic{piDiagnostic("pi_jsonl_invalid_tokens", "skipped Pi assistant token row whose token total exceeds the supported range")}, false
	}
	if counts.total != nil && *counts.total != componentTotal {
		cacheTotal, cacheOK := TokenComponentSum(counts.cacheRead, counts.cacheWrite)
		inclusiveTotal, inclusiveOK := TokenComponentSum(counts.input, counts.output)
		if cacheOK && inclusiveOK && cacheTotal > 0 && counts.input != nil && *counts.input >= cacheTotal && *counts.total == inclusiveTotal {
			adjusted := *counts.input - cacheTotal
			counts.input = &adjusted
			diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_inclusive_input_tokens", "subtracted cached tokens from legacy Pi input tokens"))
		} else {
			counts.total = nil
			diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_inconsistent_total", "ignored Pi totalTokens that did not equal the token component sum"))
		}
	}

	if counts.reasoning != nil {
		if counts.output == nil || *counts.reasoning > *counts.output {
			counts.reasoning = nil
			diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_invalid_reasoning", "ignored Pi reasoning tokens that exceeded inclusive output tokens"))
		} else {
			nonReasoningOutput := *counts.output - *counts.reasoning
			counts.output = &nonReasoningOutput
		}
	}
	return counts, diagnostics, true
}

func clampToken(value *int64, clamped *bool) {
	if value == nil || *value >= 0 {
		return
	}
	*value = 0
	*clamped = true
}

func piTimestampString(record map[string]interface{}, name string) *int64 {
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

func piDiagnostic(code string, message string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessPi,
		Severity: "warning",
		Code:     code,
		Message:  message,
	}
}
