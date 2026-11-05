// Isolated local validation preparation.

package main

import (
	"context"
	"dagger/validation/internal/dagger"
	"errors"
	"fmt"
	"time"
)

type Validation struct{}

const goImage = "golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4"
const aptSnapshot = "sed -i 's|http://deb.debian.org/debian-security|https://snapshot.debian.org/archive/debian-security/20261031T000000Z|g; s|http://deb.debian.org/debian|https://snapshot.debian.org/archive/debian/20261031T000000Z|g' /etc/apt/sources.list.d/debian.sources; rm -f /etc/apt/apt.conf.d/docker-clean; printf 'Acquire::Check-Valid-Until \"false\";\nBinary::apt::APT::Keep-Downloaded-Packages \"true\";\n' > /etc/apt/apt.conf.d/99validation-snapshot"

// aptCache names the shared apt caches for the pinned snapshot above; change it
// with the snapshot date. Caches are accelerators only (ADR 0022): they keep
// downloaded packages and indexes across runs and failed attempts, so a
// transient snapshot mirror failure retries without refetching everything.
const aptCache = "tokeninsights-validation-apt-trixie-20261031"

const goBuildCachePath = "/cache/go-build"
const goModuleCachePath = "/cache/go-mod"
const goLintCachePath = "/cache/golangci-lint"
const pnpmStorePath = "/cache/pnpm"
const serviceStopTimeout = 30 * time.Second

// Only tool caches persist. Explicit paths keep Go compilation/module reuse
// when fixtures replace HOME or XDG_CACHE_HOME; homes, databases and credentials
// remain private. Go and golangci-lint coordinate concurrent cache writers.
func withGoCaches(c *dagger.Container) *dagger.Container {
	return c.WithEnvVariable("GOCACHE", goBuildCachePath).
		WithEnvVariable("GOMODCACHE", goModuleCachePath).
		WithEnvVariable("GOLANGCI_LINT_CACHE", goLintCachePath).
		WithMountedCache(goBuildCachePath, dag.CacheVolume("tokeninsights-validation-go-build-1.27.3-linux-amd64")).
		WithMountedCache(goModuleCachePath, dag.CacheVolume("tokeninsights-validation-go-mod-1.27.3")).
		WithMountedCache(goLintCachePath, dag.CacheVolume("tokeninsights-validation-golangci-lint-go1.27.3-linux-amd64"))
}

// aptGuardPath shadows apt-get on PATH while apt runs, including the apt-get
// that Playwright's --with-deps starts.
const aptGuardPath = "/usr/local/bin/apt-get"

// aptGuard bounds apt under snapshot rate limiting, where it has flooded
// gigabytes of "delayed item" warnings and reached 30 GB RSS, taking down the
// host (the engine has no memory limit). Each attempt gets a 4 GiB address
// space cap and a 10 minute timeout, the flood is dropped before it reaches
// Dagger's log capture, and failures retry with backoff, reusing packages
// already in the cache.
const aptGuard = `#!/bin/bash
for wait in 30 90 0; do
  (ulimit -v 4194304; exec timeout 600 /usr/bin/apt-get "$@") 2>&1 | grep --line-buffered -v 'Tried to start delayed item'
  code=${PIPESTATUS[0]}
  [ "$code" -eq 0 ] && exit 0
  [ "$wait" -eq 0 ] && exit "$code"
  echo "apt-get exited $code; retrying in ${wait}s" >&2
  sleep "$wait"
done
`

// withApt pins the snapshot, installs the guard and mounts the shared apt
// caches, locked so concurrent groups never write one cache at once. withoutApt
// removes the guard and mounts so prepared containers carry neither.
func withApt(c *dagger.Container) *dagger.Container {
	locked := dagger.ContainerWithMountedCacheOpts{Sharing: dagger.CacheSharingModeLocked}
	return c.WithMountedCache("/var/cache/apt", dag.CacheVolume(aptCache+"-archives"), locked).
		WithMountedCache("/var/lib/apt/lists", dag.CacheVolume(aptCache+"-lists"), locked).
		WithNewFile(aptGuardPath, aptGuard, dagger.ContainerWithNewFileOpts{Permissions: 0o755}).
		WithExec([]string{"sh", "-c", aptSnapshot})
}

func withoutApt(c *dagger.Container) *dagger.Container {
	return c.WithoutFile(aptGuardPath).WithoutMount("/var/cache/apt").WithoutMount("/var/lib/apt/lists")
}

