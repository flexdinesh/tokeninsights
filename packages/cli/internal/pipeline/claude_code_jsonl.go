package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	native, diagnostics, ok := processor.ClaudeCodeMessage(ctx, nativeSource(source), nativeOptions(options), fallbackSessionID, record)
	fact := processorFact(native)
	if ok {
		fact.Location, _ = resolveFactLocation(ctx, options, stringValue(record, "", "cwd"), "", "")
	}
	return fact, processorDiagnostics(diagnostics), ok
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
	native := nativeFact(*fact)
	diagnostics, ok := processor.FinalizeClaudeCodeFact(&native)
	fact.OutputTokens = native.OutputTokens
	fact.ReasoningTokens = native.ReasoningTokens
	fact.TotalTokens = native.TotalTokens
	fact.DedupeKey = native.DedupeKey
	return processorDiagnostics(diagnostics), ok
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
