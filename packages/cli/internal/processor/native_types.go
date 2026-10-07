package processor

import (
	"sort"
	"strings"
)

type Harness string

const (
	HarnessOpenCode   Harness = "opencode"
	HarnessPi         Harness = "pi"
	HarnessCodex      Harness = "codex"
	HarnessClaudeCode Harness = "claude-code"
)

type NativeSource struct {
	Harness     Harness
	ID          string
	Kind        string
	RawSourceID string
}
type ParseOptions struct {
	ObservedAtMs      int64
	Collector, Parser string
}
type RawTokenFact struct {
	Harness          Harness
	SourceID         string
	SourceKind       string
	Collector        string
	Parser           string
	ObservedAtMs     int64
	OccurredAtMs     *int64
	SessionID        *string
	MessageID        *string
	Provider         *string
	Model            *string
	UsageScope       string
	Quality          string
	InputTokens      *int64
	OutputTokens     *int64
	ReasoningTokens  *int64
	CacheReadTokens  *int64
	CacheWriteTokens *int64
	TotalTokens      *int64
	MetadataJSON     *string
	DedupeKey        string
}
type Diagnostic struct {
	Harness                             Harness
	RawFactKey, Severity, Code, Message string
	MetadataJSON                        *string
}
type optionalString struct {
	String string
	Valid  bool
}
type optionalInt struct {
	Int64 int64
	Valid bool
}
type usageMetadata struct {
	Harness             Harness
	Provider, Model     optionalString
	UsageScope, Quality string
}

// Source identifiers stay in raw_token_usage; these rules affect canonical facts only.
var ProviderAliases = map[Harness]map[string]string{
	HarnessOpenCode:   {"fireworks-ai": "fireworks"},
	HarnessPi:         {"openai-codex": "openai", "fireworks-ai": "fireworks"},
	HarnessCodex:      {"fireworks-ai": "fireworks"},
	HarnessClaudeCode: {"fireworks-ai": "fireworks"},
}

var ModelPrefixes = map[string]string{
	"fireworks": "accounts/fireworks/models/",
}

// Bump when identifier logic changes outside ProviderAliases or ModelPrefixes.
const normalizationRuleRevision = "canonical-identifiers-v1"

func NormalizationRuleSignature(harness string) string {
	parts := []string{normalizationRuleRevision, string(harness)}
	for source, canonical := range ProviderAliases[Harness(harness)] {
		parts = append(parts, "provider:"+source+"="+canonical)
	}
	for provider, prefix := range ModelPrefixes {
		parts = append(parts, "model:"+provider+"="+prefix)
	}
	sort.Strings(parts[2:])
	return StableHash(strings.Join(parts, "\x00"))
}

func countable(row usageMetadata) int {
	if strings.Contains(row.UsageScope, "fallback") {
		return 0
	}
	return 1
}

func normalizedText(value optionalString, fallback string) string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return fallback
	}
	return strings.TrimSpace(value.String)
}

func canonicalProvider(row usageMetadata) (string, string) {
	if provider := normalizedText(row.Provider, ""); provider != "" {
		if alias, ok := ProviderAliases[row.Harness][provider]; ok {
			provider = alias
		}
		return provider, "explicit"
	}
	if row.Harness == HarnessClaudeCode {
		return "maybe-anthropic", "inferred"
	}
	return "unknown", "unknown"
}

func canonicalModel(sourceModel optionalString, provider string) string {
	model := normalizedText(sourceModel, "unknown")
	if prefix, ok := ModelPrefixes[provider]; ok && strings.HasPrefix(model, prefix) {
		if trimmed := strings.TrimPrefix(model, prefix); trimmed != "" {
			return trimmed
		}
	}
	return model
}

func CanonicalProvider(harness string, provider *string) (string, string) {
	return canonicalProvider(usageMetadata{Harness: Harness(harness), Provider: optionalString{String: stringValueOrEmpty(provider), Valid: provider != nil}})
}
func CanonicalModel(model *string, provider string) string {
	return canonicalModel(optionalString{String: stringValueOrEmpty(model), Valid: model != nil}, provider)
}
