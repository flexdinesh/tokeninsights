package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	claudeCodeJSONLSourceKind = "claude-code-session-jsonl"
)

type claudeCodeJSONLAdapter struct{}

func (a claudeCodeJSONLAdapter) Harness() Harness {
	return HarnessClaudeCode
}

func (a claudeCodeJSONLAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	var roots []string
	if options.Sources != nil {
		var err error
		roots, err = options.Sources.discoveryRoots(HarnessClaudeCode, options.HarnessSubdirOnly)
		if err != nil {
			return nil, err
		}
	} else {
		sourceDir := strings.TrimSpace(options.SourceDir)
		if sourceDir != "" {
			harnessDir := filepath.Join(sourceDir, string(HarnessClaudeCode))
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
			root := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
			if root == "" {
				home := strings.TrimSpace(os.Getenv("HOME"))
				if home == "" {
					return nil, nil
				}
				root = filepath.Join(home, ".claude")
			}
			roots = append(roots, filepath.Join(root, "projects"))
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
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir() {
				name := entry.Name()
				if name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if isCandidateSource(path) {
				sources = append(sources, a.source(path, root))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(sources, func(i int, j int) bool {
		return sources[i].Path < sources[j].Path
	})
	return sources, nil
}

func (a claudeCodeJSONLAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness: HarnessClaudeCode,
		ID:      stableHash("claude-code-session-file:" + filepath.ToSlash(rel)),
		Kind:    claudeCodeJSONLSourceKind,
		Path:    path,
	}
}

// Claude records for one native request are snapshots. Source time orders them;
// merging counters independently could synthesize usage never present in source.

// Only unfinalized parser snapshots use this projection. Finalization mutates
// the clone's pointer fields, leaving the original inclusive output untouched.
// Retained raw facts have already split reasoning and must never pass here.
