package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitSourcesPreserveLexicalOpenCodeIdentityAndFingerprint(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("HOME", "home")
	t.Setenv("XDG_DATA_HOME", "data")
	t.Setenv("CODEX_HOME", "codex")
	t.Setenv("CLAUDE_CONFIG_DIR", "claude")
	if err := os.MkdirAll("data/opencode", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/opencode/opencode.db", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, override := range []string{"", "data/opencode", "data/opencode/."} {
		c, err := ResolveSources(override)
		if err != nil {
			t.Fatal(err)
		}
		legacy, err := (opencodeSQLiteAdapter{}).Discover(context.Background(), DiscoverOptions{SourceDir: override})
		if err != nil {
			t.Fatal(err)
		}
		explicit, err := (opencodeSQLiteAdapter{}).Discover(context.Background(), DiscoverOptions{Sources: c})
		if err != nil {
			t.Fatal(err)
		}
		if len(legacy) != 1 || len(explicit) != 1 || legacy[0].ID != explicit[0].ID || legacy[0].RawSourceID != explicit[0].RawSourceID {
			t.Fatal("source identity changed", legacy, explicit)
		}
		parts := []string{"rebuild-sources-v1"}
		if override != "" {
			abs, err := filepath.Abs(override)
			if err != nil {
				t.Fatal(err)
			}
			parts = append(parts, "override", abs)
		} else {
			parts = append(parts, "defaults")
			for _, path := range []string{"data/opencode", "home/.pi/agent/sessions", "codex/sessions", "codex/archived_sessions", "claude/projects"} {
				abs, err := filepath.Abs(path)
				if err != nil {
					t.Fatal(err)
				}
				parts = append(parts, abs)
			}
		}
		encoded, err := json.Marshal(parts)
		if err != nil {
			t.Fatal(err)
		}
		got, err := recoverySourceKey(SyncOptions{Sources: c})
		if err != nil || got != stableHash(string(encoded)) {
			t.Fatal("recovery fingerprint changed", err)
		}
		// A different daemon environment does not redirect captured sources.
		t.Setenv("XDG_DATA_HOME", "other-data")
		again, err := (opencodeSQLiteAdapter{}).Discover(context.Background(), DiscoverOptions{Sources: c})
		if err != nil || len(again) != 1 || again[0].ID != legacy[0].ID {
			t.Fatal("captured source environment changed", err)
		}
		t.Setenv("XDG_DATA_HOME", "data")
	}
}

func TestSourceConfigMissingRootsAndOverrideSubdirectoryPolicy(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	c, err := ResolveSources("")
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range c.Roots {
		if root.Path != "" || root.Identity != "" {
			t.Fatal("invented source root")
		}
	}
	root := t.TempDir()
	c, err = ResolveSources(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range SupportedHarnesses {
		all, err := c.discoveryRoots(h, true)
		if err != nil || len(all) != 0 {
			t.Fatal("multi-harness fallback escaped subdirectory")
		}
		single, err := c.discoveryRoots(h, false)
		if err != nil || len(single) != 1 || single[0] != root {
			t.Fatal("single-harness fallback changed")
		}
	}
	before, err := recoverySourceKey(SyncOptions{Sources: c})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	after, err := recoverySourceKey(SyncOptions{Sources: c})
	if err != nil || before != after {
		t.Fatal("directory availability changed recovery scope")
	}
}
