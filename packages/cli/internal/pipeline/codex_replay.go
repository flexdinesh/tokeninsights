package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// Snapshot fields preserve source presence and both cache aliases. Replay proof
// must compare the source counters, not normalized/clamped token components.
type codexUsageSnapshot struct {
	Input       *int64 `json:"input"`
	CachedInput *int64 `json:"cachedInput"`
	CacheRead   *int64 `json:"cacheRead"`
	Output      *int64 `json:"output"`
	Reasoning   *int64 `json:"reasoning"`
	Total       *int64 `json:"total"`
}

type codexSnapshot struct {
	Last  *codexUsageSnapshot `json:"last"`
	Total *codexUsageSnapshot `json:"total"`
}

type codexCandidate struct {
	fact        RawTokenFact
	turnID      *string
	line        int
	snapshot    codexSnapshot
	replayValid bool
	cumulative  *codexTokenCounts
}

type codexSourceMetadata struct {
	sessionID string
	parentID  string
	fork      bool
	conflict  bool
}

type codexParseResult struct {
	facts       []RawTokenFact
	diagnostics []Diagnostic
	// Each fingerprint maps to unique accepted logical originals. More than one
	// original is ambiguous, even if all normalized token components agree.
	proofs     map[string]map[string]RawTokenFact
	err        error
	unresolved bool
}

func codexSnapshotFromUsage(usage map[string]interface{}) (*codexUsageSnapshot, bool) {
	if usage == nil {
		return nil, false
	}
	snapshot := &codexUsageSnapshot{}
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
		*field.dest = codexIntField(usage, field.name)
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
	encoded, _ := json.Marshal(candidate.snapshot)
	id := codexMessageID(candidate.turnID, *candidate.fact.OccurredAtMs, candidate.line) + ":" + stableHash(string(encoded))
	if candidate.turnID != nil && candidate.snapshot.Total == nil {
		id += fmt.Sprintf(":line:%d", candidate.line)
	}
	candidate.fact.MessageID = &id
}

func (candidate codexCandidate) replayFingerprint() string {
	if !candidate.replayValid || candidate.turnID == nil || candidate.fact.Provider == nil || candidate.fact.Model == nil {
		return ""
	}
	encoded, _ := json.Marshal(struct {
		Turn     string        `json:"turn"`
		Provider string        `json:"provider"`
		Model    string        `json:"model"`
		Snapshot codexSnapshot `json:"snapshot"`
	}{*candidate.turnID, *candidate.fact.Provider, *candidate.fact.Model, candidate.snapshot})
	return stableHash(string(encoded))
}

