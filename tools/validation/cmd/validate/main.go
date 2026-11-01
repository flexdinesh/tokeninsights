package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const version = "0.21.11"
const engine = "tokeninsights-validation-engine-v0.21.11"
const engineImage = "registry.dagger.io/engine@sha256:6b557cc8b5fdfee0e2e8c82d451ac922097a1255482143e1b96c0be3ff2a303c"
const engineVolume = "tokeninsights-validation-cache-v0.21.11"
const receiptVersion = "v2"

type selection []string

func (s *selection) String() string     { return strings.Join(*s, ",") }
func (s *selection) Set(v string) error { *s = append(*s, v); return nil }

type receipt struct {
	Version, Group, Key, SHA, Tree, Artifacts string
	Mode                                      string
	Completed                                 time.Time
}
type runner struct {
	ctx               context.Context
	root, home        string
	logPath           string
	manifest          manifest
	hostPath          string
	storageBenchmarks bool
}

func (r runner) command(dir, name string, args ...string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Join(r.home, "tmp"), 0700); err != nil {
		return nil, err
	}
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + r.home, "XDG_CONFIG_HOME=" + r.home + "/config", "XDG_CACHE_HOME=" + r.home + "/cache", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "DAGGER_LEAVE_OLD_ENGINE=true", "_EXPERIMENTAL_DAGGER_RUNNER_HOST=docker-container://" + engine, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0"}
	c.Env = append(c.Env, "TMPDIR="+filepath.Join(r.home, "tmp"))
	if r.hostPath != "" {
		c.Env[0] = "PATH=" + r.hostPath
		c.Env = append(c.Env, "MISE_TRUSTED_CONFIG_PATHS="+dir, "MISE_TASK_RUN_AUTO_INSTALL=false", "GOTOOLCHAIN=local")
	}
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.WaitDelay = 5 * time.Second
	var output bytes.Buffer
	c.Stdout, c.Stderr = &output, &output
	if r.logPath != "" {
		log, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return nil, err
		}
		defer log.Close()
		stream := io.MultiWriter(&output, log, os.Stdout)
		c.Stdout, c.Stderr = stream, stream
	}
	if e := r.ctx.Err(); e != nil {
		return nil, e
	}
	if e := c.Start(); e != nil {
		return nil, e
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-done:
			return
		case <-r.ctx.Done():
			_ = syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
			// Descendants can outlive their parent; cancel the owned group too.
			_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		case <-timer.C:
			_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		}
	}()
	e := c.Wait()
	close(done)
	<-stopped
	b := output.Bytes()
	if r.ctx.Err() != nil {
		return b, r.ctx.Err()
	}
	if e != nil {
		return b, fmt.Errorf("%s %v: %w\n%s", name, args, e, b)
	}
	return b, nil
}
func (r runner) git(args ...string) (string, error) {
	b, e := r.command(r.root, "git", args...)
	return strings.TrimSpace(string(b)), e
}
func (r runner) clean() (string, error) {
	s, e := r.git("status", "--porcelain", "--untracked-files=no")
	if e != nil {
		return "", e
	}
	if s != "" {
		return "", errors.New("tracked worktree/index dirty")
	}
	return r.git("rev-parse", "HEAD")
}
func (r runner) snapshot(sha, group, dest string) (string, error) {
	b, e := r.command(r.root, "git", "archive", "--format=tar", sha)
	if e != nil {
		return "", e
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00linux/amd64\x00%s\x00", receiptVersion, version, engineImage, group)
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		hdr, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader || hdr.Typeflag == tar.TypeXHeader {
			continue
		}
		if !r.selected(group, hdr.Name) {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(hdr.Name))
		rel, e := filepath.Rel(dest, p)
		if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return "", errors.New("unsafe archive path")
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00%s\x00%d\x00", hdr.Name, hdr.Mode, hdr.Typeflag, hdr.Linkname, hdr.Size)
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return "", e
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
			data, e := io.ReadAll(tr)
			if e != nil {
				return "", e
			}
			h.Write(data)
			if e = os.WriteFile(p, data, os.FileMode(hdr.Mode)); e != nil {
				return "", e
			}
		case tar.TypeSymlink:
			resolved := filepath.Clean(filepath.Join(filepath.Dir(p), hdr.Linkname))
			if filepath.IsAbs(hdr.Linkname) || !strings.HasPrefix(resolved, dest+string(filepath.Separator)) {
				return "", fmt.Errorf("snapshot symlink escapes source: %s", hdr.Name)
			}
			if e = os.Symlink(hdr.Linkname, p); e != nil {
				return "", e
			}
		default:
			return "", fmt.Errorf("unsupported archive entry %s", hdr.Name)
		}
	}
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch=validation"}, {"add", "--force", "."}} {
		if _, e = r.command(dest, "git", args...); e != nil {
			return "", e
		}
	}
	// Rebuild the index from its tree to remove checkout-dependent stat timestamps.
	tree, e := r.command(dest, "git", "write-tree")
	if e != nil {
		return "", e
	}
	if e = os.Remove(filepath.Join(dest, ".git", "index")); e != nil {
		return "", e
	}
	if _, e = r.command(dest, "git", "read-tree", strings.TrimSpace(string(tree))); e != nil {
		return "", e
	}
	// Enumeration needs only an independent index, never real history or host gitdir pointers.
	return hex.EncodeToString(h.Sum(nil)), nil
}
func evidence(path string) error {
	b, e := os.ReadFile(filepath.Join(path, "status.json"))
	if e != nil {
		return e
	}
	var s struct {
		ExitCode *int `json:"exitCode"`
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return e
	}
	if s.ExitCode == nil || *s.ExitCode != 0 {
		return errors.New("check failed or incomplete status")
	}
	f, e := os.Stat(filepath.Join(path, "check.log"))
	if e != nil {
		return e
	}
	if !f.Mode().IsRegular() {
		return errors.New("missing check log")
	}
	return nil
}
func readReceipt(path, key, group string) (receipt, bool) {
	var r receipt
	b, e := os.ReadFile(path)
	if e != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	return r, r.Version == receiptVersion && r.Key == key && r.Group == group && r.SHA != "" && !r.Completed.IsZero() && evidence(r.Artifacts) == nil
}
func saveReceipt(path string, r receipt) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), "receipt-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func (r runner) checkEngine(cli string, setup bool) error {
	b, e := r.command(r.root, cli, "version")
	if e != nil {
		return e
	}
	if !strings.Contains(string(b), "v"+version+" ") && !strings.HasSuffix(strings.TrimSpace(string(b)), "v"+version) {
		return fmt.Errorf("requires Dagger %s: %s", version, b)
	}
	if e = r.checkVolume(setup); e != nil {
		return e
	}
	b, e = r.command(r.root, "docker", "container", "inspect", engine)
	if e != nil && setup { // Only create after proving the named container is absent.
		names, err := r.command(r.root, "docker", "container", "ls", "--all", "--filter", "name=^/"+engine+"$", "--format", "{{.Names}}")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(names)) != "" {
			return e
		}
		_, e = r.command(r.root, "docker", "run", "--detach", "--privileged", "--name", engine, "--label", "dev.tokeninsights.owner=validation", "--volume", engineVolume+":/var/lib/dagger", engineImage)
		if e != nil {
			return e
		}
		b, e = r.command(r.root, "docker", "container", "inspect", engine)
	}
	if e != nil {
		return e
	}
	var v []struct {
		Config struct {
			Image  string
			Labels map[string]string
		}
		State  struct{ Running bool }
		Mounts []struct{ Type, Name, Destination string }
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return e
	}
	if len(v) != 1 || v[0].Config.Image != engineImage || v[0].Config.Labels["dev.tokeninsights.owner"] != "validation" || !v[0].State.Running {
		return errors.New("managed engine ownership, digest or running state mismatch; explicit operator repair required")
	}
	for _, mount := range v[0].Mounts {
		if mount.Destination == "/var/lib/dagger" && mount.Type == "volume" && mount.Name == engineVolume {
			return nil
		}
	}
	return errors.New("managed engine cache mount mismatch")
}

