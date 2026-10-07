package pipeline

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
)

// Extract preserves whitelisted native records. It never normalizes usage.
func Extract(ctx context.Context, options SyncOptions, store *rawcollectorstore.Store) (Summary, error) {
	var summary Summary
	harnesses := options.Harnesses
	if len(harnesses) == 0 {
		harnesses = SupportedHarnesses
	}
	summary.RequestedHarnesses = len(harnesses)
	for _, h := range harnesses {
		if options.Progress != nil {
			options.Progress(SyncProgressEvent{Harness: h, Status: SyncProgressDiscovering})
		}
		adapter, ok := AdapterFor(h)
		if !ok {
			return summary, errors.New("unsupported_harness")
		}
		sources, err := adapter.Discover(ctx, DiscoverOptions{Sources: options.Sources, SourceDir: options.SourceDir, HarnessSubdirOnly: len(harnesses) > 1})
		if err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, err)
			continue
		}
		if len(sources) == 0 {
			summary.Skipped++
			if options.Progress != nil {
				options.Progress(SyncProgressEvent{Harness: h, Status: SyncProgressSkipped})
			}
			continue
		}
		if options.Progress != nil {
			options.Progress(SyncProgressEvent{Harness: h, Status: SyncProgressSyncing})
		}
		failed := false
		captured := 0
		for _, source := range sources {
			source.ID = evidence.Tuple("source-file", source.Harness, source.ID, source.Path)
			var count int
			if source.Harness == HarnessOpenCode {
				count, err = extractSQLite(ctx, source, options, store)
			} else {
				count, err = extractJSONL(ctx, source, options, store)
			}
			summary.RawFacts += count
			captured += count
			if err != nil {
				failed = true
				summary.Errors = append(summary.Errors, err)
			}
		}
		status := SyncProgressSynced
		if failed {
			summary.Failed++
			status = SyncProgressFailed
		} else if captured == 0 && !options.FullRefresh {
			summary.Skipped++
			status = SyncProgressSkipped
		} else {
			summary.Synced++
		}
		if options.Progress != nil {
			options.Progress(SyncProgressEvent{Harness: h, Status: status})
		}
	}
	return summary, errors.Join(summary.Errors...)
}

type extractionContext struct {
	Records  []evidence.Context `json:"records"`
	Location *evidence.Location `json:"location,omitempty"`
}
type extractionCursor struct {
	SourceID, Lineage, Prefix string
	Offset, Ordinal           int64
	Context                   extractionContext
}

