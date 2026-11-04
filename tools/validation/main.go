// Isolated local validation preparation.

package main

import (
	"context"
	"dagger/validation/internal/dagger"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Validation struct{}

// Deployment connects the existing semantic assertions to supervisors mounted
// into the actual production image. Attempt must be unique for every execution.
func (m *Validation) Deployment(source *dagger.Directory, prepared *dagger.Container, attempt string) *dagger.Container {
	if strings.TrimSpace(attempt) == "" {
		panic("attempt is required")
	}
	fixture := prepared.WithWorkdir("/src/packages/cli").
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-o", "/out/deployment-fixture", "./tools/deployment-fixture"}).File("/out/deployment-fixture")
	postgres := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From("postgres:18@sha256:edb0c08dc33befd0262ab3b5cf1c4f3f8cfd8b09e88c05b45adee29d6761ddb6").
		WithEnvVariable("POSTGRES_PASSWORD", "validation-private").
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithExposedPort(5432).AsService()
	image := source.DockerBuild().WithoutDockerHealthcheck().WithoutExposedPort(8766).WithFile("/usr/local/bin/deployment-fixture", fixture).
		WithEntrypoint([]string{"/usr/local/bin/deployment-fixture"}).
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithEnvVariable("TOKENINSIGHTS_SERVER_PORT", "8767").
		WithEnvVariable("TOKENINSIGHTS_BOOTSTRAP_USERNAME", "admin").
		WithEnvVariable("TOKENINSIGHTS_BOOTSTRAP_PASSWORD_FILE", "/data/bootstrap_password").
		WithExposedPort(9876)
	sqlite := image.WithEnvVariable("TOKENINSIGHTS_STORAGE_BACKEND", "sqlite").
		WithEnvVariable("TOKENINSIGHTS_SERVER_DB_PATH", "/data/server.sqlite").AsService()
	pg := image.WithEnvVariable("TOKENINSIGHTS_STORAGE_BACKEND", "postgres").
		WithServiceBinding("postgres", postgres).
		WithEnvVariable("TOKENINSIGHTS_POSTGRES_DSN_FILE", "/data/postgres_dsn").
		WithEnvVariable("TOKENINSIGHTS_FIXTURE_POSTGRES_DSN", "postgres://postgres:validation-private@postgres:5432/postgres?sslmode=disable").AsService()
	return withSSHRemote(prepared, attempt).WithServiceBinding("sqlite", sqlite).WithServiceBinding("pg", pg).
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithEnvVariable("TOKENINSIGHTS_TEST_SQLITE_CONTROL_URL", "http://sqlite:9876").
		WithEnvVariable("TOKENINSIGHTS_TEST_SQLITE_URL", "http://sqlite:8767").
		WithEnvVariable("TOKENINSIGHTS_TEST_POSTGRES_CONTROL_URL", "http://pg:9876").
		WithEnvVariable("TOKENINSIGHTS_TEST_POSTGRES_URL", "http://pg:8767")
}

const postgresImage = "postgres:18@sha256:edb0c08dc33befd0262ab3b5cf1c4f3f8cfd8b09e88c05b45adee29d6761ddb6"
const goImage = "golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4"
const aptSnapshot = "sed -i 's|http://deb.debian.org/debian-security|https://snapshot.debian.org/archive/debian-security/20261031T000000Z|g; s|http://deb.debian.org/debian|https://snapshot.debian.org/archive/debian/20261031T000000Z|g' /etc/apt/sources.list.d/debian.sources; rm -f /etc/apt/apt.conf.d/docker-clean; printf 'Acquire::Check-Valid-Until \"false\";\nBinary::apt::APT::Keep-Downloaded-Packages \"true\";\n' > /etc/apt/apt.conf.d/99validation-snapshot"

// aptCache names the shared apt caches for the pinned snapshot above; change it
// with the snapshot date. Caches are accelerators only (ADR 0022): they keep
// downloaded packages and indexes across runs and failed attempts, so a
// transient snapshot mirror failure retries without refetching everything.
const aptCache = "tokeninsights-validation-apt-trixie-20261031"

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
func withSSHRemote(prepared *dagger.Container, attempt string) *dagger.Container {
	binaries := prepared.WithWorkdir("/src/packages/cli").WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-o", "/out/tokeninsights", "./cmd/tokeninsights"}).
		WithExec([]string{"go", "build", "-o", "/out/tokeninsights-backend", "../backend/cmd/tokeninsights-backend"}).Directory("/out")
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
		WithExec([]string{"sh", "-c", "ssh-keygen -A && mkdir -p /run/sshd"}).
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithExposedPort(22).
		AsService(dagger.ContainerAsServiceOpts{Args: []string{"/usr/sbin/sshd", "-D", "-e"}})
	// OpenSSH reads the passwd home (/root), not $HOME.
	config := "Host sshd\n  IdentityFile /root/.ssh/id\n  IdentitiesOnly yes\n  StrictHostKeyChecking accept-new\n  UserKnownHostsFile /root/.ssh/known_hosts\n  BatchMode yes\n"
	return prepared.WithExec([]string{"install", "-d", "-m", "700", "/root/.ssh"}).
		WithFile("/root/.ssh/id", keys.File("id"), dagger.ContainerWithFileOpts{Permissions: 0o600}).
		WithNewFile("/root/.ssh/config", config, dagger.ContainerWithNewFileOpts{Permissions: 0o600}).
		WithServiceBinding("sshd", sshd).
		WithEnvVariable("TOKENINSIGHTS_TEST_SSH_ENDPOINT", "ssh://remote@sshd")
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
	runner := m.prepare(source, group)
	if group != "web" {
		webInputs := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: []string{
			"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "packages/*/package.json", "docs-website/package.json",
			"packages/web/**", "packages/dashboard/**", "packages/contracts/**", "tools/build/package.json", "tools/build/src/build-web.ts",
			"tools/build/src/sync-web-assets.ts", "tools/build/src/files.ts", "tools/build/src/workspace.ts",
			".oxfmtrc.json", ".oxlintrc.json",
		}})
		web := m.prepare(webInputs, "web").WithExec([]string{"node", "tools/build/src/build-web.ts"})
		runner = runner.WithDirectory("/src/packages/core/server/static", web.Directory("/src/packages/core/server/static"))
	}
	if group == "browser" {

		runner = runner.WithWorkdir("/src/packages/cli").WithExec([]string{"go", "build", "-o", "bin/tokeninsights", "./cmd/tokeninsights"}).WithExec([]string{"go", "build", "-o", "bin/tokeninsights-backend", "../backend/cmd/tokeninsights-backend"}).WithExec([]string{"go", "build", "-o", "../server/bin/tokeninsights-server", "../server/cmd/tokeninsights-server"}).WithWorkdir("/src")
	}
	if _, err := runner.Sync(ctx); err != nil {
		return preparationFailure(err), nil
	}
	if group == "go" || group == "benchmarks" {
		pg := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(postgresImage).WithEnvVariable("POSTGRES_PASSWORD", "validation-private").WithEnvVariable("VALIDATION_ATTEMPT", attempt).WithExposedPort(5432).AsService()
		runner = runner.WithServiceBinding("postgres", pg).WithEnvVariable("TOKENINSIGHTS_TEST_POSTGRES_DSN", "postgres://postgres:validation-private@postgres:5432/postgres?sslmode=disable")
	}
	if group == "deployment" {
		args, err := source.DockerBuild().DockerHealthcheck().Args(ctx)
		if err != nil {
			return preparationFailure(err), nil
		}
		if !slices.Equal(args, []string{"/usr/local/bin/tokeninsights-server", "healthcheck"}) {
			return nil, fmt.Errorf("unexpected image healthcheck: %v", args)
		}
		runner = m.Deployment(source, runner, attempt)
	}
	executionGroup := group
	if storageBenchmarks {
		executionGroup = "storage"
	}
	result := runner.WithWorkdir("/src").WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithExec([]string{"sh", "-c", captureScript, "capture", executionGroup})
	if _, err := result.Sync(ctx); err != nil {
		return preparationFailure(err), nil
	}
	return result.Directory("/evidence"), nil
}

func preparationFailure(err error) *dagger.Directory {
	log := "Validation preparation or service startup failed: " + err.Error() + "\n"
	var execution *dagger.ExecError
	if errors.As(err, &execution) {
		log += fmt.Sprintf("Command: %q\nExit code: %d\nStdout:\n%s\nStderr:\n%s\n", execution.Cmd, execution.ExitCode, execution.Stdout, execution.Stderr)
	}
	return dag.Directory().WithNewFile("status.json", "{\"exitCode\":1}\n").WithNewFile("check.log", log)
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
	manifests := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: []string{"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "packages/*/package.json", "tools/build/package.json", "docs-website/package.json"}})
	js := base.WithDirectory("/src", manifests).WithWorkdir("/src").WithExec([]string{"pnpm", "install", "--frozen-lockfile"})
	if group == "browser" || group == "distribution" || group == "tooling" {
		js = withApt(js)
	}
	if group == "tooling" {
		js = js.WithExec([]string{"apt-get", "update"}).WithExec([]string{"apt-get", "install", "-y", "--no-install-recommends", "libarchive-tools"})
	}
	if group == "browser" || group == "distribution" {
		js = js.WithExec([]string{"pnpm", "--filter", "@tokeninsights/web", "exec", "playwright", "install", "--with-deps", "chromium"}).
			WithExec([]string{"apt-get", "install", "-y", "--no-install-recommends", "xvfb", "xauth", "x11-utils", "libgtk-3-0", "libasound2t64", "libwebkit2gtk-4.1-0"})
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
	return js.WithDirectory("/go/pkg/mod", goDeps.Directory("/go/pkg/mod")).WithDirectory("/src", source).WithWorkdir("/src")
}
