package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

type evidenceCandidate struct {
	stored evidence.Stored
	fact   publication.Fact
	code   string
	codex  *codexCandidate
}

// ProcessEvidence is deterministic and has no database, filesystem or Git reads.
// All interpretation happens here, after the server durably accepts evidence.
func ProcessEvidence(ctx context.Context, records []evidence.Stored) (evidence.Projection, error) {
	projection := evidence.Projection{}
	var candidates []*evidenceCandidate
	outcomes := make(map[string]evidence.Outcome, len(records))
	for _, stored := range records {
		if err := ctx.Err(); err != nil {
			return projection, err
		}
		outcome := evidence.Outcome{EvidenceID: stored.ID, Disposition: "context"}
		if !evidence.UsageRecord(stored.Record.Harness, stored.Record.Format, stored.Record.Data) {
			outcomes[stored.ID] = outcome
			continue
		}
		candidate, ok := parseEvidence(ctx, stored, records)
		if !ok {
			outcome.Disposition = "ambiguous"
			outcome.Code = "unusable_usage"
			outcomes[stored.ID] = outcome
			continue
		}
		candidates = append(candidates, &candidate)
	}
	resolveCodexEvidence(candidates, records)
	groups := map[string][]*evidenceCandidate{}
	for _, candidate := range candidates {
		groups[candidate.fact.ID] = append(groups[candidate.fact.ID], candidate)
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		group := groups[id]
		sort.Slice(group, func(i, j int) bool {
			if group[i].fact.OccurredAtMs != group[j].fact.OccurredAtMs {
				return group[i].fact.OccurredAtMs < group[j].fact.OccurredAtMs
			}
			return group[i].stored.ID < group[j].stored.ID
		})
		selected := group[len(group)-1]
		for _, item := range group {
			if item.code == "" {
				selected = item
			}
		}
		// OpenCode v1/v2 overlap names one native message; v2 is authoritative.
		if selected.fact.Harness == "opencode" {
			for _, item := range group {
				if item.stored.Record.Format == "opencode-v2" {
					selected = item
				}
			}
		}
		conflict := false
		for _, item := range group {
			if item.code != "" {
				continue
			}
			if selected.fact.Harness == "opencode" && selected.stored.Record.Format == "opencode-v2" && item.stored.Record.Format == "opencode-v1" {
				continue
			}
			if !sameEvidenceUsage(item.fact, selected.fact) {
				if selected.fact.Harness != "claude-code" || selected.fact.NativeRequestID == "" || item.fact.OccurredAtMs == selected.fact.OccurredAtMs {
					conflict = true
				}
			}
		}
		if conflict {
			selected.code = "conflicting_native_identity"
		}
		var provenance []string
		for _, item := range group {
			outcome := evidence.Outcome{EvidenceID: item.stored.ID, FactID: id, Disposition: "duplicate"}
			if conflict {
				outcome.Disposition = "ambiguous"
				outcome.Code = "conflicting_native_identity"
			} else if item.code == "known_duplicate" {
				outcome.Disposition = "duplicate"
			} else if item.code == "stale_snapshot" {
				outcome.Disposition = "superseded"
			} else if item.code != "" {
				outcome.Disposition = "ambiguous"
				outcome.Code = item.code
			} else if item == selected {
				outcome.Disposition = "processed"
			} else if item.fact.OccurredAtMs < selected.fact.OccurredAtMs || item.stored.Record.Format != selected.stored.Record.Format {
				outcome.Disposition = "superseded"
			}
			outcomes[item.stored.ID] = outcome
			provenance = append(provenance, item.stored.ID)
		}
		if selected.code == "" {
			projection.Contributions = append(projection.Contributions, evidence.Contribution{Fact: selected.fact, EvidenceIDs: provenance})
		} else if selected.code != "known_duplicate" && selected.code != "stale_snapshot" {
			projection.Estimates = append(projection.Estimates, evidence.Estimate{EvidenceID: selected.stored.ID, Code: selected.code, Fact: selected.fact})
		}
	}
	for _, stored := range records {
		projection.Outcomes = append(projection.Outcomes, outcomes[stored.ID])
	}
	return projection, nil
}

func sameEvidenceUsage(left, right publication.Fact) bool {
	// Locations are enrichment, not contribution identity or counter revisions.
	left.Location = nil
	right.Location = nil
	if left.Harness == "claude-code" {
		left.OccurredAtMs = 0
		right.OccurredAtMs = 0
		left.Revision = nil
		right.Revision = nil
	}
	return publication.PayloadHash(left) == publication.PayloadHash(right)
}

