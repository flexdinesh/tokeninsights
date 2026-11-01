package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type groupSpec struct {
	Inputs          []string
	Executor, Class string
}
type manifest struct {
	Common []string
	Groups map[string]groupSpec
}

func (r runner) selected(group, path string) bool {
	for _, p := range append(append([]string{}, r.manifest.Common...), r.manifest.Groups[group].Inputs...) {
		if p == path || strings.HasSuffix(p, "/") && strings.HasPrefix(path, p) {
			return true
		}
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
	}
	return false
}
func (r runner) identity(sha, g string) (string, error) {
	b, e := r.command(r.root, "git", "ls-tree", "-rz", "--full-tree", sha)
	if e != nil {
		return "", e
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00linux/amd64", receiptVersion, version, engineImage, g)
	fmt.Fprintf(h, "\x00storage-samples=%t", r.storageBenchmarks)
	manifestBytes, e := json.Marshal(r.manifest)
	if e != nil {
		return "", e
	}
	h.Write(manifestBytes)
	for _, entry := range strings.Split(string(b), "\x00") {
		parts := strings.SplitN(entry, "\t", 2)
		if len(parts) != 2 || !r.selected(g, parts[1]) {
			continue
		}
		// Git object identifiers commit to the complete bytes, including symlink targets.
		fmt.Fprint(h, entry, "\x00")
	}
	if r.manifest.Groups[g].Executor == "host" {
		fmt.Fprint(h, runtime.GOOS, "/", runtime.GOARCH)
		fingerprint, _, err := r.hostTools(sha)
		e = err
		if e != nil {
			return "", fmt.Errorf("install pinned formatter tools explicitly with mise install: %w", e)
		}
		h.Write([]byte(fingerprint))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
func slot(ctx context.Context, class string) (func(), error) {
	count := 1
	if class == "light" {
		count = 2
	} else if class != "heavy" {
		return nil, fmt.Errorf("invalid execution class %q", class)
	}
	count = capacity("TOKENINSIGHTS_VALIDATION_"+strings.ToUpper(class)+"_SLOTS", count)
	// Admission must not depend on an invocation's private TMPDIR.
	dir := filepath.Join("/tmp", fmt.Sprintf("local-validation-%d", os.Getuid()))
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || stat.Uid != uint32(os.Getuid()) {
		return nil, fmt.Errorf("unsafe admission directory %s", dir)
	}
	for {
		for i := 0; i < count; i++ {
			f, e := lockFile(filepath.Join(dir, fmt.Sprintf("%s-%d.lock", class, i)))
			if e == nil {
				return func() { f.Close() }, nil
			}
			if !errors.Is(e, syscall.EWOULDBLOCK) {
				return nil, e
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
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
