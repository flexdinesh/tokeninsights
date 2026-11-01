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
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const version = "0.21.11"
const engine = "tokeninsights-validation-engine-v0.21.11"
const engineImage = "registry.dagger.io/engine@sha256:6b557cc8b5fdfee0e2e8c82d451ac922097a1255482143e1b96c0be3ff2a303c"
const engineVolume = "tokeninsights-validation-cache-v0.21.11"
const receiptVersion = "v1"

var groups = []string{"contracts", "javascript", "go", "benchmarks", "distribution", "browser", "deployment"}

type selection []string

func (s *selection) String() string { return strings.Join(*s, ",") }
func (s *selection) Set(v string) error {
	for _, g := range groups {
		if v == g {
			*s = append(*s, v)
			return nil
		}
	}
	return fmt.Errorf("unknown group %q", v)
}

type receipt struct {
	Version, Group, Key, SHA, Tree, Artifacts string
	Completed                                 time.Time
}
type runner struct {
	ctx        context.Context
	root, home string
	logPath    string
}

func (r runner) command(dir, name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + r.home, "XDG_CONFIG_HOME=" + r.home + "/config", "XDG_CACHE_HOME=" + r.home + "/cache", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "DAGGER_LEAVE_OLD_ENGINE=true", "_EXPERIMENTAL_DAGGER_RUNNER_HOST=docker-container://" + engine, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0"}
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
		stream := io.MultiWriter(&output, log)
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
func selected(group, path string) bool {
	if group == "contracts" {
		return true
	}
	if strings.HasSuffix(path, "/package.json") || path == "docs/openapi.yaml" {
		return true
	}
	if (group == "distribution" && (strings.HasPrefix(path, "docs/") || strings.HasPrefix(path, ".github/") || strings.HasPrefix(path, "deploy/"))) || (group == "javascript" && strings.HasPrefix(path, "docs-website/")) {
		return true
	}
	if group == "deployment" && strings.HasPrefix(path, "packages/") {
		return true
	}
	if !strings.Contains(path, "/") || strings.HasPrefix(path, "tools/") || strings.HasPrefix(path, ".mise/") {
		return true
	}
	prefixes := []string{"schema/", "packages/web/", "packages/cli/"}
	if group == "javascript" {
		prefixes = append(prefixes, ".agents/plugins/", ".claude-plugin/", ".husky/", ".github/")
	}
	if group == "contracts" || group == "javascript" || group == "browser" || group == "distribution" {
		prefixes = append(prefixes, "packages/", "plugins/", "tests/")
	}
	if group == "deployment" {
		prefixes = append(prefixes, "deploy/", "docker/", "tests/")
	}
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
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
		if !selected(group, hdr.Name) {
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
func admission(ctx context.Context) (func(), error) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("local-validation-%d", os.Getuid()))
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	st, e := os.Lstat(dir)
	if e != nil {
		return nil, e
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) || !st.IsDir() || st.Mode().Perm() != 0700 {
		return nil, errors.New("unsafe admission directory")
	}
	f, e := os.OpenFile(filepath.Join(dir, "capacity.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	for {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(e, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, e
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
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

// Publication is a host operation. Credentials never enter validation runners.
func (r runner) publishCommand(name string, args ...string) ([]byte, error) {
	c := exec.CommandContext(r.ctx, name, args...)
	c.Dir = r.root
	allowed := []string{"PATH", "HOME", "XDG_CONFIG_HOME", "GH_CONFIG_DIR", "GH_TOKEN", "GITHUB_TOKEN", "SSH_AUTH_SOCK"}
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			c.Env = append(c.Env, key+"="+value)
		}
	}
	c.Env = append(c.Env, "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
	b, err := c.CombinedOutput()
	if err != nil {
		return b, fmt.Errorf("publication %s: %w", name, err)
	}
	return b, nil
}
func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	var chosen selection
	fs.Var(&chosen, "group", "group (repeatable)")
	force := fs.Bool("force", false, "execute despite successful receipts")
	status := fs.Bool("status", false, "report reusable group receipts")
	cli := fs.String("dagger", "dagger", "pinned CLI path")
	setup := fs.Bool("setup-engine", false, "create missing managed engine")
	create := fs.Bool("pr-create", false, "validate then push exact HEAD and create PR")
	update := fs.Bool("pr-update", false, "validate then push exact HEAD and update PR")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if *create && *update {
		return errors.New("choose create or update")
	}
	if *status && (*create || *update || *force) {
		return errors.New("status cannot publish or force execution")
	}
	if (*create || *update) && len(chosen) > 0 {
		return errors.New("PR requires all groups")
	}
	if len(chosen) == 0 {
		chosen = groups
	}
	home, e := os.MkdirTemp("", "validation-home-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(home)
	r := runner{ctx: ctx, home: home}
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
	release, e := admission(ctx)
	if e != nil {
		return e
	}
	defer release()
	if !*status {
		if e = r.checkEngine(*cli, *setup); e != nil {
			return e
		}
	}
	id := uuid.NewString()
	out := filepath.Join(root, ".check-logs", id)
	if e = os.MkdirAll(out, 0700); e != nil {
		return e
	}
	var records []receipt
	executed := make(map[string]bool)
	for _, g := range chosen {
		snap, e := os.MkdirTemp("", "validation-source-")
		if e != nil {
			return e
		}
		defer os.RemoveAll(snap)
		key, e := r.snapshot(sha, g, snap)
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
			fmt.Printf("%s reused %s\n", g, old.SHA)
			records = append(records, old)
			continue
		}
		artifacts := filepath.Join(out, g)
		fmt.Printf("%s executing (logs %s)\n", g, out)
		logged := r
		logged.logPath = filepath.Join(out, g+"-launcher.log")
		_, err := logged.command(root, *cli, "--mod", snap, "call", "check", "--source", snap, "--group", g, "--attempt", id, "export", "--path", artifacts)
		if err != nil {
			return err
		}
		if e = evidence(artifacts); e != nil {
			return fmt.Errorf("%s: %w (artifacts %s)", g, e, artifacts)
		}
		rec := receipt{receiptVersion, g, key, sha, tree, artifacts, time.Now().UTC()}
		if e = saveReceipt(path, rec); e != nil {
			return e
		}
		records = append(records, rec)
		executed[g] = true
	}
	if *status {
		return nil
	}
	if e = saveRun(filepath.Join(out, "run.json"), sha, tree, records, executed); e != nil {
		return e
	}
	if *create || *update {
		current, e := r.clean()
		if e != nil {
			return e
		}
		if current != sha {
			return errors.New("HEAD moved since validation")
		}
		branch, e := r.git("symbolic-ref", "--short", "HEAD")
		if e != nil {
			return e
		}
		if _, e = r.publishCommand("git", "push", "origin", sha+":refs/heads/"+branch); e != nil {
			return e
		}
		current, e = r.clean()
		if e != nil {
			return e
		}
		if current != sha {
			return errors.New("HEAD moved during push")
		}
		if !*create {
			return nil
		}
		b, e := r.publishCommand("gh", append([]string{"pr", "create", "--head", branch}, fs.Args()...)...)
		fmt.Print(string(b))
		return e
	}
	return nil
}
func saveRun(path, sha, tree string, records []receipt, executed map[string]bool) error {
	b, e := json.MarshalIndent(struct {
		SHA, Tree                     string
		Groups                        []receipt
		Executed                      map[string]bool
		Dagger, EngineImage, Platform string
	}{sha, tree, records, executed, version, engineImage, "linux/amd64"}, "", "  ")
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
