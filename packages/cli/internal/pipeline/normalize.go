package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
	"slices"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

type rawTokenRow struct {
	WorkID           sql.NullInt64
	ID               int64
	RawFactKey       string
	Harness          Harness
	SourceID         string
	ObservedAtMs     int64
	OccurredAtMs     sql.NullInt64
	SessionID        sql.NullString
	MessageID        sql.NullString
	Provider         sql.NullString
	Model            sql.NullString
	UsageScope       string
	Quality          string
	InputTokens      sql.NullInt64
	OutputTokens     sql.NullInt64
	ReasoningTokens  sql.NullInt64
	CacheReadTokens  sql.NullInt64
	CacheWriteTokens sql.NullInt64
	TotalTokens      sql.NullInt64
	LastRunID        sql.NullInt64
	LocationID       sql.NullInt64
	MetadataJSON     sql.NullString
}

type canonicalTokenValues struct {
	Key              string
	RecordedAtMs     int64
	Harness          Harness
	SessionDBID      int64
	MessageDBID      interface{}
	Provider         string
	ProviderSource   string
	Model            string
	UsageScope       string
	Quality          string
	Countable        int
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
	RawFactID        int64
	IngestRunID      interface{}
	LocationID       interface{}
}

var providerAliases = processor.ProviderAliases
var modelPrefixes = processor.ModelPrefixes

func normalizationRuleSignature(harness Harness) string {
	return processor.NormalizationRuleSignature(string(harness))
}

