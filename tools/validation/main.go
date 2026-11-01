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
	return prepared.WithServiceBinding("sqlite", sqlite).WithServiceBinding("pg", pg).
		WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithEnvVariable("TOKENINSIGHTS_TEST_SQLITE_CONTROL_URL", "http://sqlite:9876").
		WithEnvVariable("TOKENINSIGHTS_TEST_SQLITE_URL", "http://sqlite:8767").
		WithEnvVariable("TOKENINSIGHTS_TEST_POSTGRES_CONTROL_URL", "http://pg:9876").
		WithEnvVariable("TOKENINSIGHTS_TEST_POSTGRES_URL", "http://pg:8767")
}

const postgresImage = "postgres:18@sha256:edb0c08dc33befd0262ab3b5cf1c4f3f8cfd8b09e88c05b45adee29d6761ddb6"

// Check captures check failures as data so the host can export diagnostics.
func (m *Validation) Check(ctx context.Context, source *dagger.Directory, group string, attempt string) (*dagger.Directory, error) {
	if err := validateCheck(group, attempt); err != nil {
		return nil, err
	}
	runner := m.prepare(source, group)
	if group != "contracts" && group != "javascript" {
		webInputs := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: []string{
			"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "packages/*/package.json", "docs-website/package.json",
			"packages/web/**", "tools/build/**", ".oxfmtrc.json", ".oxlintrc.json",
		}})
		web := m.prepare(webInputs, "javascript").WithExec([]string{"node", "tools/build/src/build-web.ts"})
		runner = runner.WithDirectory("/src/packages/cli/internal/server/static", web.Directory("/src/packages/cli/internal/server/static"))
	}
	if group == "browser" {

		runner = runner.WithWorkdir("/src/packages/cli").WithExec([]string{"go", "build", "-o", "bin/tokeninsights", "./cmd/tokeninsights"}).WithExec([]string{"go", "build", "-o", "bin/tokeninsights-server", "./cmd/tokeninsights-server"}).WithWorkdir("/src")
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
	result := runner.WithWorkdir("/src").WithEnvVariable("VALIDATION_ATTEMPT", attempt).
		WithExec([]string{"sh", "-c", captureScript, "capture", group})
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
	base := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From("golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4").
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
	if group == "browser" {
		js = js.WithExec([]string{"sh", "-c", "sed -i 's|http://deb.debian.org/debian-security|https://snapshot.debian.org/archive/debian-security/20261031T000000Z|g; s|http://deb.debian.org/debian|https://snapshot.debian.org/archive/debian/20261031T000000Z|g' /etc/apt/sources.list.d/debian.sources; printf 'Acquire::Check-Valid-Until \"false\";\n' > /etc/apt/apt.conf.d/99validation-snapshot"}).
			WithExec([]string{"pnpm", "--filter", "@tokeninsights/web", "exec", "playwright", "install", "--with-deps", "chromium"})
	}
	if group == "javascript" {
		return js.WithDirectory("/src", source).WithWorkdir("/src")
	}
	goDeps := base.WithDirectory("/deps", source, dagger.ContainerWithDirectoryOpts{Include: []string{"packages/cli/go.mod", "packages/cli/go.sum"}}).
		WithWorkdir("/deps/packages/cli").WithExec([]string{"go", "mod", "download"})
	if group == "go" || group == "all" {
		goDeps = goDeps.WithDirectory("/deps", source, dagger.ContainerWithDirectoryOpts{Include: []string{"packages/cli/golangci-lint.mod", "packages/cli/golangci-lint.sum"}}).WithExec([]string{"go", "mod", "download", "-modfile=golangci-lint.mod"})
	}
	if group == "contracts" {
		goDeps = goDeps.WithDirectory("/validation-deps", source, dagger.ContainerWithDirectoryOpts{Include: []string{"tools/validation/go.mod", "tools/validation/go.sum"}}).
			WithWorkdir("/validation-deps/tools/validation").WithExec([]string{"go", "mod", "download"})
	}
	return js.WithDirectory("/go/pkg/mod", goDeps.Directory("/go/pkg/mod")).WithDirectory("/src", source).WithWorkdir("/src")
}
