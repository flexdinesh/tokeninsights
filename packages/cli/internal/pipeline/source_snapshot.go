package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"sort"
	"strings"
)

// sourceSnapshot hashes the exact bytes consumed by a parser, including tails.
// It retains only hashes and location metadata, never transcript content.
type sourceSnapshot struct {
	source        Source
	hasher        hash.Hash
	size          int64
	boundary      []byte
	cwds          map[string]bool
	remotes       map[string]bool
	deferred      bool
	complete      bool
	verified      bool
	metadata      sourceRefreshMetadata
	piHeader      piJSONLSessionFile
	piHeaderValid bool
	scanLocations bool
}

func newSourceSnapshot(source Source) *sourceSnapshot {
	return &sourceSnapshot{source: source, hasher: sha256.New(), cwds: map[string]bool{"": true}, remotes: map[string]bool{"": true}}
}

func (s *sourceSnapshot) write(chunk []byte) {
	_, _ = s.hasher.Write(chunk)
	s.size += int64(len(chunk))
	if len(chunk) >= sourceCursorHashWindow {
		s.boundary = append(s.boundary[:0], chunk[len(chunk)-sourceCursorHashWindow:]...)
		return
	}
	s.boundary = append(s.boundary, chunk...)
	if len(s.boundary) > sourceCursorHashWindow {
		s.boundary = append(s.boundary[:0], s.boundary[len(s.boundary)-sourceCursorHashWindow:]...)
	}
}

func (s *sourceSnapshot) Write(chunk []byte) (int, error) {
	s.write(chunk)
	return len(chunk), nil
}

func (s *sourceSnapshot) observeLine(line []byte) {
	if !bytes.Contains(line, []byte(`"cwd"`)) && !bytes.Contains(line, []byte(`"repository_url"`)) && !bytes.Contains(line, []byte(`\u`)) {
		return
	}
	text := string(line)
	var record map[string]interface{}
	if decodeJSONRecord(text, &record) != nil {
		return
	}
	s.observeRecord(record)
}

func (s *sourceSnapshot) observeRecord(record map[string]interface{}) {
	switch s.source.Harness {
	case HarnessCodex:
		kind := stringValue(record, "", "type")
		if kind != "session_meta" && kind != "turn_context" {
			return
		}
		payload := nested(record, "payload")
		if cwd := stringField(payload, "cwd"); cwd != nil {
			s.cwds[*cwd] = true
		}
		if kind == "session_meta" {
			if remote := stringField(nested(payload, "git"), "repository_url"); remote != nil {
				s.remotes[*remote] = true
			}
		}
	case HarnessClaudeCode:
		if stringValue(record, "", "type") == "assistant" {
			if cwd := stringField(record, "cwd"); cwd != nil {
				s.cwds[*cwd] = true
			}
		}
	}
}

func (s *sourceSnapshot) contentFingerprint() string {
	outer := sha256.New()
	_, _ = fmt.Fprintf(outer, "%d:%x;", s.size, s.hasher.Sum(nil))
	return fmt.Sprintf("%x", outer.Sum(nil))
}

func (s *sourceSnapshot) fingerprint(ctx context.Context, options SyncOptions) (sourceFingerprint, error) {
	if !s.complete || s.deferred {
		return sourceFingerprint{}, errors.New("incomplete source snapshot")
	}
	var signatures []string
	for cwd := range s.cwds {
		for remote := range s.remotes {
			location, conflict := resolveFactLocation(ctx, options, cwd, remote, "")
			signatures = append(signatures, locationFingerprintPart(location, conflict))
		}
	}
	sort.Strings(signatures)
	return sourceFingerprint{content: s.contentFingerprint(), location: stableHash(strings.Join(signatures, "\x00"))}, nil
}
