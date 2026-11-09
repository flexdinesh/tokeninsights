// validate runs isolated checks on committed HEAD: format on the host, every
// other group in its own Docker Compose project.
package main

import (
	"bytes"
	"context"
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
	// docker is the Docker CLI; env carries the caller's Docker endpoint
	// settings and compose variables into otherwise private subprocesses.
	docker string
	env    []string
}

// dockerEnvironment keeps the caller's Docker endpoint and CLI plugins while
// subprocesses otherwise run with a private HOME.
func dockerEnvironment() []string {
	var env []string
	for _, name := range []string{"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CERT_PATH", "DOCKER_TLS_VERIFY"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	config := os.Getenv("DOCKER_CONFIG")
	if config == "" {
		if home, err := os.UserHomeDir(); err == nil {
			config = filepath.Join(home, ".docker")
		}
	}
	if config != "" {
		env = append(env, "DOCKER_CONFIG="+config)
	}
	return env
}

func (r runner) command(dir, name string, args ...string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Join(r.home, "tmp"), 0700); err != nil {
		return nil, err
	}
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + r.home, "XDG_CONFIG_HOME=" + r.home + "/config", "XDG_CACHE_HOME=" + r.home + "/cache", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0"}
	c.Env = append(c.Env, "TMPDIR="+filepath.Join(r.home, "tmp"))
	if r.hostPath != "" {
		c.Env[0] = "PATH=" + r.hostPath
		c.Env = append(c.Env, "MISE_TRUSTED_CONFIG_PATHS="+dir, "MISE_TASK_RUN_AUTO_INSTALL=false", "GOTOOLCHAIN=local")
	}
	c.Env = append(c.Env, r.env...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.WaitDelay = 5 * time.Second
	var output bytes.Buffer
	c.Stdout, c.Stderr = &output, &output
	if r.logPath != "" {
		// Concurrent groups keep complete output in their own log files.
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
		if r.logPath != "" {
			return b, fmt.Errorf("%s %s: %w (log %s)", name, firstArg(args), e, r.logPath)
		}
		return b, fmt.Errorf("%s %v: %w\n%s", name, args, e, b)
	}
	return b, nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
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
	files, e := r.git("ls-files", "-z")
	if e != nil {
		return "", e
	}
	for _, path := range strings.Split(files, "\x00") {
		if generatedArtifactPath(path) {
			return "", fmt.Errorf("generated output must remain untracked: %s", path)
		}
	}
	return r.git("rev-parse", "HEAD")
}

func generatedArtifactPath(path string) bool {
	if strings.HasPrefix(path, "dist/") || strings.HasPrefix(path, "packages/core/server/static/") || strings.Contains(path, ".app/") {
		return true
	}
	for _, suffix := range []string{".tar.gz", ".tgz", ".dmg", ".deb", ".AppImage"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	for _, product := range []string{"cli", "backend", "desktop"} {
		for _, output := range []string{"bin", "build", "dist", "out", "release", ".mygo"} {
			if strings.HasPrefix(path, "packages/"+product+"/"+output+"/") {
				return true
			}
		}
	}
	return false
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

func run(ctx context.Context, args []string) (resultErr error) {
	started := time.Now().UTC()
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	var chosen selection
	fs.Var(&chosen, "group", "group (repeatable)")
	force := fs.Bool("force", false, "execute despite successful receipts")
	status := fs.Bool("status", false, "report reusable group receipts")
	docker := fs.String("docker", "docker", "Docker CLI path")
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
	r := runner{ctx: ctx, home: home, storageBenchmarks: *storageBenchmarks, docker: *docker, env: dockerEnvironment()}
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
	if e = r.manifest.validate(); e != nil {
		return e
	}
	if len(chosen) == 0 {
		chosen = defaultGroups()
	}
	chosen = uniqueGroups(chosen)
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
	timings := make(map[string]groupTiming)
	slot := -1
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
			if err := saveRun(filepath.Join(out, "run.json"), sha, tree, records, executed, chosen, outcome, message, started, timings, slot); err != nil && resultErr == nil {
				resultErr = err
			}
		}()
	}
	keys := make(map[string]string)
	var pending []string
	for _, g := range chosen {
		key, e := r.identity(sha, g)
		if e != nil {
			return e
		}
		path := filepath.Join(base, g, key+".json")
		old, ok := readReceipt(path, key, g)
		if *status {
			fmt.Printf("%s reusable=%t key=%s\n", g, ok, key)
			continue
		}
		keys[g] = key
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
					timings[g] = groupTiming{Reused: true}
					continue
				}
				lease.Close()
			}
		}
		pending = append(pending, g)
	}
	if *status || len(pending) == 0 {
		return nil
	}
	temp, e := os.MkdirTemp("", "validation-source-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	source := filepath.Join(temp, "tree")
	if e = r.extract(sha, source); e != nil {
		return e
	}
	inv := invocation{sha: sha, tree: tree, out: out, base: base, source: source, temp: temp, force: *force, queued: time.Now(), executionGroup: func(group string) string {
		if *storageBenchmarks {
			return "storage"
		}
		return group
	}}
	admitRun := func(ctx context.Context) (func(), error) {
		dir, err := admissionDir()
		if err != nil {
			return nil, err
		}
		fmt.Println("validate: waiting for an admission slot")
		acquired, release, err := admit(ctx, dir)
		if err != nil {
			return nil, err
		}
		local := r
		local.logPath = filepath.Join(out, "admission.log")
		if err = sweep(local, temp, acquired); err == nil {
			err = ensureVolumes(local, temp)
		}
		if err != nil {
			release()
			return nil, fmt.Errorf("prepare slot %d: %w (log %s)", acquired, err, local.logPath)
		}
		slot = acquired
		inv.slot = acquired
		fmt.Printf("validate: admitted on slot %d\n", acquired)
		return release, nil
	}
	err := scheduleChecks(ctx, pending, admitRun, func(ctx context.Context, group string) groupResult {
		local := r
		local.ctx = ctx
		fmt.Printf("validate: %s started\n", group)
		return local.executeGroup(inv, group, keys[group])
	}, func(group string, result groupResult) {
		timings[group] = result.timing
		executed[group] = result.executed
		report(group, result)
		if result.err == nil {
			records = append(records, result.receipt)
		}
	})
	if err != nil {
		return err
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

func report(group string, result groupResult) {
	elapsed := time.Duration(result.timing.TotalMS-result.timing.QueueMS) * time.Millisecond
	if result.err == nil {
		fmt.Printf("validate: %s passed (%s)\n", group, elapsed.Round(time.Second))
	} else {
		fmt.Printf("validate: %s failed (%s): %v\n", group, elapsed.Round(time.Second), result.err)
		if result.log != "" {
			if lines := tail(result.log, failureTail); lines != "" {
				fmt.Printf("--- last %d lines of %s ---\n%s\n---\n", failureTail, result.log, lines)
			}
		}
	}
	if result.warning != "" {
		fmt.Printf("validate: %s warning: %s\n", group, result.warning)
	}
}

func saveRun(path, sha, tree string, records []receipt, executed map[string]bool, requested []string, outcome, message string, started time.Time, timings map[string]groupTiming, slot int) error {
	b, e := json.MarshalIndent(struct {
		SHA, Tree          string
		Groups             []receipt
		Executed           map[string]bool
		Platform           string
		Slot               int
		Requested          []string
		Outcome, Error     string
		Started, Completed time.Time
		Timings            map[string]groupTiming
	}{sha, tree, records, executed, platform, slot, requested, outcome, message, started, time.Now().UTC(), timings}, "", "  ")
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
