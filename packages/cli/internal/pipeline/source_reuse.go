package pipeline

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

const sourceFingerprintKind = "source-fingerprint-v1"
const openCodeLogicalFingerprintKind = "opencode-logical-v1"

func fingerprintKind(source Source) string {
	if source.Harness == HarnessOpenCode {
		return openCodeLogicalFingerprintKind
	}
	return sourceFingerprintKind
}

type sourceFingerprint struct {
	content  string
	location string
}

func planSourceReuse(ctx context.Context, runner sqlRunner, source Source, options SyncOptions, metadata sourceRefreshMetadata, ok bool) (sourceFingerprint, bool, error) {
	if !ok || source.AlwaysRefresh || source.Harness == HarnessPi {
		return sourceFingerprint{}, false, nil
	}
	current, valid := fingerprintSource(ctx, source, options)
	if !valid {
		return sourceFingerprint{}, false, nil
	}
	if options.FullRefresh {
		return current, false, nil
	}
	var state sourceCursorState
	err := runner.QueryRowContext(ctx, `
		SELECT collector, parser, cursor_kind, byte_offset, source_mtime_ms, source_size_bytes, prefix_hash, boundary_hash, location_fingerprint
		FROM source_cursor_state WHERE harness = ? AND source_kind = ? AND source_state_key = ?
	`, source.Harness, source.Kind, metadata.stateKey).Scan(&state.collector, &state.parser, &state.kind, &state.offset, &state.mtimeMs, &state.sizeBytes, &state.prefixHash, &state.boundaryHash, &state.locationFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return current, false, nil
	}
	if err != nil {
		return sourceFingerprint{}, false, err
	}
	refresh, found, err := loadSourceRefreshState(ctx, runner, source, metadata.stateKey)
	if err != nil {
		return sourceFingerprint{}, false, err
	}
	valid = found && state.collector == options.Collector && state.parser == options.Parser &&
		state.kind == fingerprintKind(source) && refresh.collector == state.collector && refresh.parser == state.parser
	if source.Harness != HarnessOpenCode {
		valid = valid && state.offset == state.sizeBytes && state.sizeBytes == metadata.sizeBytes &&
			state.mtimeMs == metadata.mtimeMs && refresh.sourceMtimeMs == state.mtimeMs && refresh.sourceSizeBytes == state.sizeBytes
	}
	return current, valid && state.prefixHash == current.content && state.locationFingerprint == current.location, nil
}

func storeSourceReuse(ctx context.Context, runner sqlRunner, source Source, options SyncOptions, metadata sourceRefreshMetadata, ok bool, before sourceFingerprint) error {
	if !ok || source.Harness == HarnessPi || source.AlwaysRefresh {
		return nil
	}
	if before.content == "" {
		_, err := runner.ExecContext(ctx, "DELETE FROM source_cursor_state WHERE harness = ? AND source_kind = ? AND source_state_key = ?", source.Harness, source.Kind, metadata.stateKey)
		return err
	}
	currentMetadata, valid := sourceRefreshMetadataFor(source)
	if !valid || (source.Harness != HarnessOpenCode && (currentMetadata.mtimeMs != metadata.mtimeMs || currentMetadata.sizeBytes != metadata.sizeBytes)) {
		return nil
	}
	after, valid := fingerprintSource(ctx, source, options)
	if !valid || after != before {
		return nil
	}
	_, err := runner.ExecContext(ctx, `
		INSERT INTO source_cursor_state (
			harness, source_kind, source_state_key, collector, parser, cursor_kind,
			byte_offset, source_mtime_ms, source_size_bytes, prefix_hash, boundary_hash, location_fingerprint, updated_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (harness, source_kind, source_state_key) DO UPDATE SET
			collector = excluded.collector, parser = excluded.parser, cursor_kind = excluded.cursor_kind,
			byte_offset = excluded.byte_offset, source_mtime_ms = excluded.source_mtime_ms,
			source_size_bytes = excluded.source_size_bytes, prefix_hash = excluded.prefix_hash,
			boundary_hash = excluded.boundary_hash, location_fingerprint = excluded.location_fingerprint,
			updated_at_ms = excluded.updated_at_ms
	`, source.Harness, source.Kind, metadata.stateKey, options.Collector, options.Parser, fingerprintKind(source),
		metadata.sizeBytes, metadata.mtimeMs, metadata.sizeBytes, before.content, "", before.location, syncNowMs(options.Now))
	return err
}

