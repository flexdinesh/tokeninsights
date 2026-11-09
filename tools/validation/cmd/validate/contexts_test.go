package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestMatchPattern(t *testing.T) {
	for _, c := range []struct {
		pattern, file string
		want          bool
	}{
		{"*", "go.mod", true},
		{"*", "packages/go.mod", false},
		{"packages/", "packages/cli/main.go", true},
		{"packages/", "packagesX/main.go", false},
		{"packages/*/package.json", "packages/web/package.json", true},
		{"packages/*/package.json", "packages/web/sub/package.json", false},
		{"install*.sh", "install-main.sh", true},
		{"**/*_test.go", "main_test.go", true},
		{"**/*_test.go", "packages/core/a/b_test.go", true},
		{"**/dist/", "packages/web/dist/index.html", true},
		{"**/dist/", "packages/web/distx/index.html", false},
		{"**/*.test.*", "packages/web/src/app.test.tsx", true},
		{"go.mod", "tools/validation/go.mod", false},
	} {
		if got := matchPattern(c.pattern, c.file); got != c.want {
			t.Errorf("%s ~ %s = %t", c.pattern, c.file, got)
		}
	}
}

func TestManifestValidationRejectsUndeclaredShapes(t *testing.T) {
	valid := func() manifest {
		return manifest{
			Contexts: map[string]contextSpec{"deps": {}, "web": {}, "go": {}},
			Groups:   map[string]groupSpec{"format": {Executor: "host"}, "go": {Executor: "container", Uses: []string{"deps"}}},
		}
	}
	if err := valid().validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*manifest){
		"host uses context":    func(m *manifest) { m.Groups["format"] = groupSpec{Executor: "host", Uses: []string{"deps"}} },
		"container lacks deps": func(m *manifest) { m.Groups["go"] = groupSpec{Executor: "container"} },
		"unknown use":          func(m *manifest) { m.Groups["go"] = groupSpec{Executor: "container", Uses: []string{"deps", "x"}} },
		"unknown executor":     func(m *manifest) { m.Groups["go"] = groupSpec{Executor: "dagger", Uses: []string{"deps"}} },
		"missing context":      func(m *manifest) { delete(m.Contexts, "web") },
		"extra context":        func(m *manifest) { m.Contexts["extra"] = contextSpec{} },
	} {
		m := valid()
		mutate(&m)
		if err := m.validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// gitEntries renders a directory as `git ls-tree -r` lines so on-disk build
// contexts compare byte-for-byte with fingerprint inputs.
func gitEntries(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		var data []byte
		mode := "100644"
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			mode = "120000"
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			data = []byte(target)
		default:
			if info.Mode()&0100 != 0 {
				mode = "100755"
			}
			if data, err = os.ReadFile(path); err != nil {
				return err
			}
		}
		sum := sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...))
		entries = append(entries, fmt.Sprintf("%s blob %x\t%s", mode, sum, filepath.ToSlash(rel)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(entries)
	return entries
}

func selectedEntries(t *testing.T, r runner, sha, group, context string) []string {
	t.Helper()
	out, err := r.command(r.root, "git", "ls-tree", "-rz", "--full-tree", sha)
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	for _, line := range strings.Split(string(out), "\x00") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && r.manifest.includes(group, context, parts[1]) {
			entries = append(entries, line)
		}
	}
	slices.Sort(entries)
	return entries
}

// assertParity proves every group's materialized contexts hold exactly the
// entries its fingerprint hashes, and that undeclared contexts stay empty.
func assertParity(t *testing.T, r runner, sha string) {
	t.Helper()
	tree := filepath.Join(t.TempDir(), "tree")
	if err := r.extract(sha, tree); err != nil {
		t.Fatal(err)
	}
	for group, spec := range r.manifest.Groups {
		if spec.Executor != "container" {
			continue
		}
		dirs, err := r.manifest.materialize(tree, group, filepath.Join(t.TempDir(), group))
		if err != nil {
			t.Fatal(err)
		}
		for _, context := range append([]string{groupContext}, sharedContexts...) {
			got := gitEntries(t, dirs[context])
			want := selectedEntries(t, r, sha, group, context)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s/%s context differs from fingerprint inputs:\n got %d %v\nwant %d %v", group, context, len(got), got, len(want), want)
			}
			if !slices.Contains(r.manifest.contexts(group), context) && len(got) != 0 {
				t.Fatalf("%s received undeclared context %s", group, context)
			}
		}
		if entries := gitEntries(t, dirs["empty"]); len(entries) != 0 {
			t.Fatal("empty context has files", entries)
		}
	}
}

func repositoryManifest(t *testing.T) manifest {
	t.Helper()
	data, err := os.ReadFile("../../groups.json")
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestContextParityOnFixture(t *testing.T) {
	r := fixture(t)
	r.manifest = repositoryManifest(t)
	files := map[string]string{
		"go.mod":                               "module x",
		"package.json":                         "{}",
		"README.md":                            "docs",
		"packages/cli/main.go":                 "package main",
		"packages/cli/main_test.go":            "package main",
		"packages/cli/testdata/fixture.json":   "{}",
		"packages/core/schema.sql":             "create",
		"packages/web/src/app.tsx":             "app",
		"packages/web/src/app.test.tsx":        "test",
		"packages/web/e2e/flow.spec.ts":        "e2e",
		"packages/web/package.json":            "{}",
		"tools/build/src/build-web.ts":         "build",
		"tools/validation/fixtures/personal/a": "fixture",
	}
	for path, data := range files {
		full := filepath.Join(r.root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(r.root, "install.sh"), []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("main.go", filepath.Join(r.root, "packages/cli/link.go")); err != nil {
		t.Fatal(err)
	}
	assertParity(t, r, commit(t, r))
}

// The real tree catches predicates that only diverge on actual repository
// layouts. Inside a check container the baseline commit provides the tree.
func TestContextParityOnRepository(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skip("not inside a git checkout")
	}
	r := runner{ctx: t.Context(), root: strings.TrimSpace(string(root)), home: t.TempDir(), manifest: repositoryManifest(t)}
	sha, err := r.git("rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	assertParity(t, r, sha)
}