func parseEvidence(ctx context.Context, stored evidence.Stored, all []evidence.Stored) (evidenceCandidate, bool) {
	r := stored.Record
	record, err := evidence.DecodeData(r.Data)
	if err != nil {
		return evidenceCandidate{}, false
	}
	source := Source{Harness: Harness(r.Harness), ID: r.SourceID, Kind: r.Format}
	options := SyncOptions{Now: time.UnixMilli(0)}
	var raw RawTokenFact
	var diagnostics []Diagnostic
	var ok bool
	var codex *codexCandidate
	switch r.Harness {
	case "opencode":
		var envelope struct {
			Data    json.RawMessage `json:"data"`
			Created *int64          `json:"time_created"`
		}
		if json.Unmarshal(r.Data, &envelope) != nil {
			return evidenceCandidate{}, false
		}
		session := sql.NullString{String: evidence.String(r.Data, "session_id")}
		session.Valid = session.String != ""
		created := sql.NullInt64{}
		if envelope.Created != nil {
			created = sql.NullInt64{Int64: *envelope.Created, Valid: true}
		}
		if r.Format == "opencode-v2" {
			if evidence.String(r.Data, "type") != "assistant" {
				return evidenceCandidate{}, false
			}
			raw, diagnostics, ok = (opencodeSQLiteAdapter{}).factFromV2Message(source, options, evidence.String(r.Data, "id"), session, created, string(envelope.Data))
		} else {
			raw, diagnostics, ok = (opencodeSQLiteAdapter{}).factFromMessage(source, options, evidence.String(r.Data, "id"), session, created, string(envelope.Data))
		}
	case "pi":
		session := piJSONLSessionFile{}
		for _, entry := range r.Context {
			if evidence.String(entry.Data, "type") == "session" {
				session.sessionID = evidence.String(entry.Data, "id")
				session.hasHeader = session.sessionID != ""
			}
		}
		raw, diagnostics, ok = (piJSONLAdapter{}).factFromRecord(ctx, source, options, session, record)
	case "claude-code":
		raw, diagnostics, ok = (claudeCodeJSONLAdapter{}).factFromRecord(ctx, source, options, "", record)
		if ok {
			more, valid := finalizeClaudeCodeFact(&raw)
			diagnostics = append(diagnostics, more...)
			ok = valid
		}
	case "codex":
		state := codexJSONLState{}
		contextRecords := append([]evidence.Context(nil), r.Context...)
		sort.Slice(contextRecords, func(i, j int) bool { return contextRecords[i].Ordinal < contextRecords[j].Ordinal })
		for _, entry := range contextRecords {
			value, decodeErr := evidence.DecodeData(entry.Data)
			if decodeErr != nil {
				continue
			}
			switch evidence.String(entry.Data, "type") {
			case "session_meta":
				state.applySessionMeta(value)
			case "turn_context":
				state.applyTurnContext(value)
			case "event_msg":
				if turn := stringField(nested(value, "payload"), "turn_id"); turn != nil {
					state.turnID = turn
				}
			}
		}
		candidate, more, valid := (&codexJSONLAdapter{}).factFromEvent(ctx, source, options, &state, record, int(r.Ordinal))
		diagnostics = append(diagnostics, more...)
		ok = valid
		if !valid && len(state.pending) > 0 {
			// A later native turn context can resolve records captured before model
			// metadata. Require the same source lineage and nearest following record.
			var next *evidence.Stored
			for i := range all {
				item := &all[i]
				if item.Record.SourceID == r.SourceID && item.Record.Lineage == r.Lineage && item.Record.Ordinal > r.Ordinal && evidence.String(item.Record.Data, "type") == "turn_context" && (next == nil || item.Record.Ordinal < next.Record.Ordinal) {
					next = item
				}
			}
			if next != nil {
				value, _ := evidence.DecodeData(next.Record.Data)
				state.applyTurnContext(value)
			}
			flushed, more := state.flushPending(state.model != nil)
			diagnostics = append(diagnostics, more...)
			if len(flushed) > 0 {
				candidate = flushed[0]
				ok = true
			}
		}
		if ok {
			raw = candidate.fact
			codex = &candidate
		}
	}
	if !ok || raw.SessionID == nil || raw.OccurredAtMs == nil || !publication.ValidTimestampMs(*raw.OccurredAtMs) {
		return evidenceCandidate{}, false
	}
	row := rawTokenRow{Harness: raw.Harness, Provider: sql.NullString{String: stringValueOrEmpty(raw.Provider), Valid: raw.Provider != nil}, Model: sql.NullString{String: stringValueOrEmpty(raw.Model), Valid: raw.Model != nil}, UsageScope: raw.UsageScope, Quality: raw.Quality}
	provider, providerSource := canonicalProvider(row)
	total, valid := tokenComponentSum(raw.InputTokens, raw.OutputTokens, raw.ReasoningTokens, raw.CacheReadTokens, raw.CacheWriteTokens)
	if !valid {
		return evidenceCandidate{}, false
	}
	fact := publication.Fact{Harness: r.Harness, Session: publication.Session{Harness: r.Harness, NativeID: *raw.SessionID, FirstOccurredAtMs: *raw.OccurredAtMs, LastOccurredAtMs: *raw.OccurredAtMs}, OccurredAtMs: *raw.OccurredAtMs, Provider: provider, ProviderSource: providerSource, Model: canonicalModel(row.Model, provider), UsageScope: raw.UsageScope, Quality: raw.Quality, Countable: countable(row) == 1, InputTokens: intValueOrZero(raw.InputTokens), OutputTokens: intValueOrZero(raw.OutputTokens), ReasoningTokens: intValueOrZero(raw.ReasoningTokens), CacheReadTokens: intValueOrZero(raw.CacheReadTokens), CacheWriteTokens: intValueOrZero(raw.CacheWriteTokens), TotalTokens: total}
	code := ""
	if raw.MessageID != nil && *raw.MessageID != "" {
		fact.Message = &publication.Message{NativeID: *raw.MessageID, OccurredAtMs: *raw.OccurredAtMs}
	} else {
		code = "missing_native_identity"
		fact.Message = &publication.Message{NativeID: "estimate:" + evidence.Tuple(r.SourceID, r.Lineage, r.Ordinal), OccurredAtMs: *raw.OccurredAtMs}
	}
	if r.Harness == "claude-code" {
		fact.NativeRequestID = claudeCodeRequestID(raw.MetadataJSON)
		if fact.NativeRequestID != "" {
			fact.Revision = &publication.SourceRevision{Rule: publication.ClaudeRevisionRule, Value: fact.OccurredAtMs}
		}
	}
	if r.Location != nil {
		l := r.Location
		fact.Location = &publication.Location{DirectoryKey: l.DirectoryKey, DirectoryName: l.DirectoryName, RepositoryKey: l.RepositoryKey, RepositoryName: l.RepositoryName, RepositorySource: l.RepositorySource}
	}
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Code, "negative") || strings.Contains(diagnostic.Code, "invalid") || strings.Contains(diagnostic.Code, "inconsistent") {
			code = "invalid_source_counters"
		}
	}
	for _, diagnostic := range r.Diagnostics {
		if diagnostic != "native_subagent_present" {
			code = "invalid_source_fields"
		}
	}
	if conflictingNativeAliases(r) {
		code = "conflicting_native_fields"
	}
	if codex != nil {
		complete := func(snapshot *codexUsageSnapshot) bool {
			return snapshot != nil && snapshot.Input != nil && snapshot.Output != nil && snapshot.Total != nil
		}
		codex.replayValid = codex.replayValid && complete(codex.snapshot.Last) && complete(codex.snapshot.Total) && code == ""
	}
	publication.SetIDs(&fact)
	if publication.ValidateFact(fact) != nil {
		return evidenceCandidate{}, false
	}
	return evidenceCandidate{stored: stored, fact: fact, code: code, codex: codex}, true
}

