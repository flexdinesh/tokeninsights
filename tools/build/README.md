# Build tools

Private native TypeScript tooling for repository builds, schema validation, tests, releases, fixture-backed development, and controlled copying/comparison of staged web assets. Node 26+ executes these erasable TypeScript files directly.

Production does not depend on this package. The Go binary embeds prebuilt browser assets and runs without Node, npm, pnpm, or `node_modules`.

Run these tools through mise tasks (`mise tasks ls`). Use `mise run format`, `mise run format:check`, and `mise run lint` for repository-wide formatting and linting.

`mise run check:pr` validates committed HEAD before PR publication. Formatting remains on the host; lint, tests and other checks run in isolated Docker Compose projects. Container-only `check:internal:*` tasks must not call public validation wrappers. See [local validation](../../docs/development.md).

`build-products.ts <version> [directory]` builds shared binaries once per native platform and packages CLI and desktop archives. Each product includes its backend; desktop also includes the CLI. `publish-release.ts` verifies the channel's complete platform/product set before publishing a draft. Stable root tags point directly at source commits; main uses one rolling `main-build` prerelease. Neither channel commits generated output. `install/lib.sh` owns checksum verification, archive validation, ownership checks, staged replacement and the best-effort shell profile PATH edit; root installer scripts select the channel, and desktop is the default product unless `--cli-only` is passed. The most recent CLI or desktop install owns the `tokeninsights` command. Unrelated commands reject before replacement. Both warn when PATH runs another command. The deployment group uses private personal TCP and real OpenSSH services, with no production container image.