func (r runner) checkVolume(setup bool) error {
	b, err := r.command(r.root, "docker", "volume", "inspect", engineVolume)
	if err != nil && setup {
		names, listErr := r.command(r.root, "docker", "volume", "ls", "--format", "{{.Name}}")
		if listErr != nil {
			return listErr
		}
		for _, name := range strings.Fields(string(names)) {
			if name == engineVolume {
				return err
			}
		}
		if _, err = r.command(r.root, "docker", "volume", "create", "--label", "dev.tokeninsights.owner=validation", engineVolume); err != nil {
			return err
		}
		b, err = r.command(r.root, "docker", "volume", "inspect", engineVolume)
	}
	if err != nil {
		return err
	}
	var volumes []struct {
		Name   string
		Labels map[string]string
	}
	if err = json.Unmarshal(b, &volumes); err != nil {
		return err
	}
	if len(volumes) != 1 || volumes[0].Name != engineVolume || volumes[0].Labels["dev.tokeninsights.owner"] != "validation" {
		return errors.New("cache volume ownership mismatch; leave existing volume untouched")
	}
	return nil
}

func run(ctx context.Context, args []string) (resultErr error) {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	var chosen selection
	fs.Var(&chosen, "group", "group (repeatable)")
	force := fs.Bool("force", false, "execute despite successful receipts")
	status := fs.Bool("status", false, "report reusable group receipts")
	cli := fs.String("dagger", "dagger", "pinned CLI path")
	setup := fs.Bool("setup-engine", false, "create missing managed engine")
	prune := fs.Bool("prune", false, "prune inactive completed evidence")
	storageBenchmarks := fs.Bool("storage-benchmarks", false, "full storage benchmark samples; requires only benchmarks group")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if len(fs.Args()) != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *status && *force {
		return errors.New("status cannot force execution")
	}
	if *storageBenchmarks && (len(chosen) != 1 || chosen[0] != "benchmarks") {
		return errors.New("storage benchmarks require only benchmarks group")
	}
	home, e := os.MkdirTemp("", "validation-home-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(home)
	r := runner{ctx: ctx, home: home, storageBenchmarks: *storageBenchmarks}
	root, e := r.git("rev-parse", "--show-toplevel")
	if e != nil {
		return e
	}
	r.root = root
	sha, e := r.clean()
	if e != nil {
		return e
	}
	tree, e := r.git("rev-parse", "HEAD^{tree}")
	if e != nil {
		return e
	}
	common, e := r.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return e
	}
	common, e = filepath.EvalSymlinks(common)
	if e != nil {
		return e
	}
	base := filepath.Join(common, "tokeninsights-validation", receiptVersion)
	if *prune {
		return pruneRuns(base)
	}
	data, e := r.command(root, "git", "show", sha+":tools/validation/groups.json")
	if e != nil {
		return e
	}
	if e = json.Unmarshal(data, &r.manifest); e != nil {
		return e
	}
	if len(chosen) == 0 {
		for g := range r.manifest.Groups {
			chosen = append(chosen, g)
		}
		sort.Slice(chosen, func(i, j int) bool {
			order := map[string]int{"format": 0, "static": 1, "tooling": 2, "web": 3, "go": 4, "benchmarks": 5, "distribution": 6, "browser": 7, "deployment": 8}
			return order[chosen[i]] < order[chosen[j]]
		})
	}
	for _, g := range chosen {
		if _, ok := r.manifest.Groups[g]; !ok {
			return fmt.Errorf("unknown group %q", g)
		}
	}
	id := uuid.NewString()
	out := filepath.Join(base, "runs", id)
	if !*status {
		if e = os.MkdirAll(out, 0700); e != nil {
			return e
		}
		lease, e := lockFile(filepath.Join(out, "active.lock"))
		if e != nil {
			return e
		}
		defer lease.Close()
		defer func() {
			_ = os.WriteFile(filepath.Join(out, "completed"), []byte(time.Now().UTC().Format(time.RFC3339)), 0600)
		}()
		defer pruneRuns(base)
	}
	var records []receipt
	executed := make(map[string]bool)
	if !*status {
		defer func() {
			outcome := "passed"
			message := ""
			if resultErr != nil {
				outcome = "failed"
				message = resultErr.Error()
			}
			if ctx.Err() != nil {
				outcome = "cancelled"
			}
			if err := saveRun(filepath.Join(out, "run.json"), sha, tree, records, executed, chosen, outcome, message); err != nil && resultErr == nil {
				resultErr = err
			}
		}()
	}
	for _, g := range chosen {
		key, e := r.identity(sha, g)
		if e != nil {
			return e
		}
		path := filepath.Join(common, "tokeninsights-validation", receiptVersion, g, key+".json")
		old, ok := readReceipt(path, key, g)
		if *status {
			fmt.Printf("%s reusable=%t key=%s\n", g, ok, key)
			continue
		}
		if ok && !*force {
			// Pin reused artifacts until this run finishes. Retention uses an
			// exclusive lease and cannot remove evidence beneath a consumer.
			lease, err := os.Open(filepath.Join(filepath.Dir(old.Artifacts), "active.lock"))
			if err == nil {
				err = syscall.Flock(int(lease.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
				if err == nil && evidence(old.Artifacts) == nil {
					defer lease.Close()
					fmt.Printf("%s reused %s\n", g, old.SHA)
					records = append(records, old)
					continue
				}
				lease.Close()
			}
		}
		// A forced retry supersedes prior success. If it fails or is cancelled,
		// status must not silently fall back to the older passing attempt.
		if *force {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		release, e := slot(ctx, r.manifest.Groups[g].Class)
		if e != nil {
			return e
		}
		snap, e := os.MkdirTemp("", "validation-source-")
		if e != nil {
			release()
			return e
		}
		_, e = r.snapshot(sha, g, snap)
		if e != nil {
			release()
			os.RemoveAll(snap)
			return e
		}
		artifacts := filepath.Join(out, g)
		logged := r
		logged.logPath = filepath.Join(out, g+"-launcher.log")
		var err error
		executed[g] = true
		if r.manifest.Groups[g].Executor == "host" {
			err = logged.hostFormat(sha, snap, artifacts)
		} else {
			err = r.checkEngine(*cli, *setup)
			if err == nil {
				args := []string{"--mod", snap, "call", "check", "--source", snap, "--group", g, "--attempt", id}
				if *storageBenchmarks {
					args = append(args, "--storage-benchmarks")
				}
				args = append(args, "export", "--path", artifacts)
				_, err = logged.command(root, *cli, args...)
			}
		}
		release()
		os.RemoveAll(snap)
		if err != nil {
			return err
		}
		if e = evidence(artifacts); e != nil {
			return fmt.Errorf("%s: %w (artifacts %s)", g, e, artifacts)
		}
		mode := "default"
		if *storageBenchmarks {
			mode = "storage-samples"
		}
		rec := receipt{Version: receiptVersion, Group: g, Key: key, SHA: sha, Tree: tree, Artifacts: artifacts, Mode: mode, Completed: time.Now().UTC()}
		if e = saveReceipt(path, rec); e != nil {
			return e
		}
		records = append(records, rec)
	}
	if *status {
		return nil
	}
	current, e := r.clean()
	if e != nil {
		return e
	}
	if current != sha {
		return fmt.Errorf("validated %s; current HEAD %s differs", sha, current)
	}

	return nil
}
func saveRun(path, sha, tree string, records []receipt, executed map[string]bool, requested []string, outcome, message string) error {
	b, e := json.MarshalIndent(struct {
		SHA, Tree                     string
		Groups                        []receipt
		Executed                      map[string]bool
		Dagger, EngineImage, Platform string
		Requested                     []string
		Outcome, Error                string
	}{sha, tree, records, executed, version, engineImage, "linux/amd64", requested, outcome, message}, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx, os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
