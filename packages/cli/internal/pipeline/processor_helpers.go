package pipeline

import "github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"

func processorDiagnostics(values []processor.Diagnostic) []Diagnostic {
	var out []Diagnostic
	for _, d := range values {
		out = append(out, Diagnostic{Harness: Harness(d.Harness), RawFactKey: d.RawFactKey, Severity: d.Severity, Code: d.Code, Message: d.Message, MetadataJSON: d.MetadataJSON})
	}
	return out
}

func processorFact(v processor.RawTokenFact) RawTokenFact {
	return RawTokenFact{Harness: v.Harness, SourceID: v.SourceID, SourceKind: v.SourceKind, Collector: v.Collector, Parser: v.Parser, ObservedAtMs: v.ObservedAtMs, OccurredAtMs: v.OccurredAtMs, SessionID: v.SessionID, MessageID: v.MessageID, Provider: v.Provider, Model: v.Model, UsageScope: v.UsageScope, Quality: v.Quality, InputTokens: v.InputTokens, OutputTokens: v.OutputTokens, ReasoningTokens: v.ReasoningTokens, CacheReadTokens: v.CacheReadTokens, CacheWriteTokens: v.CacheWriteTokens, TotalTokens: v.TotalTokens, MetadataJSON: v.MetadataJSON, DedupeKey: v.DedupeKey}
}

func nativeFact(v RawTokenFact) processor.RawTokenFact {
	return processor.RawTokenFact{Harness: v.Harness, SourceID: v.SourceID, SourceKind: v.SourceKind, Collector: v.Collector, Parser: v.Parser, ObservedAtMs: v.ObservedAtMs, OccurredAtMs: v.OccurredAtMs, SessionID: v.SessionID, MessageID: v.MessageID, Provider: v.Provider, Model: v.Model, UsageScope: v.UsageScope, Quality: v.Quality, InputTokens: v.InputTokens, OutputTokens: v.OutputTokens, ReasoningTokens: v.ReasoningTokens, CacheReadTokens: v.CacheReadTokens, CacheWriteTokens: v.CacheWriteTokens, TotalTokens: v.TotalTokens, MetadataJSON: v.MetadataJSON, DedupeKey: v.DedupeKey}
}

func nativeSource(source Source) processor.NativeSource {
	return processor.NativeSource{Harness: source.Harness, ID: source.ID, Kind: source.Kind, RawSourceID: source.RawSourceID}
}
func nativeOptions(options SyncOptions) processor.ParseOptions {
	return processor.ParseOptions{ObservedAtMs: syncNowMs(options.Now), Collector: options.Collector, Parser: options.Parser}
}
