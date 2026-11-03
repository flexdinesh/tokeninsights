package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

const (
	piSessionJSONLSourceKind = "pi-session-jsonl"
)

type piJSONLAdapter struct{}

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
