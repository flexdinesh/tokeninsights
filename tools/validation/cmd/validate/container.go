package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Resource names are fixed per repository so other repositories sharing the
// admission slots never see or remove them.
const (
	resourcePrefix = "tokeninsights-check"
	imageRepo      = resourcePrefix
	teardownLimit  = 2 * time.Minute
	composeLabel   = "com.docker.compose.project"
	failureTail    = 40
)

var cacheVolumes = []string{resourcePrefix + "-go-build", resourcePrefix + "-golangci-lint"}

// serviceTargets lists the per-slot images a group's compose services build.
func serviceTargets(group string) []string {
	if group == "deployment" {
		return []string{"keys", "sshd", "personal"}
	}
	return nil
}

func projectName(slot int, group string) string {
	return fmt.Sprintf("%s-s%d-%s", resourcePrefix, slot, group)
}

func slotPrefix(slot int) string { return fmt.Sprintf("%s-s%d-", resourcePrefix, slot) }

func imageTag(slot int, target string) string {
	return fmt.Sprintf("%s:s%d-%s", imageRepo, slot, target)
}

func groupImages(slot int, group string) []string {
	images := []string{imageTag(slot, group)}
	for _, target := range serviceTargets(group) {
		images = append(images, imageTag(slot, target))
	}
	return images
}

// captureScript records check failures as data so the host can export
// diagnostics. Deployment receives its SSH identity from the project volume.
const captureScript = `mkdir -p /evidence/artifacts
{ if [ -f /keys/id ]; then install -D -m 600 /keys/id /root/.ssh/id; fi && mise run --no-deps --skip-tools "check:internal:$1"; } > /evidence/check.log 2>&1
code=$?
for path in packages/web/test-results packages/web/playwright-report .check-logs; do
 if [ -d "$path" ]; then cp -R "$path" /evidence/artifacts/ || code=1; fi
done
git diff > /evidence/artifacts/generated.diff 2>> /evidence/check.log || code=1
printf '{"exitCode":%s}\n' "$code" > /evidence/status.json
`

// containerRun owns one group's compose project from build to teardown.
type containerRun struct {
	r          runner
	slot       int
	group      string
	task       string
	tree       string
	contexts   map[string]string
	artifacts  string
	logs       string
	workdir    string
	composeEnv []string
}

func (c containerRun) project() string { return projectName(c.slot, c.group) }

func (c containerRun) runnerName() string { return c.project() + "-runner" }

func (c containerRun) composeFile() string {
	return filepath.Join(c.tree, "tools", "validation", "compose.yaml")
}

func (c containerRun) environment() []string {
	env := []string{
		"CHECK_SLOT=" + fmt.Sprint(c.slot),
		"CHECK_GROUP=" + c.group,
		"CHECK_DOCKERFILE=" + filepath.Join(c.tree, "tools", "validation", "Dockerfile"),
		"CHECK_CONTEXT_EMPTY=" + c.contexts["empty"],
		"CHECK_CONTEXT_GROUP=" + c.contexts[groupContext],
	}
	for _, name := range sharedContexts {
		env = append(env, "CHECK_CONTEXT_"+strings.ToUpper(name)+"="+c.contexts[name])
	}
	if c.group == "deployment" {
		env = append(env, "CHECK_SSH_ENDPOINT=ssh://remote@sshd", "CHECK_PERSONAL_URL=http://personal:9876")
	}
	return env
}

func (c containerRun) compose(ctx context.Context, log string, args ...string) error {
	local := c.r
	local.ctx = ctx
	local.logPath = filepath.Join(c.logs, log)
	local.env = append(append([]string{}, c.r.env...), c.environment()...)
	base := []string{"compose", "--progress", "plain", "-p", c.project(), "-f", c.composeFile()}
	if len(serviceTargets(c.group)) > 0 {
		base = append(base, "--profile", c.group)
	}
	_, err := local.command(c.workdir, c.r.docker, append(base, args...)...)
	return err
}

// prepare builds the runner and service images, then starts services. It
// returns the log of the step that ran last.
func (c containerRun) prepare(ctx context.Context) (string, error) {
	build := c.group + "-build.log"
	if err := c.compose(ctx, build, append([]string{"build", "runner"}, serviceTargets(c.group)...)...); err != nil {
		return build, fmt.Errorf("image build failed: %w", err)
	}
	services := serviceTargets(c.group)
	if len(services) == 0 {
		return build, nil
	}
	up := c.group + "-services-up.log"
	// keys is a one-shot dependency of sshd; --wait accepts its completion.
	started := slices.DeleteFunc(slices.Clone(services), func(s string) bool { return s == "keys" })
	if err := c.compose(ctx, up, append([]string{"up", "--detach", "--wait", "--no-build"}, started...)...); err != nil {
		return up, fmt.Errorf("service startup failed: %w", err)
	}
	return up, nil
}

