package processor

import "context"

// TokenComponents are exclusive components after source-native normalization.
type TokenComponents struct{ Input, Output, Reasoning, CacheRead, CacheWrite, Total *int64 }

func CodexTokens(usage map[string]interface{}) (TokenComponents, []Diagnostic, bool) {
	v, d, ok := codexTokensFromUsage(usage)
	return TokenComponents{Input: v.input, Output: v.output, Reasoning: v.reasoning, CacheRead: v.cacheRead, Total: v.total}, d, ok
}

// OpenCodeMessage interprets native message metadata. Optional row fields are
// pointers so SQL nullability stays in the reader adapter.
func OpenCodeMessage(source NativeSource, options ParseOptions, messageID string, sessionID *string, created *int64, data string, v2 bool) (RawTokenFact, []Diagnostic, bool) {
	session := optionalString{String: stringValueOrEmpty(sessionID), Valid: sessionID != nil}
	timestamp := optionalInt{Int64: intValueOrZero(created), Valid: created != nil}
	if v2 {
		return (opencodeParser{}).factFromV2Message(source, options, messageID, session, timestamp, data)
	}
	return (opencodeParser{}).factFromMessage(source, options, messageID, session, timestamp, data)
}
func PiMessage(ctx context.Context, source NativeSource, options ParseOptions, sessionID string, nativeSession bool, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
	return (piParser{}).factFromRecord(ctx, source, options, piJSONLSessionFile{sessionID: sessionID, hasHeader: nativeSession}, record)
}
func ClaudeCodeMessage(ctx context.Context, source NativeSource, options ParseOptions, fallbackSessionID string, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
	return (claudeCodeParser{}).factFromRecord(ctx, source, options, fallbackSessionID, record)
}
func FinalizeClaudeCodeFact(fact *RawTokenFact) ([]Diagnostic, bool) {
	return finalizeClaudeCodeFact(fact)
}