// withSSHRemote binds a real OpenSSH server running the CLI built from source,
// installed on the remote's default non-interactive PATH, for SSH endpoint
// contracts. The runner gets a private key and pins the host on first use.
func withSSHRemote(prepared *dagger.Container, binaries *dagger.Directory, attempt string) (*dagger.Container, *dagger.Service) {
	keys := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(goImage).
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithExec([]string{"sh", "-c", "mkdir -p /keys && ssh-keygen -q -t ed25519 -N '' -C validation -f /keys/id"}).Directory("/keys")
	sshd := withoutApt(withApt(dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(goImage)).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "--no-install-recommends", "openssh-server"})).
		WithExec([]string{"useradd", "--create-home", "--shell", "/bin/sh", "remote"}).
		WithDirectory("/opt/tokeninsights", binaries).
		WithExec([]string{"ln", "-s", "/opt/tokeninsights/tokeninsights", "/usr/local/bin/tokeninsights"}).
		WithExec([]string{"install", "-d", "-m", "700", "-o", "remote", "-g", "remote", "/home/remote/.ssh"}).
		WithFile("/home/remote/.ssh/authorized_keys", keys.File("id.pub"), dagger.ContainerWithFileOpts{Owner: "remote", Permissions: 0o600}).
		WithExec([]string{"sh", "-c", "rm -f /etc/ssh/ssh_host_* && ssh-keygen -A && mkdir -p /run/sshd"}).
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithExposedPort(22).
		AsService(dagger.ContainerAsServiceOpts{Args: []string{"/usr/sbin/sshd", "-D", "-e"}})
	// OpenSSH reads the passwd home (/root), not $HOME.
	config := "Host sshd\n  IdentityFile /root/.ssh/id\n  IdentitiesOnly yes\n  StrictHostKeyChecking accept-new\n  UserKnownHostsFile /root/.ssh/known_hosts\n  BatchMode yes\n"
	return prepared.WithExec([]string{"install", "-d", "-m", "700", "/root/.ssh"}).
		WithFile("/root/.ssh/id", keys.File("id"), dagger.ContainerWithFileOpts{Permissions: 0o600}).
		WithNewFile("/root/.ssh/config", config, dagger.ContainerWithNewFileOpts{Permissions: 0o600}).
		WithServiceBinding("sshd", sshd).
		WithEnvVariable("TOKENINSIGHTS_TEST_SSH_ENDPOINT", "ssh://remote@sshd"), sshd
}

// withPersonalRemote exercises the real CLI gateway across an isolated network.
// The control service and its temporary credentials belong to this invocation.
func withPersonalRemote(prepared *dagger.Container, binaries *dagger.Directory, fixture *dagger.File, attempt string) (*dagger.Container, *dagger.Service) {
	// This standard-library-only supervisor has an independent build input, so
	// fixture changes never invalidate the shared production binaries.
	supervisor := withGoCaches(dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(goImage)).
		WithEnvVariable("CGO_ENABLED", "0").WithEnvVariable("GOTOOLCHAIN", "local").
		WithEnvVariable("GOMAXPROCS", "2").WithEnvVariable("GOFLAGS", "-mod=readonly -buildvcs=false").
		WithFile("/src/main.go", fixture).
		WithExec([]string{"go", "build", "-o", "/out/personal-fixture", "/src/main.go"}).File("/out/personal-fixture")
	service := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(goImage).
		WithDirectory("/opt/tokeninsights", binaries).
		WithFile("/opt/tokeninsights/personal-fixture", supervisor).
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithEnvVariable("HOME", "/private/home").
		WithEnvVariable("XDG_DATA_HOME", "/private/data").
		WithEnvVariable("XDG_CONFIG_HOME", "/private/config").
		WithExposedPort(8767).WithExposedPort(9876).
		AsService(dagger.ContainerAsServiceOpts{Args: []string{"/opt/tokeninsights/personal-fixture"}})
	return prepared.WithServiceBinding("personal", service).
		WithEnvVariable("TOKENINSIGHTS_TEST_PERSONAL_CONTROL_URL", "http://personal:9876"), service
}

func (m *Validation) webAssets(source *dagger.Directory) *dagger.Directory {
	inputs := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: webBuildInputs(), Exclude: webBuildExcludes()})
	return m.prepare(inputs, "web").WithExec([]string{"node", "tools/build/src/build-web.ts"}).Directory("/src/packages/core/server/static")
}

