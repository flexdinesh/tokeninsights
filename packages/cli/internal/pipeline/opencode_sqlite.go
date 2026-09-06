package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const opencodeSQLiteSourceKind = "opencode-sqlite"

type opencodeSQLiteAdapter struct{}

type opencodeMessageData struct {
	ID         string                `json:"id"`
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

func (a opencodeSQLiteAdapter) Harness() Harness {
	return HarnessOpenCode
}

func (a opencodeSQLiteAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	roots, err := a.discoveryRoots(options)
	if err != nil {
		return nil, err
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
			if isOpenCodeSQLiteDB(root) {
				sources = append(sources, a.source(root, filepath.Dir(root)))
			}
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if isOpenCodeSQLiteDB(path) {
				sources = append(sources, a.source(path, root))
			}
		}
	}
	sort.Slice(sources, func(i int, j int) bool {
		return sources[i].Path < sources[j].Path
	})
	return sources, nil
}

func (a opencodeSQLiteAdapter) discoveryRoots(options DiscoverOptions) ([]string, error) {
	sourceDir := strings.TrimSpace(options.SourceDir)
	if sourceDir != "" {
		harnessDir := filepath.Join(sourceDir, string(HarnessOpenCode))
		if info, err := os.Stat(harnessDir); err == nil && info.IsDir() {
			return []string{harnessDir}, nil
		} else if options.HarnessSubdirOnly {
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			return nil, nil
		}
		return []string{sourceDir}, nil
	}

	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome != "" {
		return []string{filepath.Join(dataHome, "opencode")}, nil
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return nil, nil
	}
	return []string{filepath.Join(home, ".local/share/opencode")}, nil
}

func (a opencodeSQLiteAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness:     HarnessOpenCode,
		ID:          stableHash("opencode-sqlite-db:" + filepath.ToSlash(rel) + ":" + stableHash(path)),
		Kind:        opencodeSQLiteSourceKind,
		Path:        path,
		RawSourceID: stableHash("opencode-sqlite-root:" + stableHash(root)),
	}
}

func (a opencodeSQLiteAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	database, err := openReadOnlySQLite(source.Path)
	if err != nil {
		return nil, nil, err
	}
	defer database.Close()

	v1Exists, err := sqliteTableExists(ctx, database, "message")
	if err != nil {
		return nil, nil, err
	}
	v2Exists, err := sqliteTableExists(ctx, database, "session_message")
	if err != nil {
		return nil, nil, err
	}
	if !v1Exists && !v2Exists {
		return nil, []Diagnostic{opencodeDiagnostic("opencode_sqlite_missing_message_table", "OpenCode SQLite source has no supported message table")}, nil
	}

	var facts []RawTokenFact
	var diagnostics []Diagnostic
	v2Facts := map[string]bool{}
	if v2Exists {
		if err := requireSQLiteColumns(ctx, database, "session_message", []string{"id", "session_id", "type", "time_created", "data"}); err != nil {
			diagnostics = append(diagnostics, opencodeDiagnostic("opencode_sqlite_invalid_schema", "OpenCode SQLite session_message table is missing required columns"))
		} else {
			parsed, parsedDiagnostics, err := a.parseV2Messages(ctx, database, source, options, v2Facts)
			if err != nil {
				return nil, nil, err
			}
			facts = append(facts, parsed...)
			diagnostics = append(diagnostics, parsedDiagnostics...)
		}
	}
	if v1Exists {
		if err := requireSQLiteColumns(ctx, database, "message", []string{"id", "session_id", "time_created", "data"}); err != nil {
			diagnostics = append(diagnostics, opencodeDiagnostic("opencode_sqlite_invalid_schema", "OpenCode SQLite message table is missing required columns"))
		} else {
			parsed, parsedDiagnostics, err := a.parseV1Messages(ctx, database, source, options, v2Facts)
			if err != nil {
				return nil, nil, err
			}
			facts = append(facts, parsed...)
			diagnostics = append(diagnostics, parsedDiagnostics...)
		}
	}
	return facts, diagnostics, nil
}