func staleNormalizationRules(ctx context.Context, database *sql.DB, harnesses []Harness) ([]Harness, error) {
	if len(harnesses) == 0 {
		harnesses = SupportedHarnesses
	}
	var stale []Harness
	for _, harness := range harnesses {
		var signature string
		err := database.QueryRowContext(ctx, "SELECT rule_signature FROM normalization_rule_state WHERE harness = ?", harness).Scan(&signature)
		if err == sql.ErrNoRows || (err == nil && signature != normalizationRuleSignature(harness)) {
			stale = append(stale, harness)
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	return stale, nil
}

func Normalize(ctx context.Context, options NormalizeOptions) (Summary, error) {
	if options.Sources == nil {
		var err error
		options.Sources, err = ResolveSources("")
		if err != nil {
			return Summary{}, err
		}
	}
	summary := Summary{}
	compatibility, err := db.InspectEvidenceCompatibility(ctx, options.DBPath)
	if err != nil {
		return summary, err
	}
	if options.DryRun {
		if needsRecovery(compatibility) {
			return previewSync(ctx, normalizationRecoveryOptions(options), compatibility)
		}
		database, err := db.OpenEvidencePreview(ctx, options.DBPath)
		if err != nil {
			return summary, err
		}
		defer func() { _ = database.Close() }()
		rows, err := loadPendingTokenRows(ctx, database, options.Harnesses)
		if err != nil {
			return summary, err
		}
		for _, row := range rows {
			if rawTokenNormalizationDiagnostic(row) != nil {
				summary.Diagnostics++
				continue
			}
			summary.Canonical++
		}
		return summary, nil
	}

	release, err := db.AcquireWriterLock(ctx, options.DBPath)
	if err != nil {
		return summary, recoveryFailure(compatibility, err)
	}
	defer release()
	if err := db.UpgradeEvidence(ctx, options.DBPath); err != nil {
		return summary, err
	}
	compatibility, err = db.InspectCompatibility(ctx, options.DBPath)
	if err != nil {
		return summary, err
	}

	if needsRecovery(compatibility) {
		return recoverDatabase(ctx, normalizationRecoveryOptions(options), compatibility)
	}
	database, _, err := db.CreateIfMissing(options.DBPath)
	if err != nil {
		return summary, err
	}
	defer func() { _ = database.Close() }()
	return normalizePrepared(ctx, database, options)
}

// normalizePrepared shares the sync caller's connection and writer lock.
func normalizePrepared(ctx context.Context, database *sql.DB, options NormalizeOptions) (Summary, error) {
	summary := Summary{}
	rows, err := loadPendingTokenRows(ctx, database, options.Harnesses)
	if err != nil {
		return summary, err
	}
	staleHarnesses, err := staleNormalizationRules(ctx, database, options.Harnesses)
	if err != nil {
		return summary, err
	}
	if len(rows) == 0 && len(staleHarnesses) == 0 {
		return summary, nil
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return summary, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, row := range rows {
		count, diagnostic, err := normalizeRawTokenRow(ctx, tx, row, options)
		if err != nil {
			return summary, err
		}
		if err := completeNormalizationWork(ctx, tx, row); err != nil {
			return summary, err
		}
		summary.Canonical += count
		summary.Diagnostics += diagnostic
	}
	if len(staleHarnesses) > 0 {
		if err := refreshCanonicalIdentifiers(ctx, tx, staleHarnesses); err != nil {
			return summary, err
		}
		for _, harness := range staleHarnesses {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO normalization_rule_state (harness, rule_signature, updated_at_ms)
				VALUES (?, ?, ?)
				ON CONFLICT (harness) DO UPDATE SET rule_signature = excluded.rule_signature, updated_at_ms = excluded.updated_at_ms
			`, harness, normalizationRuleSignature(harness), syncNowMs(options.Now)); err != nil {
				return summary, err
			}
		}
	}
	diagnostics, err := journalCanonicalFacts(ctx, tx, nowMs(options))
	if err != nil {
		return summary, err
	}
	summary.Diagnostics += diagnostics
	if err := db.AdvanceAnalyticsRevision(ctx, tx); err != nil {
		return summary, err
	}
	if err := commitSyncTransaction(ctx, tx); err != nil {
		return summary, err
	}
	committed = true
	return summary, nil
}

func refreshCanonicalIdentifiers(ctx context.Context, runner sqlRunner, harnesses []Harness) error {
	for harness, aliases := range providerAliases {
		if len(harnesses) > 0 && !slices.Contains(harnesses, harness) {
			continue
		}
		for source, canonical := range aliases {
			if _, err := runner.ExecContext(ctx, `
				UPDATE canonical_token_usage AS c
				SET provider = ?
				FROM raw_token_usage AS r
				WHERE c.primary_raw_fact_id = r.id AND c.harness = ?
					AND r.provider = ? AND c.provider = ?
			`, canonical, harness, source, source); err != nil {
				return err
			}
		}
	}
	for provider, prefix := range modelPrefixes {
		query := `
			UPDATE canonical_token_usage AS c
			SET model = substr(r.model, length(?) + 1)
			FROM raw_token_usage AS r
			WHERE c.primary_raw_fact_id = r.id AND c.provider = ? AND r.provider IN (?, 'fireworks-ai')
				AND substr(r.model, 1, length(?)) = ? AND length(r.model) > length(?)
				AND c.model != substr(r.model, length(?) + 1)
		`
		args := []interface{}{prefix, provider, provider, prefix, prefix, prefix, prefix}
		if len(harnesses) > 0 {
			placeholders := make([]string, len(harnesses))
			for i, harness := range harnesses {
				placeholders[i] = "?"
				args = append(args, harness)
			}
			query += " AND c.harness IN (" + strings.Join(placeholders, ",") + ")"
		}
		if _, err := runner.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func loadPendingTokenRows(ctx context.Context, database *sql.DB, harnesses []Harness) ([]rawTokenRow, error) {
	var args []interface{}
	where := "WHERE q.domain = ?"
	args = append(args, db.DomainTokenUsage)
	if len(harnesses) > 0 {
		parts := make([]string, len(harnesses))
		for i, harness := range harnesses {
			parts[i] = "?"
			args = append(args, harness)
		}
		where += " AND r.harness IN (" + strings.Join(parts, ",") + ")"
	}

	query := `
		SELECT
			q.id,
			r.id, r.raw_fact_key, r.harness, r.source_id, r.observed_at_ms, r.occurred_at_ms,
			r.session_id, r.message_id, r.provider, r.model, r.usage_scope, r.quality,
			r.input_tokens, r.output_tokens, r.reasoning_tokens, r.cache_read_tokens, r.cache_write_tokens, r.total_tokens,
			r.location_id, r.metadata_json,
			(
				SELECT ro.ingest_run_id
				FROM raw_observations ro
				WHERE ro.raw_fact_id = r.id
				ORDER BY ro.id DESC
				LIMIT 1
			) AS ingest_run_id
		FROM normalization_work_queue q
		JOIN raw_token_usage r ON r.id = q.raw_fact_id
		` + where + `
		ORDER BY q.id
	`
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []rawTokenRow
	for rows.Next() {
		var row rawTokenRow
		var harness string
		if err := rows.Scan(
			&row.WorkID,
			&row.ID,
			&row.RawFactKey,
			&harness,
			&row.SourceID,
			&row.ObservedAtMs,
			&row.OccurredAtMs,
			&row.SessionID,
			&row.MessageID,
			&row.Provider,
			&row.Model,
			&row.UsageScope,
			&row.Quality,
			&row.InputTokens,
			&row.OutputTokens,
			&row.ReasoningTokens,
			&row.CacheReadTokens,
			&row.CacheWriteTokens,
			&row.TotalTokens,
			&row.LocationID,
			&row.MetadataJSON,
			&row.LastRunID,
		); err != nil {
			return nil, err
		}
		row.Harness = Harness(harness)
		result = append(result, row)
	}
	return result, rows.Err()
}

func completeNormalizationWork(ctx context.Context, runner sqlRunner, row rawTokenRow) error {
	if !row.WorkID.Valid {
		return nil
	}
	_, err := runner.ExecContext(ctx, `
		DELETE FROM normalization_work_queue
		WHERE id = ?
	`, row.WorkID.Int64)
	return err
}

func normalizeRawTokenRow(ctx context.Context, runner sqlRunner, row rawTokenRow, options NormalizeOptions) (int, int, error) {
	if diagnostic := rawTokenNormalizationDiagnostic(row); diagnostic != nil {
		inserted, err := insertDiagnostic(ctx, runner, *diagnostic, &row.ID, rawRunPointer(row), nowMs(options))
		if err != nil {
			return 0, 0, err
		}
		if inserted {
			if err := incrementIngestRunCounts(ctx, runner, row.LastRunID, 0, 1); err != nil {
				return 0, 0, err
			}
			return 0, 1, nil
		}
		return 0, 0, nil
	}

	sessionDBID, err := upsertCanonicalSession(ctx, runner, row)
	if err != nil {
		return 0, 0, err
	}
	messageDBID, err := upsertCanonicalMessage(ctx, runner, row, sessionDBID)
	if err != nil {
		return 0, 0, err
	}
	inserted, err := upsertCanonicalTokenUsage(ctx, runner, row, sessionDBID, messageDBID)
	if err != nil {
		return 0, 0, err
	}
	if inserted {
		if err := incrementIngestRunCounts(ctx, runner, row.LastRunID, 1, 0); err != nil {
			return 0, 0, err
		}
		return 1, 0, nil
	}
	return 0, 0, nil
}

// Check source evidence before creating session/message envelopes. Filename
// guesses remain raw-only; they cannot widen or conflict with native identity.
func rawTokenNormalizationDiagnostic(row rawTokenRow) *Diagnostic {
	var code, message string
	switch {
	case !row.SessionID.Valid || strings.TrimSpace(row.SessionID.String) == "":
		code, message = "missing_session", "raw token fact skipped because no stable session identity is available"
	case !publication.ValidTimestampMs(canonicalTime(row)):
		code, message = "invalid_occurrence", "raw token fact skipped because its timestamp is outside the supported range"
	case (row.Harness == HarnessPi || row.Harness == HarnessClaudeCode) && sourceSessionIdentity(rawMetadataPointer(row)) != "native":
		code, message = "publication_ambiguous_session_identity", "raw token fact withheld from canonical data because native session evidence is unavailable"
	default:
		return nil
	}
	return &Diagnostic{Harness: row.Harness, RawFactKey: row.RawFactKey, Severity: "warning", Code: code, Message: message}
}

func upsertCanonicalSession(ctx context.Context, runner sqlRunner, row rawTokenRow) (int64, error) {
	key := nativeTupleHash("session", string(row.Harness), row.SessionID.String)
	timestamp := canonicalTime(row)
	_, err := runner.ExecContext(ctx, `
		INSERT INTO canonical_sessions (
			semantic_key, harness, session_id, first_seen_at_ms, last_seen_at_ms, primary_raw_fact_id
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(semantic_key) DO UPDATE SET
			first_seen_at_ms = MIN(first_seen_at_ms, excluded.first_seen_at_ms),
			last_seen_at_ms = MAX(last_seen_at_ms, excluded.last_seen_at_ms),
			primary_raw_fact_id = COALESCE(primary_raw_fact_id, excluded.primary_raw_fact_id)
	`, key, row.Harness, row.SessionID.String, timestamp, timestamp, row.ID)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := runner.QueryRowContext(ctx, "SELECT id FROM canonical_sessions WHERE semantic_key = ?", key).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func upsertCanonicalMessage(ctx context.Context, runner sqlRunner, row rawTokenRow, sessionDBID int64) (*int64, error) {
	if !row.MessageID.Valid || strings.TrimSpace(row.MessageID.String) == "" {
		return nil, nil
	}
	key := nativeTupleHash("message", string(row.Harness), row.SessionID.String, row.MessageID.String)
	_, err := runner.ExecContext(ctx, `
		INSERT INTO canonical_messages (
			semantic_key, session_id, harness, harness_message_id, occurred_at_ms, primary_raw_fact_id
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(semantic_key) DO UPDATE SET
			occurred_at_ms = COALESCE(occurred_at_ms, excluded.occurred_at_ms),
			primary_raw_fact_id = COALESCE(primary_raw_fact_id, excluded.primary_raw_fact_id)
	`, key, sessionDBID, row.Harness, row.MessageID.String, nullableNullInt(row.OccurredAtMs), row.ID)
	if err != nil {
		return nil, err
	}
	var id int64
	if err := runner.QueryRowContext(ctx, "SELECT id FROM canonical_messages WHERE semantic_key = ?", key).Scan(&id); err != nil {
		return nil, err
	}
	return &id, nil
}

func upsertCanonicalTokenUsage(ctx context.Context, runner sqlRunner, row rawTokenRow, sessionDBID int64, messageDBID *int64) (bool, error) {
	values := canonicalTokenValuesFor(row, sessionDBID, messageDBID)
	if row.Harness == HarnessClaudeCode && row.MessageID.Valid && row.MessageID.String != "" {
		apply, err := acceptClaudeCodeCanonicalRevision(ctx, runner, values, claudeCodeRequestID(rawMetadataPointer(row)) != "")
		if err != nil || !apply {
			return false, err
		}
	}
	result, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO canonical_token_usage (
			semantic_key, recorded_at_ms, harness, session_id, message_id, provider, provider_source, model, usage_scope, quality,
			is_countable, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens,
			total_tokens, primary_raw_fact_id, ingest_run_id, location_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, values.Key, values.RecordedAtMs, values.Harness, values.SessionDBID, values.MessageDBID, values.Provider, values.ProviderSource, values.Model,
		values.UsageScope, values.Quality, values.Countable, values.InputTokens, values.OutputTokens, values.ReasoningTokens,
		values.CacheReadTokens, values.CacheWriteTokens, values.TotalTokens, values.RawFactID, values.IngestRunID, values.LocationID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	_, err = runner.ExecContext(ctx, `
		UPDATE canonical_token_usage
		SET recorded_at_ms = ?,
			provider = ?,
			provider_source = ?,
			model = ?,
			quality = ?,
			is_countable = ?,
			input_tokens = ?,
			output_tokens = ?,
			reasoning_tokens = ?,
			cache_read_tokens = ?,
			cache_write_tokens = ?,
			total_tokens = ?,
			primary_raw_fact_id = ?,
			ingest_run_id = ?,
			location_id = ?
		WHERE semantic_key = ?
	`, values.RecordedAtMs, values.Provider, values.ProviderSource, values.Model, values.Quality, values.Countable, values.InputTokens, values.OutputTokens,
		values.ReasoningTokens, values.CacheReadTokens, values.CacheWriteTokens, values.TotalTokens, values.RawFactID, values.IngestRunID,
		values.LocationID, values.Key)
	return false, err
}

// The adapter's occurrence timestamp is source revision evidence for a native
// Claude request. Local collection order must never replace a newer snapshot.
func acceptClaudeCodeCanonicalRevision(ctx context.Context, runner sqlRunner, next canonicalTokenValues, hasRevision bool) (bool, error) {
	var previous canonicalTokenValues
	err := runner.QueryRowContext(ctx, `
		SELECT recorded_at_ms, provider, provider_source, model, quality, is_countable,
			input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens
		FROM canonical_token_usage WHERE semantic_key = ?
	`, next.Key).Scan(&previous.RecordedAtMs, &previous.Provider, &previous.ProviderSource, &previous.Model,
		&previous.Quality, &previous.Countable, &previous.InputTokens, &previous.OutputTokens,
		&previous.ReasoningTokens, &previous.CacheReadTokens, &previous.CacheWriteTokens, &previous.TotalTokens)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !hasRevision && previous.RecordedAtMs != next.RecordedAtMs {
		return false, fmt.Errorf("claude code message has no native request revision evidence")
	}
	if previous.RecordedAtMs > next.RecordedAtMs {
		return false, nil
	}
	if previous.RecordedAtMs == next.RecordedAtMs && !sameCanonicalUsage(previous, next) {
		return false, fmt.Errorf("claude code native request has conflicting usage at the same source timestamp")
	}
	return true, nil
}

func sameCanonicalUsage(left, right canonicalTokenValues) bool {
	return left.Provider == right.Provider && left.ProviderSource == right.ProviderSource &&
		left.Model == right.Model && left.Quality == right.Quality && left.Countable == right.Countable &&
		left.InputTokens == right.InputTokens && left.OutputTokens == right.OutputTokens &&
		left.ReasoningTokens == right.ReasoningTokens && left.CacheReadTokens == right.CacheReadTokens &&
		left.CacheWriteTokens == right.CacheWriteTokens && left.TotalTokens == right.TotalTokens
}

func canonicalTokenValuesFor(row rawTokenRow, sessionDBID int64, messageDBID *int64) canonicalTokenValues {
	provider, providerSource := canonicalProvider(row)
	return canonicalTokenValues{
		Key:              canonicalTokenKey(row),
		RecordedAtMs:     canonicalTime(row),
		Harness:          row.Harness,
		SessionDBID:      sessionDBID,
		MessageDBID:      nullableInt64Ptr(messageDBID),
		Provider:         provider,
		ProviderSource:   providerSource,
		Model:            canonicalModel(row.Model, provider),
		UsageScope:       row.UsageScope,
		Quality:          row.Quality,
		Countable:        countable(row),
		InputTokens:      nullIntValue(row.InputTokens),
		OutputTokens:     nullIntValue(row.OutputTokens),
		ReasoningTokens:  nullIntValue(row.ReasoningTokens),
		CacheReadTokens:  nullIntValue(row.CacheReadTokens),
		CacheWriteTokens: nullIntValue(row.CacheWriteTokens),
		TotalTokens:      canonicalTotal(row),
		RawFactID:        row.ID,
		IngestRunID:      nullableNullInt(row.LastRunID),
		LocationID:       nullableNullInt(row.LocationID),
	}
}

func insertDiagnostic(ctx context.Context, runner sqlRunner, diagnostic Diagnostic, rawID *int64, runID *int64, recordedAtMs int64) (bool, error) {
	keyParts := []string{
		string(diagnostic.Harness),
		diagnostic.RawFactKey,
		int64PtrValue(rawID),
		int64PtrValue(runID),
		diagnostic.Code,
	}
	result, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO normalization_diagnostics (
			diagnostic_key, recorded_at_ms, harness, raw_fact_id, ingest_run_id, severity, code, message, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, stableHash(strings.Join(keyParts, "|")), recordedAtMs, diagnostic.Harness, nullableInt64Ptr(rawID), nullableInt64Ptr(runID), diagnostic.Severity, diagnostic.Code, diagnostic.Message, nullableString(diagnostic.MetadataJSON))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func incrementIngestRunCounts(ctx context.Context, runner sqlRunner, runID sql.NullInt64, canonicalCount int, diagnosticCount int) error {
	if !runID.Valid {
		return nil
	}
	_, err := runner.ExecContext(ctx, `
		UPDATE ingest_runs
		SET canonical_count = canonical_count + ?,
			diagnostic_count = diagnostic_count + ?
		WHERE id = ?
	`, canonicalCount, diagnosticCount, runID.Int64)
	return err
}

func canonicalTokenKey(row rawTokenRow) string {
	if row.Harness == HarnessClaudeCode && row.MessageID.Valid && row.MessageID.String != "" {
		return nativeTupleHash("token", string(row.Harness), row.SessionID.String, row.MessageID.String, claudeCodeRequestID(rawMetadataPointer(row)), row.UsageScope)
	}
	parts := []string{
		"token",
		string(row.Harness),
		row.SessionID.String,
		nullStringValue(row.MessageID),
		fmt.Sprint(canonicalTime(row)),
		row.UsageScope,
	}
	return nativeTupleHash(parts...)
}

func rawMetadataPointer(row rawTokenRow) *string {
	if !row.MetadataJSON.Valid {
		return nil
	}
	return &row.MetadataJSON.String
}

func canonicalTime(row rawTokenRow) int64 {
	if row.OccurredAtMs.Valid {
		return row.OccurredAtMs.Int64
	}
	return row.ObservedAtMs
}

func canonicalTotal(row rawTokenRow) int64 {
	if row.TotalTokens.Valid {
		return row.TotalTokens.Int64
	}
	return nullIntValue(row.InputTokens) +
		nullIntValue(row.OutputTokens) +
		nullIntValue(row.ReasoningTokens) +
		nullIntValue(row.CacheReadTokens) +
		nullIntValue(row.CacheWriteTokens)
}

func countable(row rawTokenRow) int {
	if strings.Contains(row.UsageScope, "fallback") {
		return 0
	}
	return 1
}

func canonicalProvider(row rawTokenRow) (string, string) {
	var provider *string
	if row.Provider.Valid {
		provider = &row.Provider.String
	}
	return processor.CanonicalProvider(string(row.Harness), provider)
}
func canonicalModel(sourceModel sql.NullString, provider string) string {
	var model *string
	if sourceModel.Valid {
		model = &sourceModel.String
	}
	return processor.CanonicalModel(model, provider)
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullIntValue(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableNullInt(value sql.NullInt64) interface{} {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableInt64Ptr(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func int64PtrValue(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func rawRunPointer(row rawTokenRow) *int64 {
	if !row.LastRunID.Valid {
		return nil
	}
	return &row.LastRunID.Int64
}

func nowMs(options NormalizeOptions) int64 {
	if options.Now.IsZero() {
		return time.Now().UnixMilli()
	}
	return options.Now.UnixMilli()
}
