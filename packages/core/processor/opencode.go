package processor

import (
	"encoding/json"
	"fmt"
	"strings"
)

type opencodeMessageData struct {
	Role       string                `json:"role"`
	ModelID    string                `json:"modelID"`
	ProviderID string                `json:"providerID"`
	Tokens     *opencodeTokenData    `json:"tokens"`
	Time       *opencodeMessageTimes `json:"time"`
}

type opencodeV2MessageData struct {
	Model  *opencodeV2Model      `json:"model"`
	Tokens *opencodeTokenData    `json:"tokens"`
	Time   *opencodeMessageTimes `json:"time"`
}

type opencodeV2Model struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
}

type opencodeTokenData struct {
	Input     *int64              `json:"input"`
	Output    *int64              `json:"output"`
	Reasoning *int64              `json:"reasoning"`
	Cache     *opencodeTokenCache `json:"cache"`
}

type opencodeTokenCache struct {
	Read  *int64 `json:"read"`
	Write *int64 `json:"write"`
}

type opencodeMessageTimes struct {
	Created   *int64 `json:"created"`
	Completed *int64 `json:"completed"`
}

type opencodeParser struct{}

func (a opencodeParser) factFromMessage(source NativeSource, options ParseOptions, rowMessageID string, rowSessionID optionalString, rowTimeCreated optionalInt, rawData string) (RawTokenFact, []Diagnostic, bool) {
	var messageData opencodeMessageData
	if err := json.Unmarshal([]byte(rawData), &messageData); err != nil {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_parse_error", "skipped OpenCode message row with invalid JSON data")}, false
	}
	if messageData.Role != "assistant" {
		return RawTokenFact{}, nil, false
	}
	if !hasUsableOpenCodeTokens(messageData.Tokens) {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_missing_tokens", "skipped OpenCode assistant message with no usable token components")}, false
	}
	var diagnostics []Diagnostic
	if clampOpenCodeTokens(messageData.Tokens) {
		diagnostics = append(diagnostics, opencodeDiagnostic("opencode_sqlite_negative_tokens", "clamped negative OpenCode token components to zero"))
	}
	if _, ok := openCodeTokenTotal(messageData.Tokens); !ok {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_invalid_tokens", "skipped OpenCode assistant message whose token total exceeds the supported range")}, false
	}

	occurredAt := opencodeOccurredAt(messageData.Time, rowTimeCreated)
	if occurredAt == nil {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_missing_time", "skipped OpenCode assistant message with no usable created timestamp")}, false
	}

	sourceID := source.RawSourceID
	if sourceID == "" {
		sourceID = source.ID
	}
	sessionID := trimSQLString(rowSessionID)
	return RawTokenFact{
		Harness:          HarnessOpenCode,
		SourceID:         sourceID,
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     options.ObservedAtMs,
		OccurredAtMs:     occurredAt,
		SessionID:        stringPtrFromTrimmed(sessionID),
		MessageID:        stringPtrFromTrimmed(rowMessageID),
		Provider:         stringPtrFromTrimmed(messageData.ProviderID),
		Model:            stringPtrFromTrimmed(messageData.ModelID),
		UsageScope:       "message",
		Quality:          "exact",
		InputTokens:      messageData.Tokens.Input,
		OutputTokens:     messageData.Tokens.Output,
		ReasoningTokens:  defaultZero(messageData.Tokens.Reasoning),
		CacheReadTokens:  opencodeCacheRead(messageData.Tokens),
		CacheWriteTokens: opencodeCacheWrite(messageData.Tokens),
		TotalTokens:      nil,
		MetadataJSON:     nil,
		DedupeKey:        opencodeDedupeKey(sessionID, rowMessageID, messageData.ProviderID, messageData.ModelID, messageData.Tokens, messageData.Time, occurredAt),
	}, diagnostics, true
}