func fingerprintSource(ctx context.Context, source Source, options SyncOptions) (sourceFingerprint, bool) {
	if source.Harness == HarnessOpenCode {
		fingerprint, err := openCodeLogicalFingerprint(ctx, source, options)
		return fingerprint, err == nil
	}
	content, err := sourceContentHash(ctx, source)
	if err != nil {
		return sourceFingerprint{}, false
	}
	var location string
	switch source.Harness {
	case HarnessCodex, HarnessClaudeCode:
		location, err = jsonlLocationFingerprint(ctx, source, options)
	default:
		return sourceFingerprint{}, false
	}
	if err != nil {
		return sourceFingerprint{}, false
	}
	verifiedContent, err := sourceContentHash(ctx, source)
	return sourceFingerprint{content: content, location: location}, err == nil && content == verifiedContent
}

func sourceContentHash(ctx context.Context, source Source) (string, error) {
	hash := sha256.New()
	paths := []string{source.Path}
	for _, path := range paths {
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) && path != source.Path {
			_, _ = hash.Write([]byte("missing;"))
			continue
		}
		if err != nil {
			return "", err
		}
		fileHash := sha256.New()
		size, copyErr := io.Copy(fileHash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		_, _ = fmt.Fprintf(hash, "%d:%x;", size, fileHash.Sum(nil))
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func jsonlLocationFingerprint(ctx context.Context, source Source, options SyncOptions) (string, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	cwds := map[string]bool{"": true}
	remotes := map[string]bool{"": true}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxCodexJSONLLineBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		line := scanner.Text()
		if !strings.Contains(line, `"cwd"`) && !strings.Contains(line, `"repository_url"`) && !strings.Contains(line, `\u`) {
			continue
		}
		var record map[string]interface{}
		if decodeJSONRecord(line, &record) != nil {
			continue
		}
		if source.Harness == HarnessCodex {
			switch stringValue(record, "", "type") {
			case "session_meta", "turn_context":
				payload := nested(record, "payload")
				if cwd := stringField(payload, "cwd"); cwd != nil {
					cwds[*cwd] = true
				}
				if stringValue(record, "", "type") == "session_meta" {
					if remote := stringField(nested(payload, "git"), "repository_url"); remote != nil {
						remotes[*remote] = true
					}
				}
			}
		} else if stringValue(record, "", "type") == "assistant" {
			if cwd := stringField(record, "cwd"); cwd != nil {
				cwds[*cwd] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	var signatures []string
	for cwd := range cwds {
		for remote := range remotes {
			location, conflict := resolveFactLocation(ctx, options, cwd, remote, "")
			signatures = append(signatures, locationFingerprintPart(location, conflict))
		}
	}
	sort.Strings(signatures)
	return stableHash(strings.Join(signatures, "\x00")), nil
}

func openCodeLogicalFingerprint(ctx context.Context, source Source, options SyncOptions) (sourceFingerprint, error) {
	database, err := openReadOnlySQLite(source.Path)
	if err != nil {
		return sourceFingerprint{}, err
	}
	defer func() { _ = database.Close() }()
	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return sourceFingerprint{}, err
	}
	defer func() { _ = tx.Rollback() }()
	hasher := sha256.New()
	rows, err := tx.QueryContext(ctx, `SELECT name, sql FROM sqlite_schema
		WHERE type = 'table' AND name IN ('message', 'session_message', 'session', 'project') ORDER BY name`)
	if err != nil {
		return sourceFingerprint{}, err
	}
	tables := map[string]bool{}
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			_ = rows.Close()
			return sourceFingerprint{}, err
		}
		tables[name] = true
		writeFingerprintField(hasher, name)
		writeFingerprintField(hasher, definition)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return sourceFingerprint{}, err
	}
	if !tables["message"] && !tables["session_message"] {
		return sourceFingerprint{}, errors.New("OpenCode has no message table")
	}
	for _, table := range []string{"message", "session_message"} {
		if !tables[table] {
			continue
		}
		columns := []string{"id", "session_id", "time_created", "data"}
		if table == "session_message" {
			columns = append(columns, "type")
		}
		if err := requireSQLiteColumns(ctx, tx, table, columns); err != nil {
			return sourceFingerprint{}, err
		}
		query := "SELECT id, session_id, time_created, data FROM " + table + " ORDER BY id"
		if table == "session_message" {
			query = "SELECT id, session_id, time_created, data, type FROM session_message ORDER BY id"
		}
		messageRows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return sourceFingerprint{}, err
		}
		for messageRows.Next() {
			var id, data string
			var sessionID, rowType sql.NullString
			var created sql.NullInt64
			if table == "session_message" {
				err = messageRows.Scan(&id, &sessionID, &created, &data, &rowType)
			} else {
				err = messageRows.Scan(&id, &sessionID, &created, &data)
			}
			if err != nil {
				_ = messageRows.Close()
				return sourceFingerprint{}, err
			}
			writeFingerprintField(hasher, table)
			writeFingerprintField(hasher, id)
			writeNullableFingerprintField(hasher, sessionID)
			if created.Valid {
				writeFingerprintField(hasher, strconv.FormatInt(created.Int64, 10))
			} else {
				writeFingerprintField(hasher, "null")
			}
			writeFingerprintField(hasher, data)
			if table == "session_message" {
				writeNullableFingerprintField(hasher, rowType)
			}
		}
		err = messageRows.Err()
		_ = messageRows.Close()
		if err != nil {
			return sourceFingerprint{}, err
		}
	}
	location, err := openCodeLocationFingerprint(ctx, tx, options)
	if err != nil {
		return sourceFingerprint{}, err
	}
	return sourceFingerprint{content: fmt.Sprintf("%x", hasher.Sum(nil)), location: location}, nil
}

