package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// groupContext is the per-group build context: common plus group inputs.
// Shared contexts are declared once in groups.json and listed in a group's
// uses; the Dockerfile reads every one of them as a named build context.
const groupContext = "group"

var sharedContexts = []string{"deps", "web", "go"}

const platform = "linux/amd64"

type contextSpec struct {
	Inputs, Exclude []string
}

type groupSpec struct {
	Inputs   []string
	Executor string
	Uses     []string
}

type manifest struct {
	Common   []string
	Contexts map[string]contextSpec
	Groups   map[string]groupSpec
}

func (m manifest) validate() error {
	for _, name := range sharedContexts {
		if _, ok := m.Contexts[name]; !ok {
			return fmt.Errorf("groups.json: missing context %q", name)
		}
	}
	for name := range m.Contexts {
		if !slices.Contains(sharedContexts, name) {
			return fmt.Errorf("groups.json: unknown context %q", name)
		}
	}
	for name, group := range m.Groups {
		switch group.Executor {
		case "host":
			if len(group.Uses) != 0 {
				return fmt.Errorf("groups.json: host group %s cannot use build contexts", name)
			}
		case "container":
			if !slices.Contains(group.Uses, "deps") {
				return fmt.Errorf("groups.json: container group %s must use deps", name)
			}
		default:
			return fmt.Errorf("groups.json: group %s has invalid executor %q", name, group.Executor)
		}
		for _, used := range group.Uses {
			if !slices.Contains(sharedContexts, used) {
				return fmt.Errorf("groups.json: group %s uses unknown context %q", name, used)
			}
		}
	}
	return nil
}

// contexts lists the contexts whose files feed a group, in fingerprint order.
func (m manifest) contexts(group string) []string {
	return append([]string{groupContext}, m.Groups[group].Uses...)
}

// includes is the single predicate behind both fingerprints and materialized
// build contexts.
func (m manifest) includes(group, context, file string) bool {
	if context == groupContext {
		return matchAny(append(append([]string{}, m.Common...), m.Groups[group].Inputs...), file)
	}
	if !slices.Contains(m.Groups[group].Uses, context) {
		return false
	}
	spec := m.Contexts[context]
	return matchAny(spec.Inputs, file) && !matchAny(spec.Exclude, file)
}

func matchAny(patterns []string, file string) bool {
	for _, pattern := range patterns {
		if matchPattern(pattern, file) {
			return true
		}
	}
	return false
}

// matchPattern matches slash-separated segments with path.Match. "**" matches
// zero or more segments and a trailing "/" matches everything beneath it.
func matchPattern(pattern, file string) bool {
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

func matchSegments(pattern, file []string) bool {
	if len(pattern) == 0 {
		return len(file) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(file); skip++ {
			if matchSegments(pattern[1:], file[skip:]) {
				return true
			}
		}
		return false
	}
	if len(file) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], file[0])
	return err == nil && ok && matchSegments(pattern[1:], file[1:])
}

// identity hashes every tracked entry of every context the group consumes.
// Git object identifiers commit to complete bytes, modes and symlink targets.
func (r runner) identity(sha, g string) (string, error) {
	b, e := r.command(r.root, "git", "ls-tree", "-rz", "--full-tree", sha)
	if e != nil {
		return "", e
	}
	entries := strings.Split(string(b), "\x00")
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s", receiptVersion, g, platform)
	fmt.Fprintf(h, "\x00storage-samples=%t", r.storageBenchmarks)
	manifestBytes, e := json.Marshal(r.manifest)
	if e != nil {
		return "", e
	}
	h.Write(manifestBytes)
	for _, context := range r.manifest.contexts(g) {
		fmt.Fprintf(h, "\x00context=%s\x00", context)
		for _, entry := range entries {
			parts := strings.SplitN(entry, "\t", 2)
			if len(parts) == 2 && r.manifest.includes(g, context, parts[1]) {
				fmt.Fprint(h, entry, "\x00")
			}
		}
	}
	if r.manifest.Groups[g].Executor == "host" {
		fmt.Fprint(h, runtime.GOOS, "/", runtime.GOARCH)
		fingerprint, _, err := r.hostTools(sha)
		if err != nil {
			return "", fmt.Errorf("install pinned formatter tools explicitly with mise install: %w", err)
		}
		h.Write([]byte(fingerprint))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extract writes the committed tree once; contexts hardlink from it.
func (r runner) extract(sha, dest string) error {
	b, e := r.command(r.root, "git", "archive", "--format=tar", sha)
	if e != nil {
		return e
	}
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		hdr, e := tr.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader || hdr.Typeflag == tar.TypeXHeader || hdr.Typeflag == tar.TypeDir {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(hdr.Name))
		rel, e := filepath.Rel(dest, p)
		if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return errors.New("unsafe archive path")
		}
		// Directory modes are copied into images; the private temp root
		// already excludes other users.
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			return e
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
			data, e := io.ReadAll(tr)
			if e != nil {
				return e
			}
			if e = os.WriteFile(p, data, os.FileMode(hdr.Mode)&0777); e != nil {
				return e
			}
		case tar.TypeSymlink:
			resolved := filepath.Clean(filepath.Join(filepath.Dir(p), hdr.Linkname))
			if filepath.IsAbs(hdr.Linkname) || !strings.HasPrefix(resolved, dest+string(filepath.Separator)) {
				return fmt.Errorf("snapshot symlink escapes source: %s", hdr.Name)
			}
			if e = os.Symlink(hdr.Linkname, p); e != nil {
				return e
			}
		default:
			return fmt.Errorf("unsupported archive entry %s", hdr.Name)
		}
	}
}

// materialize hardlinks each context's selected files under dest. Contexts a
// group does not use stay empty, so an undeclared Dockerfile use fails.
func (m manifest) materialize(tree, group, dest string) (map[string]string, error) {
	dirs := map[string]string{"empty": filepath.Join(dest, "empty")}
	for _, context := range append([]string{groupContext}, sharedContexts...) {
		dirs[context] = filepath.Join(dest, context)
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}
	err := filepath.WalkDir(tree, func(source string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(tree, source)
		if err != nil {
			return err
		}
		file := filepath.ToSlash(rel)
		for _, context := range m.contexts(group) {
			if !m.includes(group, context, file) {
				continue
			}
			target := filepath.Join(dirs[context], rel)
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			// link(2) does not follow symlinks, so links stay links.
			if err := os.Link(source, target); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}