func codexMetadataFromPayload(payload map[string]interface{}, fallback string) codexSourceMetadata {
	metadata := codexSourceMetadata{sessionID: stringValue(payload, fallback, "id")}
	spawn := nested(nested(nested(payload, "source"), "subagent"), "thread_spawn")
	metadata.fork = nested(payload, "source")["subagent"] != nil
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
		reference := stringField(field.object, field.name)
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

// Discovery only peeks through the first usable header. No transcript payloads
// or full paths enter facts, fingerprints or diagnostics.
func codexReadSourceMetadata(ctx context.Context, source Source) (codexSourceMetadata, error) {
	metadata := codexSourceMetadata{sessionID: codexSessionIDFromFilename(source.Path)}
	if err := ctx.Err(); err != nil {
		return metadata, err
	}
	file, err := os.Open(source.Path)
	if err != nil {
		// Ordinary read failures belong to per-source ingestion. Cancellation
		// must instead stop discovery and optional ancestry parsing immediately.
		return metadata, ctx.Err()
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxCodexJSONLLineBytes)
	for {
		if err := ctx.Err(); err != nil {
			return metadata, err
		}
		if !scanner.Scan() {
			break
		}
		var record map[string]interface{}
		if json.Unmarshal(scanner.Bytes(), &record) != nil || stringValue(record, "", "type") != "session_meta" {
			continue
		}
		payload := nested(record, "payload")
		header := codexMetadataFromPayload(payload, metadata.sessionID)
		header.fork = header.fork || metadata.fork
		metadata = header
		if stringField(payload, "id") != nil {
			break
		}
	}
	return metadata, ctx.Err()
}

// Validate the entire chain before recursing. This makes cycles and copied or
// conflicting parent sources deterministic regardless of ingestion order.
func (a *codexJSONLAdapter) ancestryProblem(source Source) string {
	seen := make(map[string]bool)
	for {
		metadata := a.metadata[source.Path]
		if seen[metadata.sessionID] {
			return "codex_jsonl_replay_cycle"
		}
		seen[metadata.sessionID] = true
		if metadata.conflict || len(a.sessions[metadata.sessionID]) > 1 {
			return "codex_jsonl_replay_ambiguous_parent"
		}
		if !metadata.fork {
			return ""
		}
		parents := a.sessions[metadata.parentID]
		if metadata.parentID == "" || len(parents) == 0 {
			return "codex_jsonl_replay_missing_parent"
		}
		if len(parents) != 1 {
			return "codex_jsonl_replay_ambiguous_parent"
		}
		source = parents[0]
	}
}

func (a *codexJSONLAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	result := a.parseResolved(ctx, source, options)
	facts := make([]RawTokenFact, len(result.facts))
	for i, fact := range result.facts {
		fact.Collector = options.Collector
		fact.Parser = options.Parser
		fact.ObservedAtMs = syncNowMs(options.Now)
		facts[i] = fact
	}
	return facts, result.diagnostics, result.err
}

func (a *codexJSONLAdapter) parseResolved(ctx context.Context, source Source, options SyncOptions) codexParseResult {
	if err := ctx.Err(); err != nil {
		return codexParseResult{err: err}
	}
	if result, found := a.cache[source.Path]; found {
		return result
	}
	if a.cache == nil {
		a.cache = make(map[string]codexParseResult)
	}
	if a.metadata == nil {
		a.metadata = make(map[string]codexSourceMetadata)
	}
	metadata, found := a.metadata[source.Path]
	if !found {
		var err error
		metadata, err = codexReadSourceMetadata(ctx, source)
		if err != nil {
			return codexParseResult{err: err}
		}
		a.metadata[source.Path] = metadata
	}
	candidates, diagnostics, err := a.parseCandidates(ctx, source, options)
	if err := ctx.Err(); err != nil {
		return codexParseResult{err: err}
	}
	result := codexParseResult{diagnostics: diagnostics, err: err, proofs: make(map[string]map[string]RawTokenFact)}
	if err != nil {
		a.cache[source.Path] = result
		return result
	}
	problem := a.ancestryProblem(source)
	if metadata.fork && problem == "" {
		parent := a.parseResolved(ctx, a.sessions[metadata.parentID][0], options)
		if err := ctx.Err(); err != nil {
			return codexParseResult{err: err}
		}
		if parent.err != nil {
			problem = "codex_jsonl_replay_parent_read_error"
		} else if parent.unresolved {
			problem = "codex_jsonl_replay_unresolved_parent"
		} else {
			for fingerprint, originals := range parent.proofs {
				for _, original := range originals {
					result.addProof(fingerprint, original)
				}
			}
		}
	}
	if problem != "" {
		result.unresolved = true
		result.diagnostics = append(result.diagnostics, codexDiagnostic(problem, "retained Codex usage because explicit ancestry could not establish replay ownership"))
	}
	var previousTotal *codexTokenCounts
	var previousTurn *string
	resolvedReplay := false
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return codexParseResult{err: err}
		}
		fingerprint := candidate.replayFingerprint()
		originals := result.proofs[fingerprint]
		// Only ancestry proves replay, never another request in this session.
		inherited := make(map[string]RawTokenFact)
		for key, original := range originals {
			if original.SessionID != nil && candidate.fact.SessionID != nil && *original.SessionID != *candidate.fact.SessionID {
				inherited[key] = original
			}
		}
		if len(inherited) == 1 {
			resolvedReplay = true
			for _, original := range inherited {
				result.facts = append(result.facts, original)
			}
			continue
		}
		uncertain := problem != "" || fingerprint == "" || len(inherited) > 1
		// An unmatched fork snapshot may still be inherited when an ancestor
		// omitted or changed metadata. Only an explicit new turn releases that
		// possible inherited baseline; same-turn stale protection remains intact.
		if metadata.fork && candidate.turnID != nil && (previousTurn == nil || *previousTurn != *candidate.turnID) {
			if previousTotal != nil {
				result.diagnostics = append(result.diagnostics, codexDiagnostic("codex_jsonl_replay_boundary_uncertain", "reset cumulative suppression at an explicit turn with unresolved Codex history"))
			}
			previousTotal = nil
		}
		previousTurn = candidate.turnID
		if len(inherited) > 1 {
			result.diagnostics = append(result.diagnostics, codexDiagnostic("codex_jsonl_replay_ambiguous_fact", "retained Codex token snapshot matching multiple logical ancestor facts"))
		} else if metadata.fork && fingerprint == "" {
			result.diagnostics = append(result.diagnostics, codexDiagnostic("codex_jsonl_replay_unverifiable_snapshot", "retained Codex usage without complete valid replay identity"))
		}
		if candidate.cumulative != nil && len(inherited) <= 1 {
			if previousTotal != nil && previousTotal.equal(*candidate.cumulative) {
				result.diagnostics = append(result.diagnostics, codexDiagnostic("codex_jsonl_duplicate_token_snapshot", "suppressed duplicate Codex token-count snapshot"))
				continue
			}
			if previousTotal != nil && candidate.cumulative.lessThan(*previousTotal) {
				result.diagnostics = append(result.diagnostics, codexDiagnostic("codex_jsonl_stale_token_snapshot", "suppressed stale Codex token-count snapshot with regressed cumulative totals"))
				continue
			}
			previousTotal = candidate.cumulative
		}
		result.facts = append(result.facts, candidate.fact)
		if !uncertain {
			result.addProof(fingerprint, candidate.fact)
		}
	}
	if err := ctx.Err(); err != nil {
		return codexParseResult{err: err}
	}
	if resolvedReplay {
		result.diagnostics = append(result.diagnostics, Diagnostic{
			Harness:  HarnessCodex,
			Severity: "info",
			Code:     "codex_jsonl_replay_resolved",
			Message:  "matched inherited Codex token usage to original session facts",
		})
	}
	a.cache[source.Path] = result
	return result
}

func (result *codexParseResult) addProof(fingerprint string, fact RawTokenFact) {
	if result.proofs[fingerprint] == nil {
		result.proofs[fingerprint] = make(map[string]RawTokenFact)
	}
	result.proofs[fingerprint][rawFactKey(fact)] = fact
}