func conflictingNativeAliases(record evidence.Record) bool {
	groups := [][]string{}
	switch record.Harness {
	case "claude-code":
		groups = [][]string{{"sessionId", "session_id"}, {"requestId", "request_id"}, {"message.provider", "message.provider_id", "message.providerID"}, {"message.model", "message.model_id", "message.modelID"}, {"message.usage.output_tokens_details.thinking_tokens", "message.usage.output_tokens_details.reasoning_tokens"}}
	case "codex":
		for _, prefix := range []string{"payload.info.last_token_usage.", "payload.info.total_token_usage."} {
			groups = append(groups, []string{prefix + "cached_input_tokens", prefix + "cache_read_input_tokens"})
		}
	}
	object, err := evidence.DecodeData(record.Data)
	if err != nil {
		return true
	}
	for _, group := range groups {
		var previous []byte
		for _, path := range group {
			var value interface{} = object
			for _, part := range strings.Split(path, ".") {
				parent, ok := value.(map[string]interface{})
				if !ok {
					value = nil
					break
				}
				value = parent[part]
			}
			if value == nil || value == "" {
				continue
			}
			encoded, _ := json.Marshal(value)
			if previous != nil && string(encoded) != string(previous) {
				return true
			}
			previous = encoded
		}
	}
	return false
}