func cursorFor(ctx context.Context, tx *sql.Tx, source Source, format string) (extractionCursor, error) {
	var cursor extractionCursor
	var body string
	err := tx.QueryRowContext(ctx, "SELECT source_id,lineage,byte_offset,ordinal,prefix_hash,context_json FROM evidence_sources WHERE source_key=? AND format=? AND extractor_version=?", source.ID, format, evidence.ExtractorVersion).Scan(&cursor.SourceID, &cursor.Lineage, &cursor.Offset, &cursor.Ordinal, &cursor.Prefix, &body)
	if errors.Is(err, sql.ErrNoRows) {
		cursor.SourceID = source.ID
		cursor.Lineage, err = evidence.RandomID()
		return cursor, err
	}
	if err != nil {
		return cursor, err
	}
	err = json.Unmarshal([]byte(body), &cursor.Context)
	return cursor, err
}
func saveCursor(ctx context.Context, tx *sql.Tx, source Source, format string, cursor extractionCursor) error {
	body, err := json.Marshal(cursor.Context)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO evidence_sources(source_key,source_id,lineage,format,extractor_version,byte_offset,ordinal,prefix_hash,context_json,updated_at_ms) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(source_key) DO UPDATE SET source_id=excluded.source_id,lineage=excluded.lineage,format=excluded.format,extractor_version=excluded.extractor_version,byte_offset=excluded.byte_offset,ordinal=excluded.ordinal,prefix_hash=excluded.prefix_hash,context_json=excluded.context_json,updated_at_ms=excluded.updated_at_ms`, source.ID, cursor.SourceID, cursor.Lineage, format, evidence.ExtractorVersion, cursor.Offset, cursor.Ordinal, cursor.Prefix, string(body), time.Now().UnixMilli())
	return err
}
func prefix(file *os.File, n int64) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.CopyN(hash, file, n); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func extractJSONL(ctx context.Context, source Source, options SyncOptions, store *rawcollectorstore.Store) (int, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	format := string(source.Harness) + "-jsonl"
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	cursor, err := cursorFor(ctx, tx, source, format)
	if err != nil {
		return 0, err
	}
	if cursor.Offset > 0 {
		hash, hashErr := prefix(file, cursor.Offset)
		if hashErr != nil || hash != cursor.Prefix {
			cursor.Offset = 0
			cursor.Ordinal = 0
			cursor.Context = extractionContext{}
			cursor.Lineage, err = evidence.RandomID()
			if err != nil {
				return 0, err
			}
		}
	}
	if options.FullRefresh {
		cursor.Offset = 0
		cursor.Ordinal = 0
		cursor.Context = extractionContext{}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	capturedHash := sha256.New()
	if _, err := io.CopyN(capturedHash, file, cursor.Offset); err != nil {
		return 0, err
	}
	if cursor.Offset > 0 && hex.EncodeToString(capturedHash.Sum(nil)) != cursor.Prefix {
		return 0, errors.New("source_changed_during_capture")
	}
	reader := bufio.NewReader(file)
	count := 0
	verifiedLength := cursor.Offset
	if cursor.Offset == 0 {
		cursor.Prefix = hex.EncodeToString(capturedHash.Sum(nil))
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		// Private source bytes may exceed the wire limit. Bound memory without
		// advancing past an unprocessed record or retaining a partial tail.
		line, readErr := readEvidenceLine(reader)
		tail := errors.Is(readErr, io.EOF)
		if tail && (len(line) == 0 || !json.Valid(line)) {
			break
		}
		if readErr != nil && !tail {
			return 0, readErr
		}
		next := cursor
		next.Context.Records = append([]evidence.Context(nil), cursor.Context.Records...)
		next.Offset += int64(len(line))
		next.Ordinal++
		_, _ = capturedHash.Write(line)
		added, err := extractJSONRecord(ctx, tx, source, options, format, &next, line)
		if err != nil {
			return 0, err
		}
		count += added
		verifiedLength = next.Offset
		// A complete final JSON value is evidence, but its cursor stays before
		// the unterminated line. Append/retry rereads only that last record.
		if tail {
			break
		}
		cursor = next
		cursor.Prefix = hex.EncodeToString(capturedHash.Sum(nil))
	}
	// Re-read only bytes, not records: append-only continuity is checked before
	// committing the raw records and their checkpoint in one SQLite transaction.
	hash, err := prefix(file, verifiedLength)
	if err != nil {
		return 0, err
	}
	if hash != hex.EncodeToString(capturedHash.Sum(nil)) {
		return 0, errors.New("source_changed_during_capture")
	}
	opened, err := file.Stat()
	if err != nil {
		return 0, err
	}
	current, err := os.Stat(source.Path)
	if err != nil {
		return 0, err
	}
	if !os.SameFile(opened, current) {
		return 0, errors.New("source_replaced_during_capture")
	}
	if err := saveCursor(ctx, tx, source, format, cursor); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func extractJSONRecord(ctx context.Context, tx *sql.Tx, source Source, options SyncOptions, format string, cursor *extractionCursor, line []byte) (int, error) {
	data, diagnostics, err := evidence.Sanitize(string(source.Harness), format, line)
	if err != nil {
		return 0, nil
	}
	kind := evidence.String(data, "type")
	contextRecord := source.Harness == HarnessPi && kind == "session" || source.Harness == HarnessCodex && (kind == "session_meta" || kind == "turn_context" || kind == "event_msg" && evidence.String(data, "payload.type") == "task_started")
	if contextRecord {
		filtered := cursor.Context.Records[:0]
		for _, entry := range cursor.Context.Records {
			if evidence.String(entry.Data, "type") != kind {
				filtered = append(filtered, entry)
			}
		}
		cursor.Context.Records = append(filtered, evidence.Context{Ordinal: cursor.Ordinal, Data: data, Diagnostics: diagnostics})
	}
	if !contextRecord && !evidence.UsageRecord(string(source.Harness), format, data) {
		return 0, nil
	}
	if object, err := evidence.DecodeData(line); err == nil {
		directory := stringValue(object, "", "cwd")
		remote := ""
		if source.Harness == HarnessCodex {
			payload := nested(object, "payload")
			directory = stringValue(payload, "", "cwd")
			remote = stringValue(nested(payload, "git"), "", "repository_url", "origin_url")
		}
		if directory != "" {
			cursor.Context.Location = ExtractLocation(ctx, options, directory, remote, "")
		}
	}
	record := evidence.Record{Harness: string(source.Harness), Format: format, SourceID: cursor.SourceID, Lineage: cursor.Lineage, Ordinal: cursor.Ordinal, Data: data, Location: cursor.Context.Location, Diagnostics: diagnostics}
	for _, entry := range cursor.Context.Records {
		if entry.Ordinal != cursor.Ordinal {
			record.Context = append(record.Context, entry)
		}
	}
	added, err := rawcollectorstore.Record(ctx, tx, record, time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	if added {
		return 1, nil
	}
	return 0, nil
}

const maxEvidenceLineBytes = 16 << 20

func readEvidenceLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxEvidenceLineBytes {
			return nil, errors.New("source_record_limit")
		}
		line = append(line, fragment...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}

func extractSQLite(ctx context.Context, source Source, options SyncOptions, store *rawcollectorstore.Store) (int, error) {
	database, err := openReadOnlySQLite(source.Path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = database.Close() }()
	snapshot, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer func() { _ = snapshot.Rollback() }()
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	cursor, err := cursorFor(ctx, tx, source, "opencode-sqlite")
	if err != nil {
		return 0, err
	}
	// Existing messages can be revised without an update cursor. Snapshot scans
	// are required; immutable observation keys avoid re-enqueueing unchanged rows.
	locations, count, err := extractOpenCodeContext(ctx, snapshot, tx, source, options, cursor)
	if err != nil {
		return 0, err
	}
	for _, version := range []struct {
		table, format string
		v2            bool
	}{{"message", "opencode-v1", false}, {"session_message", "opencode-v2", true}} {
		exists, err := sqliteTableExists(ctx, snapshot, version.table)
		if err != nil {
			return 0, err
		}
		if !exists {
			continue
		}
		query := "SELECT id,session_id,time_created,data FROM message"
		if version.v2 {
			query = "SELECT id,session_id,time_created,data,type FROM session_message"
		}
		rows, err := snapshot.QueryContext(ctx, query)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id string
			var session sql.NullString
			var created sql.NullInt64
			var payload string
			var kind sql.NullString
			var scanErr error
			if version.v2 {
				scanErr = rows.Scan(&id, &session, &created, &payload, &kind)
			} else {
				scanErr = rows.Scan(&id, &session, &created, &payload)
			}
			if scanErr != nil {
				_ = rows.Close()
				return 0, scanErr
			}
			var data json.RawMessage
			if json.Valid([]byte(payload)) {
				data = json.RawMessage(payload)
			} else {
				data = json.RawMessage("null")
			}
			object := map[string]interface{}{"id": id, "session_id": nil, "time_created": nil, "data": data}
			if session.Valid {
				object["session_id"] = session.String
			}
			if created.Valid {
				object["time_created"] = created.Int64
			}
			if version.v2 {
				object["type"] = nil
				if kind.Valid {
					object["type"] = kind.String
				}
			}
			body, err := json.Marshal(object)
			if err != nil {
				_ = rows.Close()
				return 0, err
			}
			safe, diagnostics, err := evidence.Sanitize("opencode", version.format, body)
			if err != nil {
				_ = rows.Close()
				return 0, err
			}
			record := evidence.Record{Harness: "opencode", Format: version.format, SourceID: cursor.SourceID, Lineage: cursor.Lineage, Data: safe, Diagnostics: diagnostics, Location: locations[session.String]}
			if !evidence.UsageRecord(record.Harness, record.Format, record.Data) {
				continue
			}
			inserted, err := rawcollectorstore.Record(ctx, tx, record, time.Now().UnixMilli())
			if err != nil {
				_ = rows.Close()
				return 0, err
			}
			if inserted {
				count++
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return 0, err
		}

	}
	if err := saveCursor(ctx, tx, source, "opencode-sqlite", cursor); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// Capture approved native context separately and precompute only session
// enrichment. Message extraction streams rows without retaining a full transcript.
func extractOpenCodeContext(ctx context.Context, snapshot, tx *sql.Tx, source Source, options SyncOptions, cursor extractionCursor) (map[string]*evidence.Location, int, error) {
	locations := map[string]*evidence.Location{}
	count := 0
	sessions := []RawTokenFact{}
	for _, kind := range []struct{ table, format, column string }{{"session", "opencode-session", "project_id"}, {"project", "opencode-project", "vcs"}} {
		if err := requireSQLiteColumns(ctx, snapshot, kind.table, []string{"id", kind.column}); err != nil {
			continue
		}
		rows, err := snapshot.QueryContext(ctx, "SELECT id,"+kind.column+" FROM "+kind.table)
		if err != nil {
			return nil, 0, err
		}
		for rows.Next() {
			var id string
			var value sql.NullString
			if err := rows.Scan(&id, &value); err != nil {
				_ = rows.Close()
				return nil, 0, err
			}
			if kind.table == "session" {
				native := id
				sessions = append(sessions, RawTokenFact{SessionID: &native})
			}
			if kind.table == "project" && value.String != "git" {
				continue
			}
			object := map[string]interface{}{"id": id, kind.column: nil}
			if value.Valid {
				object[kind.column] = value.String
			}
			body, err := json.Marshal(object)
			if err != nil {
				_ = rows.Close()
				return nil, 0, err
			}
			data, diagnostics, err := evidence.Sanitize("opencode", kind.format, body)
			if err != nil {
				_ = rows.Close()
				return nil, 0, err
			}
			inserted, err := rawcollectorstore.Record(ctx, tx, evidence.Record{Harness: "opencode", Format: kind.format, SourceID: cursor.SourceID, Lineage: cursor.Lineage, Data: data, Diagnostics: diagnostics}, time.Now().UnixMilli())
			if err != nil {
				_ = rows.Close()
				return nil, 0, err
			}
			if inserted {
				count++
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, 0, err
		}
	}
	if err := attachOpenCodeLocations(ctx, snapshot, options, sessions); err != nil {
		return nil, 0, err
	}
	for _, session := range sessions {
		if session.SessionID == nil || session.Location == nil {
			continue
		}
		location := session.Location
		safe := &evidence.Location{DirectoryKey: location.DirectoryKey, DirectoryName: locationLabel(filepath.Base(location.DirectoryName)), RepositoryKey: location.RepositoryKey, RepositoryName: location.RepositoryName, RepositorySource: location.RepositorySource}
		if !evidence.SafeMetadata(safe.DirectoryName) {
			safe.DirectoryName = "unknown"
		}
		if !evidence.SafeMetadata(safe.RepositoryName) {
			safe.RepositoryName = "unknown"
		}
		locations[*session.SessionID] = safe
	}
	return locations, count, nil
}
