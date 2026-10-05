package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

type publishableToken struct {
	Fact          publication.Fact
	RawFactID     int64
	RawFactKey    string
	HasTime       bool
	SessionSource string
}

// Canonical facts and their immutable journal snapshots commit together. The
// initial implementation scans the canonical snapshot inside the writer tx;
// collectorstore.Record suppresses unchanged payloads.
func journalCanonicalFacts(ctx context.Context, tx *sql.Tx, nowMs int64) (int, error) {
	facts, err := loadPublicationFacts(ctx, tx)
	if err != nil {
		return 0, err
	}
	byID := make(map[string]string, len(facts))
	ambiguous := make(map[string]bool)
	for _, candidate := range facts {
		hash := publication.PayloadHash(candidate.Fact)
		if previous, exists := byID[candidate.Fact.ID]; exists && previous != hash {
			ambiguous[candidate.Fact.ID] = true
		}
		byID[candidate.Fact.ID] = hash
	}
	var diagnostics int
	for _, candidate := range facts {
		code := ""
		switch {
		case !candidate.HasTime:
			code = "publication_missing_occurrence"
		case (candidate.Fact.Harness == string(HarnessPi) || candidate.Fact.Harness == string(HarnessClaudeCode)) && candidate.SessionSource != "native":
			code = "publication_ambiguous_session_identity"
		case candidate.Fact.Message == nil && candidate.Fact.NativeRequestID == "":
			code = "publication_ambiguous_native_identity"
		case ambiguous[candidate.Fact.ID]:
			code = "publication_unsupported_native_revision"
		default:
			if err := publication.ValidateFact(candidate.Fact); err != nil {
				code = "publication_invalid_normalized_fact"
			}
		}
		if code != "" {
			created, err := insertDiagnostic(ctx, tx, Diagnostic{Harness: Harness(candidate.Fact.Harness),
				RawFactKey: candidate.RawFactKey, Severity: "warning", Code: code,
				Message: "canonical token fact withheld from publication because stable normalized evidence is unavailable"}, &candidate.RawFactID, nil, nowMs)
			if err != nil {
				return diagnostics, err
			}
			if created {
				diagnostics++
			}
			continue
		}
		if _, err := collectorstore.Record(ctx, tx, candidate.Fact, nowMs); err != nil {
			return diagnostics, fmt.Errorf("journal canonical token fact: %w", err)
		}
	}
	return diagnostics, nil
}

func loadPublicationFacts(ctx context.Context, tx *sql.Tx) ([]publishableToken, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT c.harness, s.session_id, s.first_seen_at_ms, s.last_seen_at_ms,
			m.harness_message_id, m.occurred_at_ms, c.recorded_at_ms,
			c.provider, c.provider_source, c.model, c.usage_scope, c.quality, c.is_countable,
			c.input_tokens, c.output_tokens, c.reasoning_tokens, c.cache_read_tokens,
			c.cache_write_tokens, c.total_tokens, r.id, r.raw_fact_key, r.occurred_at_ms, r.metadata_json,
			l.directory_key, l.directory_name, l.repository_key, l.repository_name, l.repository_source
		FROM canonical_token_usage c
		JOIN canonical_sessions s ON s.id = c.session_id
		LEFT JOIN canonical_messages m ON m.id = c.message_id
		JOIN raw_token_usage r ON r.id = c.primary_raw_fact_id
		LEFT JOIN usage_locations l ON l.id = c.location_id
		ORDER BY c.semantic_key
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var facts []publishableToken
	for rows.Next() {
		var candidate publishableToken
		fact := &candidate.Fact
		var messageID, metadata sql.NullString
		var messageTime, rawTime sql.NullInt64
		var directoryKey, directoryName, repositoryKey, repositoryName, repositorySource sql.NullString
		if err := rows.Scan(&fact.Harness, &fact.Session.NativeID, &fact.Session.FirstOccurredAtMs, &fact.Session.LastOccurredAtMs,
			&messageID, &messageTime, &fact.OccurredAtMs,
			&fact.Provider, &fact.ProviderSource, &fact.Model, &fact.UsageScope, &fact.Quality, &fact.Countable,
			&fact.InputTokens, &fact.OutputTokens, &fact.ReasoningTokens, &fact.CacheReadTokens,
			&fact.CacheWriteTokens, &fact.TotalTokens, &candidate.RawFactID, &candidate.RawFactKey, &rawTime, &metadata,
			&directoryKey, &directoryName, &repositoryKey, &repositoryName, &repositorySource); err != nil {
			return nil, err
		}
		candidate.HasTime = rawTime.Valid
		if metadata.Valid {
			candidate.SessionSource = sourceSessionIdentity(&metadata.String)
		}
		fact.Session.Harness = fact.Harness
		if messageID.Valid && messageID.String != "" {
			fact.Message = &publication.Message{NativeID: messageID.String, OccurredAtMs: messageTime.Int64}
		}
		if fact.Harness == string(HarnessClaudeCode) {
			if metadata.Valid {
				fact.NativeRequestID = claudeCodeRequestID(&metadata.String)
			}
			if fact.NativeRequestID != "" && fact.Message != nil && rawTime.Valid {
				fact.Revision = &publication.SourceRevision{Rule: publication.ClaudeRevisionRule, Value: rawTime.Int64}
			}
		}
		if directoryKey.String != "" || repositoryKey.String != "" {
			directoryLabel := ""
			if directoryKey.String != "" {
				directoryLabel = locationLabel(path.Base(strings.ReplaceAll(directoryName.String, "\\", "/")))
			}
			fact.Location = &publication.Location{DirectoryKey: directoryKey.String, DirectoryName: directoryLabel,
				RepositoryKey: repositoryKey.String, RepositoryName: repositoryName.String, RepositorySource: repositorySource.String}
		}
		publication.SetIDs(fact)
		facts = append(facts, candidate)
	}
	return facts, rows.Err()
}