func (a opencodeParser) factFromV2Message(source NativeSource, options ParseOptions, rowMessageID string, rowSessionID optionalString, rowTimeCreated optionalInt, rawData string) (RawTokenFact, []Diagnostic, bool) {
	var messageData opencodeV2MessageData
	if err := json.Unmarshal([]byte(rawData), &messageData); err != nil {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_parse_error", "skipped OpenCode V2 assistant message row with invalid JSON data")}, false
	}
	if !hasUsableOpenCodeTokens(messageData.Tokens) {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_missing_tokens", "skipped OpenCode V2 assistant message with no usable token components")}, false
	}
	var diagnostics []Diagnostic
	if clampOpenCodeTokens(messageData.Tokens) {
		diagnostics = append(diagnostics, opencodeDiagnostic("opencode_sqlite_negative_tokens", "clamped negative OpenCode token components to zero"))
	}
	if _, ok := openCodeTokenTotal(messageData.Tokens); !ok {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_invalid_tokens", "skipped OpenCode V2 assistant message whose token total exceeds the supported range")}, false
	}
	occurredAt := opencodeOccurredAt(messageData.Time, rowTimeCreated)
	if occurredAt == nil {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_missing_time", "skipped OpenCode V2 assistant message with no usable created timestamp")}, false
	}

	sourceID := source.RawSourceID
	if sourceID == "" {
		sourceID = source.ID
	}
	providerID := ""
	modelID := ""
	if messageData.Model != nil {
		providerID = messageData.Model.ProviderID
		modelID = messageData.Model.ID
	}

	return RawTokenFact{
		Harness:          HarnessOpenCode,
		SourceID:         sourceID,
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     options.ObservedAtMs,
		OccurredAtMs:     occurredAt,
		SessionID:        stringPtrFromTrimmed(trimSQLString(rowSessionID)),
		MessageID:        stringPtrFromTrimmed(rowMessageID),
		Provider:         stringPtrFromTrimmed(providerID),
		Model:            stringPtrFromTrimmed(modelID),
		UsageScope:       "message",
		Quality:          "exact",
		InputTokens:      messageData.Tokens.Input,
		OutputTokens:     messageData.Tokens.Output,
		ReasoningTokens:  defaultZero(messageData.Tokens.Reasoning),
		CacheReadTokens:  opencodeCacheRead(messageData.Tokens),
		CacheWriteTokens: opencodeCacheWrite(messageData.Tokens),
		TotalTokens:      nil,
		MetadataJSON:     nil,
		DedupeKey:        opencodeDedupeKey(trimSQLString(rowSessionID), rowMessageID, providerID, modelID, messageData.Tokens, messageData.Time, occurredAt),
	}, diagnostics, true
}

func hasUsableOpenCodeTokens(tokens *opencodeTokenData) bool {
	if tokens == nil {
		return false
	}
	return tokens.Input != nil ||
		tokens.Output != nil ||
		tokens.Reasoning != nil ||
		opencodeCacheRead(tokens) != nil ||
		opencodeCacheWrite(tokens) != nil
}

func clampOpenCodeTokens(tokens *opencodeTokenData) bool {
	clamped := false
	clamped = clampInt(tokens.Input) || clamped
	clamped = clampInt(tokens.Output) || clamped
	clamped = clampInt(tokens.Reasoning) || clamped
	if tokens.Cache != nil {
		clamped = clampInt(tokens.Cache.Read) || clamped
		clamped = clampInt(tokens.Cache.Write) || clamped
	}
	return clamped
}

func openCodeTokenTotal(tokens *opencodeTokenData) (int64, bool) {
	return TokenComponentSum(tokens.Input, tokens.Output, tokens.Reasoning, opencodeCacheRead(tokens), opencodeCacheWrite(tokens))
}

func clampInt(value *int64) bool {
	if value == nil || *value >= 0 {
		return false
	}
	*value = 0
	return true
}

func opencodeOccurredAt(times *opencodeMessageTimes, rowTimeCreated optionalInt) *int64 {
	if times != nil && times.Created != nil {
		return times.Created
	}
	if rowTimeCreated.Valid {
		return &rowTimeCreated.Int64
	}
	return nil
}

func trimSQLString(value optionalString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func stringPtrFromTrimmed(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func defaultZero(value *int64) *int64 {
	if value != nil {
		return value
	}
	zero := int64(0)
	return &zero
}

func opencodeCacheRead(tokens *opencodeTokenData) *int64 {
	if tokens == nil || tokens.Cache == nil {
		return nil
	}
	return tokens.Cache.Read
}

func opencodeCacheWrite(tokens *opencodeTokenData) *int64 {
	if tokens == nil || tokens.Cache == nil {
		return nil
	}
	return tokens.Cache.Write
}

func opencodeDedupeKey(sessionID string, messageID string, providerID string, modelID string, tokens *opencodeTokenData, times *opencodeMessageTimes, occurredAt *int64) string {
	completedAt := ""
	if times != nil && times.Completed != nil {
		completedAt = fmt.Sprint(*times.Completed)
	}
	parts := []string{
		"opencode-sqlite-message",
		strings.TrimSpace(sessionID),
		strings.TrimSpace(messageID),
		intPtrValue(occurredAt),
		completedAt,
		strings.TrimSpace(providerID),
		strings.TrimSpace(modelID),
		intPtrValue(tokens.Input),
		intPtrValue(tokens.Output),
		intPtrValue(tokens.Reasoning),
		intPtrValue(opencodeCacheRead(tokens)),
		intPtrValue(opencodeCacheWrite(tokens)),
	}
	return NativeTupleHash(parts...)
}

func intPtrValue(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func opencodeDiagnostic(code string, message string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessOpenCode,
		Severity: "warning",
		Code:     code,
		Message:  message,
	}
}