func resolveCodexEvidence(candidates []*evidenceCandidate, records []evidence.Stored) {
	metadata := map[string]codexSourceMetadata{}
	for _, stored := range records {
		if stored.Record.Harness != "codex" {
			continue
		}
		contexts := append([]evidence.Context{{Data: stored.Record.Data, Diagnostics: stored.Record.Diagnostics}}, stored.Record.Context...)
		for _, entry := range contexts {
			if evidence.String(entry.Data, "type") != "session_meta" {
				continue
			}
			object, _ := evidence.DecodeData(entry.Data)
			next := codexMetadataFromPayload(nested(object, "payload"), "")
			if next.sessionID == "" {
				continue
			}
			for _, code := range entry.Diagnostics {
				if code == "native_subagent_present" {
					next.fork = true
				}
			}
			if previous, exists := metadata[next.sessionID]; exists && (previous.parentID != next.parentID || previous.fork != next.fork) {
				next.conflict = true
			}
			if previous, exists := metadata[next.sessionID]; exists {
				next.conflict = next.conflict || previous.conflict
			}
			metadata[next.sessionID] = next
		}
	}
	sessions := map[string][]*evidenceCandidate{}
	for _, candidate := range candidates {
		if candidate.codex != nil {
			sessions[candidate.fact.Session.NativeID] = append(sessions[candidate.fact.Session.NativeID], candidate)
		}
	}
	proofs := map[string]map[string]map[string]publication.Fact{}
	visiting := map[string]bool{}
	done := map[string]bool{}
	problems := map[string]string{}
	var resolve func(string)
	resolve = func(session string) {
		if done[session] {
			return
		}
		if visiting[session] {
			problems[session] = "ancestry_cycle"
			return
		}
		visiting[session] = true
		meta, exists := metadata[session]
		problem := ""
		if !exists {
			problem = "missing_session_context"
		}
		if meta.conflict {
			problem = "conflicting_parent"
		}
		proofs[session] = map[string]map[string]publication.Fact{}
		if meta.fork {
			if meta.parentID == "" {
				problem = "missing_parent"
			} else if _, exists := metadata[meta.parentID]; !exists {
				problem = "missing_parent"
			} else {
				resolve(meta.parentID)
				if problems[meta.parentID] != "" || visiting[meta.parentID] {
					problem = "unresolved_parent"
				} else {
					for fingerprint, originals := range proofs[meta.parentID] {
						copyMap := map[string]publication.Fact{}
						for id, fact := range originals {
							copyMap[id] = fact
						}
						proofs[session][fingerprint] = copyMap
					}
				}
			}
		}
		if problems[session] != "" {
			problem = problems[session]
		}
		items := sessions[session]
		sort.Slice(items, func(i, j int) bool {
			if items[i].fact.OccurredAtMs != items[j].fact.OccurredAtMs {
				return items[i].fact.OccurredAtMs < items[j].fact.OccurredAtMs
			}
			if items[i].stored.Record.Ordinal != items[j].stored.Record.Ordinal {
				return items[i].stored.Record.Ordinal < items[j].stored.Record.Ordinal
			}
			return items[i].stored.ID < items[j].stored.ID
		})
		var previousTotal *codexTokenCounts
		previousTurn := ""
		seen := map[string]*evidenceCandidate{}
		for _, item := range items {
			candidate := item.codex
			fingerprint := candidate.replayFingerprint()
			inherited := map[string]publication.Fact{}
			for id, original := range proofs[session][fingerprint] {
				if original.Session.NativeID != session {
					inherited[id] = original
				}
			}
			if len(inherited) == 1 {
				for _, original := range inherited {
					item.fact = original
					item.code = "known_duplicate"
				}
				continue
			}
			if previous, exists := seen[item.fact.ID]; exists {
				if previous.code != "" {
					item.code = previous.code
				}
				continue
			}
			seen[item.fact.ID] = item
			turn := stringValueOrEmpty(candidate.turnID)
			if meta.fork && turn != previousTurn {
				previousTotal = nil
			}
			previousTurn = turn
			if candidate.cumulative != nil {
				if previousTotal != nil && previousTotal.equal(*candidate.cumulative) {
					item.code = "known_duplicate"
					continue
				}
				if previousTotal != nil && candidate.cumulative.lessThan(*previousTotal) {
					item.code = "stale_snapshot"
					continue
				}
				previousTotal = candidate.cumulative
			}
			if problem != "" {
				item.code = problem
			} else if fingerprint == "" {
				item.code = "unverifiable_snapshot"
			} else if len(inherited) > 1 {
				item.code = "ambiguous_parent_fact"
			} else if meta.fork {
				// An unmatched snapshot in an ancestor's turn may be copied history.
				for _, originals := range proofs[meta.parentID] {
					for _, original := range originals {
						if original.Message != nil && strings.HasPrefix(original.Message.NativeID, turn+":") {
							item.code = "uncertain_replay_boundary"
						}
					}
				}
			}
			if item.code == "" && fingerprint != "" {
				if proofs[session][fingerprint] == nil {
					proofs[session][fingerprint] = map[string]publication.Fact{}
				}
				proofs[session][fingerprint][item.fact.ID] = item.fact
			}
		}
		problems[session] = problem
		visiting[session] = false
		done[session] = true
	}
	keys := make([]string, 0, len(sessions))
	for key := range sessions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		resolve(key)
	}
}