// Browser, SSH and TCP use identical pure-Go builds. Distribution intentionally
// uses build-products.ts instead: its release linker metadata is part of coverage.
func (m *Validation) binaries(source *dagger.Directory, assets *dagger.Directory) *dagger.Directory {
	inputs := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: goBuildInputs(), Exclude: goBuildExcludes()})
	return m.prepare(inputs, "go").WithDirectory("/src/packages/core/server/static", assets).
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-o", "/out/tokeninsights", "./packages/cli/cmd/tokeninsights"}).
		WithExec([]string{"go", "build", "-o", "/out/tokeninsights-backend", "./packages/backend/cmd/tokeninsights-backend"}).Directory("/out")
}

func stopService(ctx context.Context, service *dagger.Service) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), serviceStopTimeout)
	defer cancel()
	_, _ = service.Stop(cleanup)
}

// Check captures check failures as data so the host can export diagnostics.
func (m *Validation) Check(ctx context.Context, source *dagger.Directory, group string, attempt string,
	// Run full storage benchmark samples instead of smoke.
	// +optional
	storageBenchmarks bool,
) (*dagger.Directory, error) {
	if err := validateCheck(group, attempt); err != nil {
		return nil, err
	}
	if storageBenchmarks && group != "benchmarks" {
		return nil, fmt.Errorf("storage benchmarks require benchmarks group")
	}
	preparationStart := time.Now()
	var timing checkTiming
	var services []*dagger.Service
	runner := m.prepare(source, group)
	if needsWebAssets(group) {
		assets := m.webAssets(source)
		runner = runner.WithDirectory("/src/packages/core/server/static", assets)
		if group == "browser" || group == "deployment" {
			binaries := m.binaries(source, assets)
			runner = runner.WithDirectory("/src/packages/cli/bin", binaries)
			if group == "deployment" {
				var sshd, personal *dagger.Service
				runner, sshd = withSSHRemote(runner, binaries, attempt)
				runner, personal = withPersonalRemote(runner, binaries, source.File("tools/validation/fixtures/personal/main.go"), attempt)
				services = append(services, sshd, personal)
			}
		}
	}
	if _, err := runner.Sync(ctx); err != nil {
		timing.PreparationMS = time.Since(preparationStart).Milliseconds()
		return preparationFailure(err, timing), nil
	}
	// Service startup belongs to preparation. Keep these invocation-owned
	// services alive until execution finishes, including partial-startup failure.
	for _, service := range services {
		defer stopService(ctx, service)
		if _, err := service.Start(ctx); err != nil {
			timing.PreparationMS = time.Since(preparationStart).Milliseconds()
			return preparationFailure(err, timing), nil
		}
	}
	timing.PreparationMS = time.Since(preparationStart).Milliseconds()
	executionGroup := group
	if storageBenchmarks {
		executionGroup = "storage"
	}
	// Introduce the nonce only after immutable preparation. Persistent compiler
	// caches never substitute for executing the checks (Go retains -count=1).
	executionStart := time.Now()
	result := runner.WithWorkdir("/src").WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithExec([]string{"sh", "-c", captureScript, "capture", executionGroup})
	_, err := result.Sync(ctx)
	timing.ExecutionMS = time.Since(executionStart).Milliseconds()
	if err != nil {
		return preparationFailure(err, timing), nil
	}
	return result.Directory("/evidence").WithNewFile("timing.json", timing.JSON()), nil
}

func preparationFailure(err error, timing checkTiming) *dagger.Directory {
	log := "Validation preparation, service startup or execution failed: " + err.Error() + "\n"
	var execution *dagger.ExecError
	if errors.As(err, &execution) {
		log += fmt.Sprintf("Command: %q\nExit code: %d\nStdout:\n%s\nStderr:\n%s\n", execution.Cmd, execution.ExitCode, execution.Stdout, execution.Stderr)
	}
	return dag.Directory().WithNewFile("status.json", "{\"exitCode\":1}\n").WithNewFile("check.log", log).
		WithNewFile("timing.json", timing.JSON())
}

// Prepare caches dependency graphs independently from application source.
func (m *Validation) Prepare(source *dagger.Directory) *dagger.Container {
	return m.prepare(source, "all")
}

