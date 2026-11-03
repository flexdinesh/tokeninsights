package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SourceConfig captures a caller's source environment without changing process
// environment or working directory. Roots retain lexical identity separately
// from absolute paths used for I/O.
type SourceConfig struct {
	BaseDir  string        `json:"baseDir"`
	Home     string        `json:"home"`
	Override SourceRoot    `json:"override"`
	Roots    [5]SourceRoot `json:"roots"`
}

type SourceRoot struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
}

func ResolveSources(sourceDir string) (*SourceConfig, error) {
	base, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	c := &SourceConfig{BaseDir: base, Home: home}
	root := func(value string) SourceRoot {
		if value == "" {
			return SourceRoot{}
		}
		absolute := value
		if !filepath.IsAbs(value) {
			absolute = filepath.Join(base, value)
		}
		return SourceRoot{Path: filepath.Clean(absolute), Identity: value}
	}
	c.Override = root(strings.TrimSpace(sourceDir))
	data := configuredRoot("XDG_DATA_HOME", home, ".local", "share")
	codex := configuredRoot("CODEX_HOME", home, ".codex")
	claude := configuredRoot("CLAUDE_CONFIG_DIR", home, ".claude")
	c.Roots = [5]SourceRoot{
		root(rootChild(data, "opencode")), root(rootChild(home, ".pi", "agent", "sessions")),
		root(rootChild(codex, "sessions")), root(rootChild(codex, "archived_sessions")), root(rootChild(claude, "projects")),
	}
	return c, nil
}

func (c *SourceConfig) Validate() error {
	if c == nil || !filepath.IsAbs(c.BaseDir) {
		return fmt.Errorf("invalid source base directory")
	}
	for _, r := range append(c.Roots[:], c.Override) {
		if r.Path == "" && r.Identity == "" {
			continue
		}
		path := r.Identity
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.BaseDir, path)
		}
		if !filepath.IsAbs(r.Path) || filepath.Clean(path) != r.Path {
			return fmt.Errorf("invalid source root")
		}
	}
	return nil
}

func (c *SourceConfig) discoveryRoots(h Harness, subdirOnly bool) ([]string, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Override.Path != "" {
		child := filepath.Join(c.Override.Path, string(h))
		if info, err := os.Stat(child); err == nil && info.IsDir() {
			return []string{child}, nil
		} else if subdirOnly {
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			return nil, nil
		}
		return []string{c.Override.Path}, nil
	}
	indices := map[Harness][]int{HarnessOpenCode: {0}, HarnessPi: {1}, HarnessCodex: {2, 3}, HarnessClaudeCode: {4}}
	var roots []string
	for _, index := range indices[h] {
		if c.Roots[index].Path != "" {
			roots = append(roots, c.Roots[index].Path)
		}
	}
	return roots, nil
}

func (c *SourceConfig) identityPath(path string) string {
	roots := c.Roots[:]
	if c.Override.Path != "" {
		roots = []SourceRoot{c.Override}
	}
	for _, root := range roots {
		if root.Path == "" {
			continue
		}
		rel, err := filepath.Rel(root.Path, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return root.Identity
			}
			return filepath.Join(root.Identity, rel)
		}
	}
	return path
}
