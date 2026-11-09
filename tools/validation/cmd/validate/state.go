package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// prepareHostSource gives host groups an independent Git index over exactly
// their selected files, never real history or host gitdir pointers.
func (r runner) prepareHostSource(tree, group, dest string) (string, error) {
	contexts, err := r.manifest.materialize(tree, group, dest)
	if err != nil {
		return "", err
	}
	source := contexts[groupContext]
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch=validation"}, {"add", "--force", "."}} {
		if _, err = r.command(source, "git", args...); err != nil {
			return "", err
		}
	}
	// Rebuild the index from its tree to remove checkout-dependent stat timestamps.
	written, err := r.command(source, "git", "write-tree")
	if err != nil {
		return "", err
	}
	if err = os.Remove(filepath.Join(source, ".git", "index")); err != nil {
		return "", err
	}
	if _, err = r.command(source, "git", "read-tree", strings.TrimSpace(string(written))); err != nil {
		return "", err
	}
	return source, nil
}

func (r runner) hostFormat(sha, snap, artifacts string) error {
	// Resolve installed tools before entering the isolated home. No install task is invoked.
	r.logPath = ""
	_, path, err := r.hostTools(sha)
	if err != nil {
		return err
	}
	r.hostPath = path
	var e error
	if e = os.MkdirAll(artifacts, 0700); e != nil {
		return e
	}
	r.logPath = filepath.Join(artifacts, "check.log")
	_, e = r.command(snap, "mise", "run", "--no-deps", "--skip-tools", "check:internal:format")
	if e != nil {
		return fmt.Errorf("format check failed; install formatter dependencies explicitly with mise run setup if missing: %w", e)
	}
	return os.WriteFile(filepath.Join(artifacts, "status.json"), []byte("{\"exitCode\":0}"), 0600)
}

// Host formatting consumes installed pinned tools, never host project data or
// dependency installation. The snapshot needs no node_modules mount.
func (r runner) hostTools(sha string) (string, string, error) {
	config, err := r.command(r.root, "git", "show", sha+":mise.toml")
	if err != nil {
		return "", "", err
	}
	manifestBytes, err := r.command(r.root, "git", "show", sha+":package.json")
	if err != nil {
		return "", "", err
	}
	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err = json.Unmarshal(manifestBytes, &pkg); err != nil {
		return "", "", err
	}
	pins := map[string]string{"oxfmt": pkg.DevDependencies["oxfmt"]}
	for _, name := range []string{"go", "node", "min_version"} {
		match := regexp.MustCompile(`(?m)^` + name + ` = "([^"]+)"`).FindSubmatch(config)
		if len(match) != 2 {
			return "", "", fmt.Errorf("missing formatter tool pin %s", name)
		}
		pins[name] = string(match[1])
	}
	pins["mise"] = pins["min_version"]
	var fingerprint strings.Builder
	var paths []string
	for _, name := range []string{"go", "node", "oxfmt", "mise"} {
		binary, err := exec.LookPath(name)
		if err != nil {
			return "", "", fmt.Errorf("missing %s; run mise install and mise run setup explicitly: %w", name, err)
		}
		arg := "--version"
		if name == "go" {
			arg = "version"
		}
		output, err := r.command(r.root, binary, arg)
		if err != nil {
			return "", "", err
		}
		actual := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindString(string(output))
		if pins[name] == "" || actual != pins[name] {
			return "", "", fmt.Errorf("%s requires %s, installed %s", name, pins[name], actual)
		}
		fmt.Fprintf(&fingerprint, "%s=%s\n", name, actual)
		paths = append(paths, filepath.Dir(binary))
		if name == "go" {
			goroot, err := r.command(r.root, binary, "env", "GOROOT")
			if err != nil {
				return "", "", err
			}
			bin := filepath.Join(strings.TrimSpace(string(goroot)), "bin")
			paths = append([]string{bin}, paths...)
			formatter, err := os.ReadFile(filepath.Join(bin, "gofmt"))
			if err != nil {
				return "", "", err
			}
			fmt.Fprintf(&fingerprint, "gofmt=%x\n", sha256.Sum256(formatter))
		}
	}
	paths = append(paths, "/usr/bin", "/bin")
	return fingerprint.String(), strings.Join(paths, string(os.PathListSeparator)), nil
}
func lockFile(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func capacity(name string, fallback int) int {
	n, e := strconv.Atoi(os.Getenv(name))
	if e == nil && n > 0 {
		return n
	}
	return fallback
}

// admissionSlots bounds concurrent container runs machine-wide. Other
// repositories use the same directory and flock files, so the limit is global.
const admissionSlots = 2

func admissionDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "local-validation", "slots"), nil
}

// admit holds one slot for a whole run. Status and cache hits never queue.
func admit(ctx context.Context, dir string) (int, func(), error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		for slot := range admissionSlots {
			f, e := lockFile(filepath.Join(dir, fmt.Sprintf("%d.lock", slot)))
			if e == nil {
				return slot, func() { f.Close() }, nil
			}
			if !errors.Is(e, syscall.EWOULDBLOCK) {
				return 0, nil, e
			}
		}
		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
func pruneRuns(base string) error {
	entries, e := os.ReadDir(filepath.Join(base, "runs"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	type candidate struct {
		path string
		age  time.Time
		size int64
	}
	var runs []candidate
	var total int64
	for _, entry := range entries {
		p := filepath.Join(base, "runs", entry.Name())
		st, e := os.Stat(filepath.Join(p, "completed"))
		if os.IsNotExist(e) {
			// Interrupted processes may never write completed. The lease below
			// still excludes live runs, including ones older than retention.
			st, e = os.Stat(p)
		}
		if e != nil || !entry.IsDir() {
			continue
		}
		var size int64
		_ = filepath.Walk(p, func(_ string, info os.FileInfo, e error) error {
			if e == nil && info.Mode().IsRegular() {
				size += info.Size()
			}
			return nil
		})
		runs = append(runs, candidate{p, st.ModTime(), size})
		total += size
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].age.Before(runs[j].age) })
	days := capacity("TOKENINSIGHTS_VALIDATION_RETENTION_DAYS", 14)
	limit := int64(capacity("TOKENINSIGHTS_VALIDATION_RETENTION_GIB", 10)) * 1024 * 1024 * 1024
	for _, run := range runs {
		if time.Since(run.age) < time.Duration(days)*24*time.Hour && total <= limit {
			continue
		}
		f, e := lockFile(filepath.Join(run.path, "active.lock"))
		if e != nil {
			continue
		}
		e = os.RemoveAll(run.path)
		f.Close()
		if e != nil {
			return e
		}
		total -= run.size
	}
	return nil
}