func (a opencodeSQLiteAdapter) parseV1Messages(ctx context.Context, database *sql.DB, source Source, options SyncOptions, v2Facts map[string]bool) ([]RawTokenFact, []Diagnostic, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT id, session_id, time_created, data
		FROM message
		ORDER BY time_created, id
	`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var facts []RawTokenFact
	var diagnostics []Diagnostic
	for rows.Next() {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		var messageID string
		var sessionID sql.NullString
		var timeCreated sql.NullInt64
		var data string
		if err := rows.Scan(&messageID, &sessionID, &timeCreated, &data); err != nil {
			return nil, nil, err
		}
		fact, rowDiagnostics, ok := a.factFromMessage(source, options, messageID, sessionID, timeCreated, data)
		diagnostics = append(diagnostics, rowDiagnostics...)
		if ok {
			if v2Facts[opencodeLogicalMessageKey(sessionID, messageID)] {
				continue
			}
			facts = append(facts, fact)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return facts, diagnostics, nil
}

func (a opencodeSQLiteAdapter) parseV2Messages(ctx context.Context, database *sql.DB, source Source, options SyncOptions, v2Facts map[string]bool) ([]RawTokenFact, []Diagnostic, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT id, session_id, time_created, data
		FROM session_message
		WHERE type = 'assistant'
		ORDER BY time_created, id
	`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var facts []RawTokenFact
	var diagnostics []Diagnostic
	for rows.Next() {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		var messageID string
		var sessionID sql.NullString
		var timeCreated sql.NullInt64
		var data string
		if err := rows.Scan(&messageID, &sessionID, &timeCreated, &data); err != nil {
			return nil, nil, err
		}
		fact, rowDiagnostics, ok := a.factFromV2Message(source, options, messageID, sessionID, timeCreated, data)
		diagnostics = append(diagnostics, rowDiagnostics...)
		if ok {
			facts = append(facts, fact)
			v2Facts[opencodeLogicalMessageKey(sessionID, messageID)] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return facts, diagnostics, nil
}

func (a opencodeSQLiteAdapter) factFromMessage(source Source, options SyncOptions, rowMessageID string, rowSessionID sql.NullString, rowTimeCreated sql.NullInt64, rawData string) (RawTokenFact, []Diagnostic, bool) {
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

	occurredAt := opencodeOccurredAt(messageData.Time, rowTimeCreated)
	if occurredAt == nil {
		return RawTokenFact{}, []Diagnostic{opencodeDiagnostic("opencode_sqlite_missing_time", "skipped OpenCode assistant message with no usable created timestamp")}, false
	}

	sourceID := source.RawSourceID
	if sourceID == "" {
		sourceID = source.ID
	}
	sessionID := trimSQLString(rowSessionID)
	factMessageID := strings.TrimSpace(messageData.ID)
	if factMessageID == "" {
		factMessageID = strings.TrimSpace(rowMessageID)
	}

	return RawTokenFact{
		Harness:          HarnessOpenCode,
		SourceID:         sourceID,
		SourceKind:       source.Kind,
		Collector:        options.Collector,
		Parser:           options.Parser,
		ObservedAtMs:     syncNowMs(options.Now),
		OccurredAtMs:     occurredAt,
		SessionID:        stringPtrFromTrimmed(sessionID),
		MessageID:        stringPtrFromTrimmed(factMessageID),
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
		DedupeKey:        opencodeDedupeKey(messageData.ProviderID, messageData.ModelID, messageData.Tokens, messageData.Time, occurredAt),
	}, diagnostics, true
}

func (a opencodeSQLiteAdapter) factFromV2Message(source Source, options SyncOptions, rowMessageID string, rowSessionID sql.NullString, rowTimeCreated sql.NullInt64, rawData string) (RawTokenFact, []Diagnostic, bool) {
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
		ObservedAtMs:     syncNowMs(options.Now),
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
		DedupeKey:        opencodeDedupeKey(providerID, modelID, messageData.Tokens, messageData.Time, occurredAt),
	}, diagnostics, true
}

func opencodeLogicalMessageKey(sessionID sql.NullString, messageID string) string {
	return trimSQLString(sessionID) + "\x00" + strings.TrimSpace(messageID)
}

func isOpenCodeSQLiteDB(path string) bool {
	name := filepath.Base(path)
	if name == "opencode.db" {
		return true
	}
	if !strings.HasPrefix(name, "opencode-") || !strings.HasSuffix(name, ".db") {
		return false
	}
	channel := strings.TrimSuffix(strings.TrimPrefix(name, "opencode-"), ".db")
	if channel == "" {
		return false
	}
	for _, r := range channel {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func openReadOnlySQLite(path string) (*sql.DB, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fileURL := url.URL{Scheme: "file", Path: absPath}
	query := fileURL.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(true)")
	query.Add("_pragma", "busy_timeout(5000)")
	fileURL.RawQuery = query.Encode()
	return sql.Open("sqlite", fileURL.String())
}

func sqliteTableExists(ctx context.Context, database *sql.DB, table string) (bool, error) {
	var name string
	err := database.QueryRowContext(ctx, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = ?
	`, table).Scan(&name)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func requireSQLiteColumns(ctx context.Context, database *sql.DB, table string, columns []string) error {
	rows, err := database.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, column := range columns {
		if !found[column] {
			return fmt.Errorf("missing column %s", column)
		}
	}
	return nil
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

func clampInt(value *int64) bool {
	if value == nil || *value >= 0 {
		return false
	}
	*value = 0
	return true
}

func opencodeOccurredAt(times *opencodeMessageTimes, rowTimeCreated sql.NullInt64) *int64 {
	if times != nil && times.Created != nil {
		return times.Created
	}
	if rowTimeCreated.Valid {
		return &rowTimeCreated.Int64
	}
	return nil
}

func trimSQLString(value sql.NullString) string {
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

func opencodeDedupeKey(providerID string, modelID string, tokens *opencodeTokenData, times *opencodeMessageTimes, occurredAt *int64) string {
	completedAt := ""
	if times != nil && times.Completed != nil {
		completedAt = fmt.Sprint(*times.Completed)
	}
	parts := []string{
		"opencode-sqlite-message",
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
	return stableHash(strings.Join(parts, "|"))
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
