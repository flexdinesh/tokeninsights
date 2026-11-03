package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	codexSessionJSONLSourceKind = "codex-session-jsonl"
)

type codexJSONLAdapter struct{}

func (a *codexJSONLAdapter) Harness() Harness {
	return HarnessCodex
}

func (a *codexJSONLAdapter) Discover(ctx context.Context, options DiscoverOptions) ([]Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var roots []string
	if options.Sources != nil {
		var err error
		roots, err = options.Sources.discoveryRoots(HarnessCodex, options.HarnessSubdirOnly)
		if err != nil {
			return nil, err
		}
	} else {
		sourceDir := strings.TrimSpace(options.SourceDir)
		if sourceDir != "" {
			harnessDir := filepath.Join(sourceDir, string(HarnessCodex))
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
			root := strings.TrimSpace(os.Getenv("CODEX_HOME"))
			if root == "" {
				home := strings.TrimSpace(os.Getenv("HOME"))
				if home == "" {
					return nil, nil
				}
				root = filepath.Join(home, ".codex")
			}
			roots = append(roots,
				filepath.Join(root, "sessions"),
				filepath.Join(root, "archived_sessions"),
			)
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
			if isCodexSessionSource(root) {
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
			if isCodexSessionSource(path) {
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
	return sources, ctx.Err()
}

func (a *codexJSONLAdapter) source(path string, root string) Source {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return Source{
		Harness: HarnessCodex,
		ID:      stableHash("codex-session-file:" + filepath.ToSlash(rel)),
		Kind:    codexSessionJSONLSourceKind,
		Path:    path,
	}
}

func isCodexSessionSource(path string) bool {
	if !isCandidateSource(path) {
		return false
	}
	return strings.HasPrefix(filepath.Base(path), "rollout-")
}
