package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	codexSessionJSONLSourceKind = "codex-session-jsonl"
	maxCodexJSONLLineBytes      = 16 * 1024 * 1024
)

type codexJSONLAdapter struct{}

type codexJSONLState struct {
	filenameSessionID string
	sessionID         string
	sawSessionMeta    bool
	provider          *string
	model             *string
	turnID            *string
	previousTotal     *codexTokenCounts
	pending           []codexPendingFact
}

type codexTokenCounts struct {
	input     *int64
	output    *int64
	reasoning *int64
	cacheRead *int64
	total     *int64
}

type codexPendingFact struct {
	fact        RawTokenFact
	diagnostics []Diagnostic
}

func (a codexJSONLAdapter) Harness() Harness {
	return HarnessCodex
}

func (a codexJSONLAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	var roots []string
	sourceDir := strings.TrimSpace(options.SourceDir)
	if sourceDir != "" {
		harnessDir := filepath.Join(sourceDir, string(HarnessCodex))
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
		root := strings.TrimSpace(os.Getenv("CODEX_HOME"))
		if root == "" {
			home := strings.TrimSpace(os.Getenv("HOME"))
			if home == "" {
				return nil, nil
			}
			root = filepath.Join(home, ".codex")
		}
		roots = append(roots, filepath.Join(root, "sessions"))
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
			if isCodexSessionSource(root) {
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
			if isCodexSessionSource(path) {
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

func (a codexJSONLAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness: HarnessCodex,
		ID:      stableHash("codex-session-file:" + filepath.ToSlash(rel)),
		Kind:    codexSessionJSONLSourceKind,
		Path:    path,
	}
}

func (a codexJSONLAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	state := codexJSONLState{filenameSessionID: codexSessionIDFromFilename(source.Path)}
	state.sessionID = state.filenameSessionID
	var facts []RawTokenFact
	var diagnostics []Diagnostic
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxCodexJSONLLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_parse_error", "skipped unparsable Codex JSONL line"))
			continue
		}
		recordType := stringValue(record, "", "type")
		switch recordType {
		case "session_meta":
			diagnostics = append(diagnostics, state.applySessionMeta(record)...)
		case "turn_context":
			state.applyTurnContext(record)
			pendingFacts, pendingDiagnostics := state.flushPending(true)
			facts = append(facts, pendingFacts...)
			diagnostics = append(diagnostics, pendingDiagnostics...)
		case "event_msg":
			fact, rowDiagnostics, ok := a.factFromEvent(source, options, &state, record, lineNumber)
			diagnostics = append(diagnostics, rowDiagnostics...)
			if ok {
				facts = append(facts, fact)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	pendingFacts, pendingDiagnostics := state.flushPending(false)
	facts = append(facts, pendingFacts...)
	diagnostics = append(diagnostics, pendingDiagnostics...)
	if strings.TrimSpace(state.sessionID) == "" && len(facts) == 0 {
		diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_missing_session_meta", "Codex session file had no usable session metadata"))
	}
	return facts, diagnostics, nil
}

func (state *codexJSONLState) applySessionMeta(record map[string]interface{}) []Diagnostic {
	payload := nested(record, "payload")
	if payload == nil {
		return nil
	}
	sessionID := stringField(payload, "id")
	if sessionID == nil {
		return nil
	}
	var diagnostics []Diagnostic
	if !state.sawSessionMeta {
		state.sawSessionMeta = true
		if state.filenameSessionID != "" && state.filenameSessionID != *sessionID {
			diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_session_id_mismatch", "Codex session metadata id differs from filename session id"))
		}
		state.sessionID = *sessionID
		if state.provider == nil {
			state.provider = stringField(payload, "model_provider")
		}
		return diagnostics
	} else if state.sessionID != *sessionID {
		diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_multiple_session_meta", "Codex session file contains multiple session metadata ids"))
	}
	if state.provider == nil {
		state.provider = stringField(payload, "model_provider")
	}
	return diagnostics
}

func (state *codexJSONLState) applyTurnContext(record map[string]interface{}) {
	payload := nested(record, "payload")
	if payload == nil {
		return
	}
	if turnID := stringField(payload, "turn_id"); turnID != nil {
		state.turnID = turnID
	}
	if model := stringField(payload, "model"); model != nil {
		state.model = model
	}
}

func (state *codexJSONLState) flushPending(resolved bool) ([]RawTokenFact, []Diagnostic) {
	if len(state.pending) == 0 {
		return nil, nil
	}
	pending := state.pending
	state.pending = nil
	facts := make([]RawTokenFact, 0, len(pending))
	var diagnostics []Diagnostic
	for _, item := range pending {
		diagnostics = append(diagnostics, item.diagnostics...)
		fact := item.fact
		if resolved {
			fact.Model = state.model
			if state.turnID != nil && fact.OccurredAtMs != nil {
				messageID := codexMessageID(state.turnID, *fact.OccurredAtMs, 0)
				fact.MessageID = &messageID
			}
		} else {
			diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_missing_model", "ingested Codex token-count row before model state was resolved"))
		}
		facts = append(facts, fact)
	}
	return facts, diagnostics
}

func (a codexJSONLAdapter) factFromEvent(source Source, options SyncOptions, state *codexJSONLState, record map[string]interface{}, lineNumber int) (RawTokenFact, []Diagnostic, bool) {
	payload := nested(record, "payload")
	if payload == nil {
		return RawTokenFact{}, nil, false
	}
	if stringValue(payload, "", "type") == "task_started" {
		if turnID := stringField(payload, "turn_id"); turnID != nil {
			state.turnID = turnID
		}
		return RawTokenFact{}, nil, false
	}
	if stringValue(payload, "", "type") != "token_count" {
		return RawTokenFact{}, nil, false
	}
	if strings.TrimSpace(state.sessionID) == "" {
		return RawTokenFact{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_session", "skipped Codex token-count row with no stable session id")}, false
	}
	occurredAt := codexTimestampString(record, "timestamp")
	if occurredAt == nil {
		return RawTokenFact{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_time", "skipped Codex token-count row with no usable timestamp")}, false
	}
	info := nested(payload, "info")
	if info == nil {
		return RawTokenFact{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_tokens", "skipped Codex token-count row with no usable token components")}, false
	}
	lastUsage := nested(info, "last_token_usage")
	if lastUsage == nil {
		return RawTokenFact{}, []Diagnostic{codexDiagnostic("codex_jsonl_missing_tokens", "skipped Codex token-count row with no usable last token usage")}, false
	}
	tokens, tokenDiagnostics, ok := codexTokensFromUsage(lastUsage)
	if !ok {
		return RawTokenFact{}, tokenDiagnostics, false
	}

	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, tokenDiagnostics...)
	totalUsage := nested(info, "total_token_usage")
	if totalUsage != nil {
		cumulative, cumulativeDiagnostics, cumulativeOK := codexTokensFromUsage(totalUsage)
		diagnostics = append(diagnostics, cumulativeDiagnostics...)
		if !cumulativeOK {
			return RawTokenFact{}, diagnostics, false
		}
		if state.previousTotal != nil && state.previousTotal.equal(cumulative) {
			diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_duplicate_token_snapshot", "suppressed duplicate Codex token-count snapshot"))
			return RawTokenFact{}, diagnostics, false
		}
		if state.previousTotal != nil && cumulative.lessThan(*state.previousTotal) {
			diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_stale_token_snapshot", "suppressed stale Codex token-count snapshot with regressed cumulative totals"))
			return RawTokenFact{}, diagnostics, false
		}
		state.previousTotal = &cumulative
	} else {
		diagnostics = append(diagnostics, codexDiagnostic("codex_jsonl_last_without_total", "ingested Codex token-count row without cumulative duplicate protection"))
	}
	nowMs := options.Now.UnixMilli()
	if options.Now.IsZero() {
		nowMs = time.Now().UnixMilli()
	}
	messageID := codexMessageID(state.turnID, *occurredAt, lineNumber)
	sourceID := stableHash("codex-session:" + state.sessionID)
	fact := RawTokenFact{
		Harness:          HarnessCodex,
		SourceID:         sourceID,
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     nowMs,
		OccurredAtMs:     occurredAt,
		SessionID:        &state.sessionID,
		MessageID:        &messageID,
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
	if state.model == nil {
		state.pending = append(state.pending, codexPendingFact{fact: fact, diagnostics: diagnostics})
		return RawTokenFact{}, nil, false
	}
	return fact, diagnostics, true
}

func codexTokensFromUsage(usage map[string]interface{}) (codexTokenCounts, []Diagnostic, bool) {
	if hasInvalidCodexToken(usage, "input_tokens", "cached_input_tokens", "cache_read_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens") {
		return codexTokenCounts{}, []Diagnostic{codexDiagnostic("codex_jsonl_invalid_tokens", "skipped Codex token-count row with non-numeric token components")}, false
	}
	rawInput := intField(usage, "input_tokens")
	cacheRead := largerInt(intField(usage, "cached_input_tokens"), intField(usage, "cache_read_input_tokens"))
	counts := codexTokenCounts{
		input:     rawInput,
		output:    intField(usage, "output_tokens"),
		reasoning: intField(usage, "reasoning_output_tokens"),
		cacheRead: cacheRead,
		total:     intField(usage, "total_tokens"),
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
	if counts.input != nil && counts.cacheRead != nil {
		*counts.input -= *counts.cacheRead
		clampToken(counts.input, &clamped)
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
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) {
				return true
			}
		case string:
			if intField(usage, name) == nil {
				return true
			}
		default:
			return true
		}
	}
	return false
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

func codexMessageID(turnID *string, occurredAtMs int64, lineNumber int) string {
	if turnID != nil {
		return fmt.Sprintf("%s:%d", *turnID, occurredAtMs)
	}
	return fmt.Sprintf("line:%d:%d", lineNumber, occurredAtMs)
}

func isCodexSessionSource(path string) bool {
	if !isCandidateSource(path) {
		return false
	}
	return strings.HasPrefix(filepath.Base(path), "rollout-")
}

func codexSessionIDFromFilename(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !strings.HasPrefix(name, "rollout-") {
		return ""
	}
	rest := strings.TrimPrefix(name, "rollout-")
	const rolloutTimestampLength = len("2026-01-01T00-00-00")
	if len(rest) > rolloutTimestampLength+1 {
		separator := rest[rolloutTimestampLength]
		if separator == '_' || separator == '-' {
			return strings.TrimSpace(rest[rolloutTimestampLength+1:])
		}
	}
	if separator := strings.Index(rest, "_"); separator >= 0 && separator < len(rest)-1 {
		return strings.TrimSpace(rest[separator+1:])
	}
	return ""
}

func codexDiagnostic(code string, message string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessCodex,
		Severity: "warning",
		Code:     code,
		Message:  message,
	}
}