func writeFingerprintField(hasher hash.Hash, value string) {
	_, _ = fmt.Fprintf(hasher, "%d:", len(value))
	_, _ = io.WriteString(hasher, value)
}

func writeNullableFingerprintField(hasher hash.Hash, value sql.NullString) {
	if !value.Valid {
		writeFingerprintField(hasher, "null")
		return
	}
	writeFingerprintField(hasher, "value")
	writeFingerprintField(hasher, value.String)
}

func openCodeLocationFingerprint(ctx context.Context, database openCodeReader, options SyncOptions) (string, error) {
	if requireSQLiteColumns(ctx, database, "session", []string{"id", "directory", "project_id"}) != nil {
		return stableHash(""), nil
	}
	gitProjects := map[string]bool{}
	if requireSQLiteColumns(ctx, database, "project", []string{"id", "vcs"}) == nil {
		rows, err := database.QueryContext(ctx, "SELECT id FROM project WHERE vcs = 'git'")
		if err != nil {
			return "", err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return "", err
			}
			gitProjects[id] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return "", err
		}
		_ = rows.Close()
	}
	rows, err := database.QueryContext(ctx, "SELECT id, directory, project_id FROM session")
	if err != nil {
		return "", err
	}
	var signatures []string
	for rows.Next() {
		var sessionID string
		var directory, projectID sql.NullString
		if err := rows.Scan(&sessionID, &directory, &projectID); err != nil {
			_ = rows.Close()
			return "", err
		}
		project := ""
		if gitProjects[projectID.String] {
			project = projectID.String
		}
		location, conflict := resolveFactLocation(ctx, options, directory.String, "", project)
		signatures = append(signatures, stableHash(sessionID+"\x00"+locationFingerprintPart(location, conflict)))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	_ = rows.Close()
	sort.Strings(signatures)
	return stableHash(strings.Join(signatures, "\x00")), nil
}

func locationFingerprintPart(location *Location, conflict bool) string {
	if location == nil {
		return fmt.Sprintf("none:%t", conflict)
	}
	return stableHash(strings.Join([]string{
		location.DirectoryKey, location.DirectoryName, location.RepositoryKey,
		location.RepositoryName, location.RepositorySource, fmt.Sprint(conflict),
	}, "\x00"))
}
