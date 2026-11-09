package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeDocker records argv and simulates compose, cp and listing commands.
const fakeDocker = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_LOG"
case "$1" in
compose)
  for arg in "$@"; do
    case "$arg" in
    run)
      if [ -n "$FAKE_RUN_SLEEP" ]; then sleep "$FAKE_RUN_SLEEP"; fi
      exit "${FAKE_RUN_EXIT:-0}" ;;
    build) exit "${FAKE_BUILD_EXIT:-0}" ;;
    esac
  done
  exit 0 ;;
cp)
  mkdir -p "$3"
  printf '{"exitCode":%s}\n' "${FAKE_STATUS:-0}" > "$3/status.json"
  echo check-output > "$3/check.log"
  exit 0 ;;
image)
  if [ "$2" = ls ]; then printf '%s\n' $FAKE_IMAGES; fi
  exit 0 ;;
container|network|volume)
  if [ "$2" = ls ]; then
    case "$*" in
    *--quiet*)
      for arg in "$@"; do project=${arg##*=}; done
      printf '%s\n' "$1:$project" ;;
    *) printf '%s\n' $FAKE_PROJECTS ;;
    esac
  fi
  exit 0 ;;
esac
exit 0
`

type dockerFixture struct {
	r   runner
	log string
	inv invocation
}

func newDockerFixture(t *testing.T, env ...string) dockerFixture {
	t.Helper()
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	if err := os.WriteFile(docker, []byte(fakeDocker), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "argv.log")
	source := filepath.Join(dir, "tree")
	if err := os.MkdirAll(filepath.Join(source, "packages"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"pnpm-lock.yaml", "packages/main.go"} {
		if err := os.WriteFile(filepath.Join(source, file), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := runner{ctx: t.Context(), root: dir, home: t.TempDir(), docker: docker, env: append([]string{"FAKE_LOG=" + log}, env...), manifest: manifest{
		Common:   []string{"*", "packages/"},
		Contexts: map[string]contextSpec{"deps": {Inputs: []string{"pnpm-lock.yaml"}}, "web": {}, "go": {}},
		Groups: map[string]groupSpec{
			"web":        {Executor: "container", Uses: []string{"deps"}},
			"deployment": {Executor: "container", Uses: []string{"deps"}},
		},
	}}
	inv := invocation{sha: "sha", tree: "tree", out: t.TempDir(), base: t.TempDir(), source: source, temp: t.TempDir(), slot: 1, queued: time.Now(), executionGroup: func(g string) string { return g }}
	return dockerFixture{r: r, log: log, inv: inv}
}

func (f dockerFixture) calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// ordered asserts each fragment appears in a later call than the previous one.
func ordered(t *testing.T, calls []string, fragments ...string) {
	t.Helper()
	next := 0
	for _, fragment := range fragments {
		found := false
		for next < len(calls) {
			next++
			if strings.Contains(calls[next-1], fragment) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in order:\n%s", fragment, strings.Join(calls, "\n"))
		}
	}
}

func TestNamesAreSlotAndRepositoryScoped(t *testing.T) {
	if got := projectName(1, "go"); got != "tokeninsights-check-s1-go" {
		t.Fatal(got)
	}
	if got := imageTag(0, "sshd"); got != "tokeninsights-check:s0-sshd" {
		t.Fatal(got)
	}
	if got := groupImages(1, "deployment"); !slices.Equal(got, []string{"tokeninsights-check:s1-deployment", "tokeninsights-check:s1-keys", "tokeninsights-check:s1-sshd", "tokeninsights-check:s1-personal"}) {
		t.Fatal(got)
	}
	if got := groupImages(0, "web"); !slices.Equal(got, []string{"tokeninsights-check:s0-web"}) {
		t.Fatal(got)
	}
}

func TestDeploymentBuildsStartsRunsCopiesAndTearsDown(t *testing.T) {
	f := newDockerFixture(t, "FAKE_IMAGES=tokeninsights-check:s1-deployment tokeninsights-check:s1-sshd tokeninsights-check:s0-go tokeninsights-check:s1-keys tokeninsights-check:s1-personal")
	result := f.r.executeGroup(f.inv, "deployment", "key")
	if result.err != nil || result.warning != "" {
		t.Fatal(result.err, result.warning)
	}
	calls := f.calls(t)
	project := "-p tokeninsights-check-s1-deployment"
	ordered(t, calls,
		project+" -f "+filepath.Join(f.inv.source, "tools/validation/compose.yaml")+" --profile deployment build runner keys sshd personal",
		project+" -f "+filepath.Join(f.inv.source, "tools/validation/compose.yaml")+" --profile deployment up --detach --wait --no-build sshd personal",
		"run -T --no-deps --name tokeninsights-check-s1-deployment-runner runner sh -c",
		"cp tokeninsights-check-s1-deployment-runner:/evidence/. "+filepath.Join(f.inv.out, "deployment"),
		"logs --no-color",
		"rm -f tokeninsights-check-s1-deployment-runner",
		project+" -f "+filepath.Join(f.inv.source, "tools/validation/compose.yaml")+" --profile deployment down -v --remove-orphans",
		"image rm tokeninsights-check:s1-deployment tokeninsights-check:s1-sshd tokeninsights-check:s1-keys tokeninsights-check:s1-personal",
	)
	for _, call := range calls {
		if strings.Contains(call, "s0-go") && strings.Contains(call, "image rm") {
			t.Fatal("removed another slot's image", call)
		}
	}
	if !slices.ContainsFunc(calls, func(c string) bool { return strings.HasSuffix(c, "capture deployment") }) {
		t.Fatal("check task not passed to capture")
	}
	if _, ok := readReceipt(filepath.Join(f.inv.base, "deployment", "key.json"), "key", "deployment"); !ok {
		t.Fatal("passing group not recorded")
	}
}

func TestComposeReceivesContextsAndNoServiceProfileForPlainGroups(t *testing.T) {
	f := newDockerFixture(t)
	dir := t.TempDir()
	env := filepath.Join(dir, "env")
	// Record the compose environment of the build call.
	wrapper := filepath.Join(dir, "docker")
	script := "#!/bin/sh\ncase \"$*\" in *' build '*) env > " + env + ";; esac\nexec " + f.r.docker + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	f.r.docker = wrapper
	if result := f.r.executeGroup(f.inv, "web", "key"); result.err != nil {
		t.Fatal(result.err)
	}
	for _, call := range f.calls(t) {
		if strings.Contains(call, "--profile") {
			t.Fatal("plain group enabled service profile", call)
		}
	}
	data, err := os.ReadFile(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CHECK_SLOT=1", "CHECK_GROUP=web", "CHECK_CONTEXT_GROUP=", "CHECK_CONTEXT_DEPS=", "CHECK_CONTEXT_WEB=", "CHECK_CONTEXT_GO=", "CHECK_CONTEXT_EMPTY=", "CHECK_DOCKERFILE=" + filepath.Join(f.inv.source, "tools/validation/Dockerfile")} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("compose environment lacks %s", want)
		}
	}
	if strings.Contains(string(data), "CHECK_SSH_ENDPOINT") {
		t.Fatal("plain group received service endpoints")
	}
}

func TestFailedCheckTearsDownAndRecordsNothing(t *testing.T) {
	f := newDockerFixture(t, "FAKE_STATUS=1")
	result := f.r.executeGroup(f.inv, "web", "key")
	if result.err == nil {
		t.Fatal("failed status accepted")
	}
	ordered(t, f.calls(t), "run -T", "cp ", "rm -f tokeninsights-check-s1-web-runner", "down -v")
	if _, ok := readReceipt(filepath.Join(f.inv.base, "web", "key.json"), "key", "web"); ok {
		t.Fatal("failed group recorded")
	}
	if result.log != filepath.Join(f.inv.out, "web", "check.log") {
		t.Fatal("failure log", result.log)
	}
}

func TestBuildFailureSkipsRunAndTearsDown(t *testing.T) {
	f := newDockerFixture(t, "FAKE_BUILD_EXIT=1")
	result := f.r.executeGroup(f.inv, "web", "key")
	if result.err == nil {
		t.Fatal("build failure accepted")
	}
	calls := f.calls(t)
	if slices.ContainsFunc(calls, func(c string) bool { return strings.Contains(c, "run -T") }) {
		t.Fatal("ran check after build failure")
	}
	ordered(t, calls, "build runner", "rm -f", "down -v")
	if result.log != filepath.Join(f.inv.out, "web-build.log") {
		t.Fatal("failure log", result.log)
	}
}

func TestCancellationTearsDownRunningProject(t *testing.T) {
	f := newDockerFixture(t, "FAKE_RUN_SLEEP=30")
	ctx, cancel := context.WithCancel(t.Context())
	f.r.ctx = ctx
	done := make(chan groupResult, 1)
	go func() { done <- f.r.executeGroup(f.inv, "web", "key") }()
	deadline := time.After(10 * time.Second)
	for {
		data, _ := os.ReadFile(f.log)
		if strings.Contains(string(data), "run -T") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("run never started")
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	select {
	case result := <-done:
		if result.err == nil {
			t.Fatal("cancelled group succeeded")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("cancellation hung")
	}
	ordered(t, f.calls(t), "run -T", "rm -f tokeninsights-check-s1-web-runner", "down -v --remove-orphans")
}

func TestSweepRemovesOnlyThisRepositorysSlot(t *testing.T) {
	f := newDockerFixture(t,
		"FAKE_PROJECTS=tokeninsights-check-s0-go tokeninsights-check-s1-web tokeninsights-check-s10-go diffx-check-s0-go",
		"FAKE_IMAGES=tokeninsights-check:s0-go tokeninsights-check:s1-go tokeninsights-check:s10-go",
	)
	if err := sweep(f.r, t.TempDir(), 1); err != nil {
		t.Fatal(err)
	}
	var removals []string
	for _, call := range f.calls(t) {
		if strings.Contains(call, " rm") {
			removals = append(removals, call)
		}
	}
	want := []string{
		"container rm -f container:tokeninsights-check-s1-web",
		"network rm network:tokeninsights-check-s1-web",
		"volume rm volume:tokeninsights-check-s1-web",
		"image rm tokeninsights-check:s1-go",
	}
	if !slices.Equal(removals, want) {
		t.Fatalf("removals:\n%s", strings.Join(removals, "\n"))
	}
}

func TestCaptureScriptPreservesFailureAndArtifacts(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "evidence")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte("#!/bin/sh\necho \"diagnostic $*\"\nexit 23\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho generated-diff\n"), 0755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(root, "packages/web/test-results")
	if err := os.MkdirAll(artifact, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifact, "trace.zip"), []byte("trace"), 0644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", strings.ReplaceAll(captureScript, "/evidence", evidence), "capture", "browser")
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	for path, want := range map[string]string{"status.json": "{\"exitCode\":23}\n", "check.log": "diagnostic run --no-deps --skip-tools check:internal:browser\n", "artifacts/test-results/trace.zip": "trace", "artifacts/generated.diff": "generated-diff\n"} {
		data, err := os.ReadFile(filepath.Join(evidence, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s: %q", path, data)
		}
	}
}
