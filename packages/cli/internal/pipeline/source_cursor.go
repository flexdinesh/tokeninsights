package pipeline

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

const sourceCursorHashWindow = 4096
const piByteCursorKind = "pi-jsonl-byte-v1"

type sourceCursorState struct {
	collector, parser, kind    string
	offset, mtimeMs, sizeBytes int64
	prefixHash, boundaryHash   string
	locationFingerprint        string
}

func preparePiSource(ctx context.Context, source Source, options SyncOptions, state sourceState) preparedSource {
	metadata, ok := sourceRefreshMetadataFor(source)
	result := preparedSource{source: source, metadata: metadata, hasMetadata: ok, status: "ingested"}
	result.sourceInfo, _ = os.Stat(source.Path)
	snapshot := newSourceSnapshot(source)
	var offset int64
	cursor, refresh := state.cursor, state.refresh
	valid := ok && !options.FullRefresh && !source.AlwaysRefresh && state.hasCursor && state.hasRefresh &&
		cursor.collector == options.Collector && cursor.parser == options.Parser && cursor.kind == piByteCursorKind &&
		cursor.offset > 0 && cursor.offset == cursor.sizeBytes && metadata.sizeBytes >= cursor.offset &&
		refresh.collector == cursor.collector && refresh.parser == cursor.parser &&
		refresh.sourceMtimeMs == cursor.mtimeMs && refresh.sourceSizeBytes == cursor.sizeBytes
	locationFingerprint := ""
	if valid {
		file, err := os.Open(source.Path)
		if err == nil {
			_, err = io.CopyN(snapshot, contextReader{ctx: ctx, reader: file}, cursor.offset)
			_ = file.Close()
		}
		boundary := sha256.Sum256(snapshot.boundary)
		valid = err == nil && len(snapshot.boundary) > 0 && snapshot.boundary[len(snapshot.boundary)-1] == '\n' &&
			fmt.Sprintf("%x", snapshot.hasher.Sum(nil)) == cursor.prefixHash && fmt.Sprintf("%x", boundary) == cursor.boundaryHash
		if valid {
			file, err := os.Open(source.Path)
			if err == nil {
				snapshot.piHeader, snapshot.piHeaderValid, err = piCursorHeader(ctx, file, piSessionIDFromFilename(source.Path))
				_ = file.Close()
			}
			if err == nil && snapshot.piHeaderValid {
				location, _ := resolveFactLocation(ctx, options, snapshot.piHeader.cwd, "", "")
				if location != nil {
					locationFingerprint = locationSemanticKey(*location)
				}
			}
			valid = err == nil && snapshot.piHeaderValid && locationFingerprint == cursor.locationFingerprint
		}
		if valid && metadata.sizeBytes == cursor.offset && metadata.mtimeMs == cursor.mtimeMs {
			result.unchanged, result.status, result.cursor = true, "unchanged", &cursor
			return result
		}
		if valid && metadata.sizeBytes > cursor.offset {
			offset = cursor.offset
		}
	}
	if offset == 0 {
		snapshot = newSourceSnapshot(source)
	}
	options.sourceSnapshot = snapshot
	var eligible bool
	result.facts, result.diagnostics, eligible, result.parseErr = (piJSONLAdapter{}).ParseFrom(ctx, source, options, offset)
	if result.parseErr != nil {
		return result
	}
	if snapshot.deferred {
		result.status = "deferred"
		return result
	}
	current, currentOK := sourceRefreshMetadataFor(source)
	if ok && (!currentOK || current != metadata || snapshot.size != metadata.sizeBytes) {
		result.status = "deferred"
		return result
	}
	if !ok || !eligible || !snapshot.complete || len(snapshot.boundary) == 0 || snapshot.boundary[len(snapshot.boundary)-1] != '\n' {
		return result
	}
	if offset == 0 {
		if !snapshot.piHeaderValid {
			return result
		}
		location, _ := resolveFactLocation(ctx, options, snapshot.piHeader.cwd, "", "")
		if location != nil {
			locationFingerprint = locationSemanticKey(*location)
		}
	}
	verified, err := sourceContentHash(ctx, source)
	if err != nil || verified != snapshot.contentFingerprint() {
		result.status = "deferred"
		return result
	}
	boundary := sha256.Sum256(snapshot.boundary)
	result.cursor = &sourceCursorState{collector: options.Collector, parser: options.Parser, kind: piByteCursorKind,
		offset: snapshot.size, mtimeMs: metadata.mtimeMs, sizeBytes: metadata.sizeBytes,
		prefixHash: fmt.Sprintf("%x", snapshot.hasher.Sum(nil)), boundaryHash: fmt.Sprintf("%x", boundary), locationFingerprint: locationFingerprint}
	return result
}

func sourceCursorHashes(path string, offset int64) (string, string, bool, error) {
	if offset <= 0 {
		return "", "", false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = file.Close() }()
	length := min(int64(sourceCursorHashWindow), offset)
	boundary := make([]byte, length)
	prefixHasher := sha256.New()
	if _, err := io.CopyN(prefixHasher, file, offset); err != nil {
		return "", "", false, fmt.Errorf("read source cursor prefix: %w", err)
	}
	if _, err := file.ReadAt(boundary, offset-length); err != nil {
		return "", "", false, fmt.Errorf("read source cursor boundary: %w", err)
	}
	boundarySum := sha256.Sum256(boundary)
	return fmt.Sprintf("%x", prefixHasher.Sum(nil)), fmt.Sprintf("%x", boundarySum), boundary[len(boundary)-1] == '\n', nil
}
