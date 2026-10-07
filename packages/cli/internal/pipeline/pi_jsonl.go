package pipeline

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	piSessionJSONLSourceKind = "pi-session-jsonl"
)

type piJSONLAdapter struct{}

type piJSONLSessionFile struct {
	sessionID         string
	filenameSessionID string
	hasHeader         bool
	cwd               string
}

func (a piJSONLAdapter) Harness() Harness {
	return HarnessPi
}

func (a piJSONLAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	var roots []string
	if options.Sources != nil {
		var err error
		roots, err = options.Sources.discoveryRoots(HarnessPi, options.HarnessSubdirOnly)
		if err != nil {
			return nil, err
		}
	} else {
		sourceDir := strings.TrimSpace(options.SourceDir)
		if sourceDir != "" {
			harnessDir := filepath.Join(sourceDir, string(HarnessPi))
			if info, err := os.Stat(harnessDir); err == nil && info.IsDir() {
				roots = append(roots, harnessDir)
			} else if options.HarnessSubdirOnly {
				if err != nil && !os.IsNotExist(err) {
					return nil, err
				}
				return nil, nil
			} else {
				roots = append(roots, sourceDir)
			}
		} else {
			home := strings.TrimSpace(os.Getenv("HOME"))
			if home == "" {
				return nil, nil
			}
			roots = append(roots, filepath.Join(home, ".pi", "agent", "sessions"))
		}

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
			if isCandidateSource(root) {
				sources = append(sources, a.source(root, filepath.Dir(root)))
			}
			continue
		}
		rootSources, err := a.sourcesInRoot(ctx, root)
		if err != nil {
			return nil, err
		}
		sources = append(sources, rootSources...)
	}
	return sources, nil
}

func (a piJSONLAdapter) sourcesInRoot(ctx context.Context, root string) ([]Source, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var sources []Source
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		path := filepath.Join(root, entry.Name())
		if !entry.IsDir() {
			if isCandidateSource(path) {
				sources = append(sources, a.source(path, root))
			}
			continue
		}
		childEntries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, childEntry := range childEntries {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			childPath := filepath.Join(path, childEntry.Name())
			if !childEntry.IsDir() && isCandidateSource(childPath) {
				sources = append(sources, a.source(childPath, root))
			}
		}
	}
	return sources, nil
}

func (a piJSONLAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness: HarnessPi,
		ID:      stableHash("pi-source:" + filepath.ToSlash(rel)),
		Kind:    piSessionJSONLSourceKind,
		Path:    path,
	}
}

func (a piJSONLAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	facts, diagnostics, _, err := a.ParseFrom(ctx, source, options, 0)
	return facts, diagnostics, err
}

func (a piJSONLAdapter) ParseFrom(ctx context.Context, source Source, options SyncOptions, offset int64) ([]RawTokenFact, []Diagnostic, bool, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = file.Close() }()
	recordSourceParse(ctx)

	session := piJSONLSessionFile{filenameSessionID: piSessionIDFromFilename(source.Path)}
	session.sessionID = session.filenameSessionID
	eligible := false
	if offset > 0 {
		var header piJSONLSessionFile
		var ok bool
		if options.sourceSnapshot != nil && options.sourceSnapshot.piHeaderValid {
			header, ok = options.sourceSnapshot.piHeader, true
		} else {
			header, ok, err = piCursorHeader(ctx, file, session.filenameSessionID)
		}
		if err != nil {
			return nil, nil, false, err
		}
		if ok {
			session = header
			eligible = true
		} else {
			offset = 0
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return nil, nil, false, err
		}
	}
	var facts []RawTokenFact
	var diagnostics []Diagnostic
	scanner := newSourceJSONLReader(ctx, file, source, options)
	firstRecord := offset == 0
	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]interface{}
		if err := decodeJSONRecord(line, &record); err != nil {
			diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_parse_error", "skipped unparsable Pi JSONL line"))
			firstRecord = false
			continue
		}
		if stringValue(record, "", "type") == "session" {
			id := stringField(record, "id")
			eligible = firstRecord && id != nil && strings.TrimSpace(*id) != ""
			session.cwd = stringValue(record, session.cwd, "cwd")
			if sessionID := stringField(record, "id"); sessionID != nil {
				session.hasHeader = true
				session.sessionID = *sessionID
				if session.filenameSessionID != "" && session.filenameSessionID != session.sessionID {
					diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_session_id_mismatch", "Pi session header id differs from filename session id"))
				}
			}
			firstRecord = false
			continue
		}
		firstRecord = false
		fact, rowDiagnostics, ok := a.factFromRecord(ctx, source, options, session, record)
		diagnostics = append(diagnostics, rowDiagnostics...)
		if ok {
			facts = append(facts, fact)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, false, err
	}
	if scanner.deferred {
		diagnostics = append(diagnostics, Diagnostic{Harness: HarnessPi, Severity: "info", Code: "jsonl_incomplete_tail", Message: "unfinished final JSONL record deferred until next sync"})
	}
	if !session.hasHeader && session.filenameSessionID != "" && len(facts) > 0 {
		diagnostics = append(diagnostics, piDiagnostic("pi_jsonl_missing_session_header", "used Pi filename session id because the session header was missing"))
	}
	if options.sourceSnapshot != nil {
		options.sourceSnapshot.piHeader, options.sourceSnapshot.piHeaderValid = session, eligible
	}
	return facts, diagnostics, eligible, nil
}

func piCursorHeader(ctx context.Context, file *os.File, filenameSessionID string) (piJSONLSessionFile, bool, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return piJSONLSessionFile{}, false, err
	}
	scanner := newJSONLReader(ctx, file)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return piJSONLSessionFile{}, false, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]interface{}
		if decodeJSONRecord(line, &record) != nil || stringValue(record, "", "type") != "session" {
			return piJSONLSessionFile{}, false, nil
		}
		id := stringField(record, "id")
		if id == nil || strings.TrimSpace(*id) == "" {
			return piJSONLSessionFile{}, false, nil
		}
		return piJSONLSessionFile{sessionID: *id, filenameSessionID: filenameSessionID, hasHeader: true, cwd: stringValue(record, "", "cwd")}, true, nil
	}
	return piJSONLSessionFile{}, false, scanner.Err()
}

func (a piJSONLAdapter) factFromRecord(ctx context.Context, source Source, options SyncOptions, session piJSONLSessionFile, record map[string]interface{}) (RawTokenFact, []Diagnostic, bool) {
	native, diagnostics, ok := processor.PiMessage(ctx, nativeSource(source), nativeOptions(options), session.sessionID, session.hasHeader, record)
	fact := processorFact(native)
	if ok {
		fact.Location, _ = resolveFactLocation(ctx, options, session.cwd, "", "")
	}
	return fact, processorDiagnostics(diagnostics), ok
}

func piSessionIDFromFilename(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	separator := strings.Index(name, "_")
	if separator < 0 || separator == len(name)-1 {
		return ""
	}
	return strings.TrimSpace(name[separator+1:])
}

func piDiagnostic(code string, message string) Diagnostic {
	return Diagnostic{
		Harness:  HarnessPi,
		Severity: "warning",
		Code:     code,
		Message:  message,
	}
}