func (m *Validation) prepare(source *dagger.Directory, group string) *dagger.Container {
	node := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From("node:26.12.1-trixie-slim@sha256:36793bcbb160a6a16cede1e62b93f84e26b78a972a8a4c0b879c1c12f22761ec")
	base := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(goImage).
		WithDirectory("/usr/local", node.Directory("/usr/local")).
		WithEnvVariable("PATH", "/usr/local/bin:/usr/local/go/bin:/usr/bin:/bin").
		WithEnvVariable("GOTOOLCHAIN", "local").WithEnvVariable("GOFLAGS", "-mod=readonly -buildvcs=false").
		WithEnvVariable("GOMAXPROCS", "2").WithEnvVariable("TOKENINSIGHTS_CHECK_CONCURRENCY", "2").
		WithEnvVariable("HOME", "/private/home").WithEnvVariable("XDG_CONFIG_HOME", "/private/config").
		WithEnvVariable("XDG_DATA_HOME", "/private/data").WithEnvVariable("XDG_CACHE_HOME", "/private/cache").
		WithEnvVariable("TMPDIR", "/private/tmp").WithEnvVariable("HUSKY", "0").
		WithEnvVariable("MISE_TRUSTED_CONFIG_PATHS", "/src").WithEnvVariable("MISE_TASK_RUN_AUTO_INSTALL", "false").
		WithExec([]string{"mkdir", "-p", "/private/home", "/private/config", "/private/data", "/private/cache", "/private/tmp"}).
		WithExec([]string{"npm", "install", "--global", "pnpm@12.15.0"}).
		WithExec([]string{"sh", "-c", "curl -fsSL https://github.com/jdx/mise/releases/download/v2026.10.18/mise-v2026.10.18-linux-x64.tar.gz -o /tmp/mise.tar.gz && echo '0d41b60745a6273a80089c86f80d8729042d2f57f87fda70aecc7d1c3ffbd3be  /tmp/mise.tar.gz' | sha256sum -c - && tar xzf /tmp/mise.tar.gz -C /tmp && cp /tmp/mise/bin/mise /usr/local/bin/mise"})
	base = withGoCaches(base)
	manifests := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: dependencyManifestInputs()})
	js := base.WithMountedCache(pnpmStorePath, dag.CacheVolume("tokeninsights-validation-pnpm-12.15.0-linux-amd64")).
		WithDirectory("/src", manifests).WithWorkdir("/src").WithExec([]string{"pnpm", "install", "--frozen-lockfile", "--store-dir", pnpmStorePath})
	if group == "browser" || group == "distribution" || group == "tooling" {
		js = withApt(js)
	}
	if group == "tooling" {
		js = js.WithExec([]string{"apt-get", "update"}).WithExec([]string{"apt-get", "install", "-y", "--no-install-recommends", "libarchive-tools"})
	}
	if group == "browser" || group == "distribution" {
		js = js.WithExec([]string{"pnpm", "--filter", "@tokeninsights/web", "exec", "playwright", "install", "--with-deps", "chromium"}).
			WithExec([]string{"apt-get", "install", "-y", "--no-install-recommends", "xvfb", "xauth", "x11-utils", "libgtk-3-0", "libwebkit2gtk-4.1-0"})
	}
	if group == "browser" || group == "distribution" || group == "tooling" {
		js = withoutApt(js)
	}
	if group == "web" {
		return js.WithDirectory("/src", source).WithWorkdir("/src")
	}
	goDeps := base.WithDirectory("/deps", source, dagger.ContainerWithDirectoryOpts{Include: []string{"go.mod", "go.sum"}}).
		WithWorkdir("/deps").WithExec([]string{"go", "mod", "download"})
	if group == "static" || group == "all" {
		goDeps = goDeps.WithDirectory("/deps", source, dagger.ContainerWithDirectoryOpts{Include: []string{"golangci-lint.mod", "golangci-lint.sum"}}).WithExec([]string{"go", "mod", "download", "-modfile=golangci-lint.mod"})
	}
	if group == "tooling" {
		goDeps = goDeps.WithDirectory("/validation-deps", source, dagger.ContainerWithDirectoryOpts{Include: []string{"tools/validation/go.mod", "tools/validation/go.sum"}}).
			WithWorkdir("/validation-deps/tools/validation").WithExec([]string{"go", "mod", "download"})
	}
	// Materialize dependency preparation without copying a mounted cache into an
	// immutable layer. Both branches mount the same module cache; /deps preserves
	// the download dependency before application source enters the graph.
	return js.WithDirectory("/deps", goDeps.Directory("/deps")).WithDirectory("/src", source).WithWorkdir("/src")
}
