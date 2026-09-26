package pipeline

import (
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

func sourceMarkerValid(source Source, options SyncOptions, metadata sourceRefreshMetadata, state sourceState) bool {
	if options.FullRefresh || source.AlwaysRefresh || !state.hasRefresh || !state.hasCursor {
		return false
	}
	cursor, refresh := state.cursor, state.refresh
	if cursor.collector != options.Collector || cursor.parser != options.Parser ||
		cursor.kind != fingerprintKind(source) || refresh.collector != cursor.collector || refresh.parser != cursor.parser {
		return false
	}
	return source.Harness == HarnessOpenCode || (cursor.offset == cursor.sizeBytes && cursor.sizeBytes == metadata.sizeBytes &&
		cursor.mtimeMs == metadata.mtimeMs && refresh.sourceMtimeMs == cursor.mtimeMs && refresh.sourceSizeBytes == cursor.sizeBytes)
}

func preparedFingerprintCursor(source Source, options SyncOptions, metadata sourceRefreshMetadata, fingerprint sourceFingerprint) *sourceCursorState {
	return &sourceCursorState{collector: options.Collector, parser: options.Parser, kind: fingerprintKind(source),
		offset: metadata.sizeBytes, mtimeMs: metadata.mtimeMs, sizeBytes: metadata.sizeBytes,
		prefixHash: fingerprint.content, locationFingerprint: fingerprint.location}
}

func prepareJSONLSource(ctx context.Context, adapter Adapter, source Source, options SyncOptions, state sourceState) preparedSource {
	metadata, ok := sourceRefreshMetadataFor(source)
	result := preparedSource{source: source, metadata: metadata, hasMetadata: ok, status: "ingested"}
	result.sourceInfo, _ = os.Stat(source.Path)
	if ok && sourceMarkerValid(source, options, metadata, state) {
		fingerprint, valid := fingerprintSource(ctx, source, options)
		if valid && fingerprint.content == state.cursor.prefixHash && fingerprint.location == state.cursor.locationFingerprint {
			result.fingerprint = fingerprint
			result.cursor = preparedFingerprintCursor(source, options, metadata, fingerprint)
			result.unchanged, result.status = true, "unchanged"
			return result
		}
	}
	snapshot := newSourceSnapshot(source)
	options.sourceSnapshot = snapshot
	result.facts, result.diagnostics, result.parseErr = adapter.Parse(ctx, source, options)
	if result.parseErr != nil {
		return result
	}
	if snapshot.deferred {
		result.status = "deferred"
		return result
	}
	if !ok || source.AlwaysRefresh {
		return result
	}
	current, valid := sourceRefreshMetadataFor(source)
	if !valid || current != metadata {
		result.status = "deferred"
		return result
	}
	fingerprint, err := snapshot.fingerprint(ctx, options)
	if err != nil {
		return result
	}
	if !snapshot.verified || snapshot.metadata != metadata {
		verified, err := sourceContentHash(ctx, source)
		if err != nil || verified != fingerprint.content {
			result.status = "deferred"
			return result
		}
	}
	result.fingerprint = fingerprint
	result.cursor = preparedFingerprintCursor(source, options, metadata, fingerprint)
	return result
}

func prepareOpenCodeSource(ctx context.Context, source Source, options SyncOptions, state sourceState) preparedSource {
	metadata, ok := sourceRefreshMetadataFor(source)
	result := preparedSource{source: source, metadata: metadata, hasMetadata: ok, status: "ingested"}
	result.sourceInfo, _ = os.Stat(source.Path)
	database, err := openReadOnlySQLite(source.Path)
	if err != nil {
		result.parseErr = err
		return result
	}
	defer func() { _ = database.Close() }()
	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		result.parseErr = err
		return result
	}
	defer func() { _ = tx.Rollback() }()
	fingerprint, fingerprintErr := openCodeSnapshotFingerprint(ctx, tx, options)
	if fingerprintErr == nil && ok && sourceMarkerValid(source, options, metadata, state) &&
		fingerprint.content == state.cursor.prefixHash && fingerprint.location == state.cursor.locationFingerprint {
		result.fingerprint = fingerprint
		result.cursor = preparedFingerprintCursor(source, options, metadata, fingerprint)
		result.unchanged, result.status = true, "unchanged"
		return result
	}
	result.facts, result.diagnostics, result.parseErr = (opencodeSQLiteAdapter{}).parseSnapshot(ctx, tx, source, options)
	if result.parseErr == nil && fingerprintErr == nil && ok {
		result.fingerprint = fingerprint
		result.cursor = preparedFingerprintCursor(source, options, metadata, fingerprint)
	}
	return result
}

func fingerprintSource(ctx context.Context, source Source, options SyncOptions) (sourceFingerprint, bool) {
	if source.Harness == HarnessOpenCode {
		fingerprint, err := openCodeLogicalFingerprint(ctx, source, options)
		return fingerprint, err == nil
	}
	if source.Harness != HarnessCodex && source.Harness != HarnessClaudeCode {
		return sourceFingerprint{}, false
	}
	current, err := scanJSONLFingerprint(ctx, source, options)
	if err != nil {
		return sourceFingerprint{}, false
	}
	verifiedContent, err := sourceContentHash(ctx, source)
	return current, err == nil && current.content == verifiedContent
}

func scanJSONLFingerprint(ctx context.Context, source Source, options SyncOptions) (sourceFingerprint, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return sourceFingerprint{}, err
	}
	defer func() { _ = file.Close() }()
	snapshot := newSourceSnapshot(source)
	snapshot.scanLocations = true
	options.sourceSnapshot = snapshot
	scanner := newSourceJSONLReader(ctx, file, source, options)
	for scanner.Scan() {
	}
	if err := scanner.Err(); err != nil {
		return sourceFingerprint{}, err
	}
	return snapshot.fingerprint(ctx, options)
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
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return "", statErr
		}
		size, copyErr := io.Copy(fileHash, contextReader{ctx: ctx, reader: io.NewSectionReader(file, 0, info.Size())})
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
	return openCodeSnapshotFingerprint(ctx, tx, options)
}

func openCodeSnapshotFingerprint(ctx context.Context, tx openCodeReader, options SyncOptions) (sourceFingerprint, error) {
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