// execute runs the check and copies its evidence out of the stopped runner.
func (c containerRun) execute(ctx context.Context) error {
	runErr := c.compose(ctx, c.group+"-run.log", "run", "-T", "--no-deps", "--name", c.runnerName(), "runner", "sh", "-c", captureScript, "capture", c.task)
	if err := os.MkdirAll(c.artifacts, 0700); err != nil {
		return err
	}
	local := c.r
	local.ctx = context.WithoutCancel(ctx)
	local.logPath = ""
	if _, err := local.command(c.workdir, c.r.docker, "cp", c.runnerName()+":/evidence/.", c.artifacts); err != nil && runErr == nil {
		runErr = fmt.Errorf("evidence export failed: %w", err)
	}
	if runErr != nil {
		return fmt.Errorf("check container failed: %w", runErr)
	}
	return nil
}

// teardown removes everything the project owns, whatever happened before.
func (c containerRun) teardown(ctx context.Context) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), teardownLimit)
	defer cancel()
	if len(serviceTargets(c.group)) > 0 {
		_ = c.compose(cleanup, c.group+"-services.log", "logs", "--no-color", "--timestamps")
	}
	local := c.r
	local.ctx = cleanup
	local.logPath = filepath.Join(c.logs, c.group+"-teardown.log")
	var failures []string
	if _, err := local.command(c.workdir, c.r.docker, "rm", "-f", c.runnerName()); err != nil && !strings.Contains(err.Error(), "No such container") {
		failures = append(failures, err.Error())
	}
	if err := c.compose(cleanup, c.group+"-teardown-compose.log", "down", "-v", "--remove-orphans", "--timeout", "10"); err != nil {
		failures = append(failures, err.Error())
	}
	if err := removeImages(local, c.workdir, groupImages(c.slot, c.group)); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return fmt.Errorf("teardown %s: %s", c.project(), strings.Join(failures, "; "))
	}
	return nil
}

func removeImages(r runner, dir string, images []string) error {
	present, err := r.command(dir, r.docker, "image", "ls", "--format", "{{.Repository}}:{{.Tag}}", imageRepo)
	if err != nil {
		return err
	}
	var remove []string
	for _, line := range strings.Fields(string(present)) {
		if slices.Contains(images, line) {
			remove = append(remove, line)
		}
	}
	if len(remove) == 0 {
		return nil
	}
	_, err = r.command(dir, r.docker, append([]string{"image", "rm"}, remove...)...)
	return err
}

// sweep removes resources a crashed earlier holder of this slot left behind.
// Holding the slot lock proves no live run owns s<slot> resources.
func sweep(r runner, dir string, slot int) error {
	prefix := slotPrefix(slot)
	projects := make(map[string]bool)
	for _, kind := range []string{"container", "network", "volume"} {
		args := []string{kind, "ls", "--filter", "label=" + composeLabel, "--format", "{{.Label \"" + composeLabel + "\"}}"}
		if kind == "container" {
			args = []string{"container", "ls", "--all", "--filter", "label=" + composeLabel, "--format", "{{.Label \"" + composeLabel + "\"}}"}
		}
		out, err := r.command(dir, r.docker, args...)
		if err != nil {
			return err
		}
		for _, project := range strings.Fields(string(out)) {
			if strings.HasPrefix(project, prefix) {
				projects[project] = true
			}
		}
	}
	names := make([]string, 0, len(projects))
	for project := range projects {
		names = append(names, project)
	}
	slices.Sort(names)
	for _, project := range names {
		filter := "label=" + composeLabel + "=" + project
		for _, kind := range []string{"container", "network", "volume"} {
			args := []string{kind, "ls", "--quiet", "--filter", filter}
			if kind == "container" {
				args = []string{"container", "ls", "--all", "--quiet", "--filter", filter}
			}
			out, err := r.command(dir, r.docker, args...)
			if err != nil {
				return err
			}
			if ids := strings.Fields(string(out)); len(ids) > 0 {
				remove := []string{kind, "rm"}
				if kind == "container" {
					remove = append(remove, "-f")
				}
				if _, err := r.command(dir, r.docker, append(remove, ids...)...); err != nil {
					return err
				}
			}
		}
	}
	present, err := r.command(dir, r.docker, "image", "ls", "--format", "{{.Repository}}:{{.Tag}}", imageRepo)
	if err != nil {
		return err
	}
	var stale []string
	for _, image := range strings.Fields(string(present)) {
		if strings.HasPrefix(image, fmt.Sprintf("%s:s%d-", imageRepo, slot)) {
			stale = append(stale, image)
		}
	}
	if len(stale) > 0 {
		if _, err := r.command(dir, r.docker, append([]string{"image", "rm"}, stale...)...); err != nil {
			return err
		}
	}
	return nil
}

// ensureVolumes creates the shared accelerator volumes; create is idempotent.
func ensureVolumes(r runner, dir string) error {
	for _, volume := range cacheVolumes {
		if _, err := r.command(dir, r.docker, "volume", "create", volume); err != nil {
			return err
		}
	}
	return nil
}

func tail(path string, lines int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var kept []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		kept = append(kept, scanner.Text())
		if len(kept) > lines {
			kept = kept[1:]
		}
	}
	return strings.Join(kept, "\n")
}
