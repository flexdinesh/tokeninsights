package pipeline

import (
	"context"
	"os"
	"strings"
)

const codexAncestryCursorKind = "codex-ancestry-v1"

type codexVerifiedSource struct {
	source      Source
	metadata    sourceRefreshMetadata
	fingerprint sourceFingerprint
	valid       bool
	sourceInfo  os.FileInfo
}

type codexVerificationCall struct {
	done   chan struct{}
	result codexVerifiedSource
}

// Ancestors are computed inline. A worker never waits for a parent queued in
// the same bounded pool, and the acyclic chain prevents recursive cache waits.
func (a *codexJSONLAdapter) verifySource(ctx context.Context, source Source, options SyncOptions) codexVerifiedSource {
	a.mu.Lock()
	if call, found := a.verified[source.Path]; found {
		a.mu.Unlock()
		select {
		case <-call.done:
			result := call.result
			current, found := sourceRefreshMetadataFor(source)
			info, err := os.Stat(source.Path)
			result.valid = result.valid && found && current == result.metadata && err == nil && result.sourceInfo != nil && os.SameFile(result.sourceInfo, info)
			return result
		case <-ctx.Done():
			return codexVerifiedSource{}
		}
	}
	if a.verified == nil {
		a.verified = make(map[string]*codexVerificationCall)
	}
	call := &codexVerificationCall{done: make(chan struct{})}
	a.verified[source.Path] = call
	a.mu.Unlock()
	info, infoErr := os.Stat(source.Path)
	metadata, hasMetadata := sourceRefreshMetadataFor(source)
	fingerprint, valid := fingerprintSource(ctx, source, options)
	after, hasAfter := sourceRefreshMetadataFor(source)
	afterInfo, afterInfoErr := os.Stat(source.Path)
	header, err := codexReadSourceMetadata(ctx, source)
	expected, found := a.sourceMetadata(source)
	result := codexVerifiedSource{source: source, metadata: metadata, fingerprint: fingerprint, sourceInfo: info,
		valid: infoErr == nil && afterInfoErr == nil && os.SameFile(info, afterInfo) && hasMetadata && hasAfter && metadata == after && valid && err == nil && found && header == expected}
	a.mu.Lock()
	call.result = result
	if ctx.Err() != nil {
		delete(a.verified, source.Path)
	}
	close(call.done)
	a.mu.Unlock()
	return result
}

func (a *codexJSONLAdapter) ancestrySources(source Source) []Source {
	if a.ancestryProblem(source) != "" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var sources []Source
	for {
		sources = append(sources, source)
		metadata := a.metadata[source.Path]
		if !metadata.fork {
			return sources
		}
		source = a.sessions[metadata.parentID][0]
	}
}

func (a *codexJSONLAdapter) ancestryFingerprint(dependencies []codexVerifiedSource, options SyncOptions) string {
	parts := []string{codexAncestryCursorKind, options.Collector, options.Parser}
	for _, dependency := range dependencies {
		if !dependency.valid {
			return ""
		}
		metadata, found := a.sourceMetadata(dependency.source)
		if !found {
			return ""
		}
		parts = append(parts, dependency.source.ID, dependency.source.Kind, metadata.sessionID, metadata.parentID,
			dependency.fingerprint.content, dependency.fingerprint.location)
	}
	return stableHash(strings.Join(parts, "\x00"))
}

func prepareCodexSource(ctx context.Context, adapter *codexJSONLAdapter, source Source, options SyncOptions, state sourceState) preparedSource {
	metadata, found := adapter.sourceMetadata(source)
	if !found || !metadata.fork {
		current, hasCurrent := sourceRefreshMetadataFor(source)
		if hasCurrent && sourceMarkerValid(source, options, current, state) {
			verified := adapter.verifySource(ctx, source, options)
			if verified.valid && verified.fingerprint.content == state.cursor.prefixHash && verified.fingerprint.location == state.cursor.locationFingerprint {
				return preparedSource{source: source, metadata: current, hasMetadata: true, sourceInfo: verified.sourceInfo,
					fingerprint: verified.fingerprint, cursor: preparedFingerprintCursor(source, options, current, verified.fingerprint),
					unchanged: true, status: "unchanged"}
			}
			// The shared verification already disproved reuse; parse directly.
			state = sourceState{}
		}
		return prepareJSONLSource(ctx, adapter, source, options, state)
	}
	chain := adapter.ancestrySources(source)
	current, hasCurrent := sourceRefreshMetadataFor(source)
	validMarker := !options.FullRefresh && hasCurrent && state.hasCursor && state.hasRefresh &&
		state.cursor.kind == codexAncestryCursorKind && state.cursor.collector == options.Collector && state.cursor.parser == options.Parser &&
		state.refresh.collector == options.Collector && state.refresh.parser == options.Parser &&
		state.cursor.offset == current.sizeBytes && state.cursor.sizeBytes == current.sizeBytes && state.cursor.mtimeMs == current.mtimeMs &&
		state.refresh.sourceSizeBytes == current.sizeBytes && state.refresh.sourceMtimeMs == current.mtimeMs
	if validMarker && len(chain) != 0 {
		dependencies := make([]codexVerifiedSource, 0, len(chain))
		for _, dependency := range chain {
			dependencies = append(dependencies, adapter.verifySource(ctx, dependency, options))
		}
		own := dependencies[0]
		ancestry := adapter.ancestryFingerprint(dependencies, options)
		if ancestry != "" && own.fingerprint.content == state.cursor.prefixHash && own.fingerprint.location == state.cursor.locationFingerprint && ancestry == state.cursor.boundaryHash {
			prepared := preparedSource{source: source, metadata: current, hasMetadata: true, sourceInfo: own.sourceInfo, fingerprint: own.fingerprint, unchanged: true, status: "unchanged", startedAtMs: syncNowMs(options.Now)}
			for _, dependency := range dependencies {
				prepared.dependencies = append(prepared.dependencies, sourceDependency{source: dependency.source, metadata: dependency.metadata, sourceInfo: dependency.sourceInfo})
			}
			return prepared
		}
	}
	prepared := prepareJSONLSource(ctx, adapter, source, options, state)
	if prepared.parseErr != nil || prepared.status == "deferred" || len(chain) == 0 {
		return prepared
	}
	result := adapter.parseResolved(ctx, source, options)
	if result.err != nil || result.unresolved || len(result.dependencies) != len(chain) {
		return prepared
	}
	ancestry := adapter.ancestryFingerprint(result.dependencies, options)
	if ancestry == "" {
		return prepared
	}
	own := result.dependencies[0]
	if !prepared.hasMetadata || own.metadata != prepared.metadata {
		return prepared
	}
	prepared.fingerprint = own.fingerprint
	prepared.cursor = &sourceCursorState{collector: options.Collector, parser: options.Parser, kind: codexAncestryCursorKind,
		offset: own.metadata.sizeBytes, mtimeMs: own.metadata.mtimeMs, sizeBytes: own.metadata.sizeBytes,
		prefixHash: own.fingerprint.content, boundaryHash: ancestry, locationFingerprint: own.fingerprint.location}
	for _, dependency := range result.dependencies {
		prepared.dependencies = append(prepared.dependencies, sourceDependency{source: dependency.source, metadata: dependency.metadata, sourceInfo: dependency.sourceInfo})
	}
	return prepared
}
