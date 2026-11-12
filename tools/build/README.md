# Build tools

Private native TypeScript tooling for repository builds, schema validation, tests, releases, fixture-backed development, and controlled copying/comparison of staged web assets. Node 26+ executes these erasable TypeScript files directly.

Production does not depend on this package. The Go binary embeds prebuilt browser assets and runs without Node, npm, pnpm, or `node_modules`.

Run these tools through mise tasks (`mise tasks ls`). Use `mise run format`, `mise run format:check`, and `mise run lint` for repository-wide formatting and linting.

`mise run check:pr` validates committed HEAD before PR publication. Formatting remains on the host; lint, tests and other checks run in isolated Docker Compose projects. Container-only `check:internal:*` tasks must not call public validation wrappers. See [local validation](../../docs/development.md).

`build-products.ts <x.y.z|src.sha12> [directory] [products...] [--ref=<ref>]` builds shared binaries once for the host platform and packages the selected archives (stable defaults to CLI and desktop; source versions require explicit products and record `ref` in `build-info.json`). Each product includes its backend; desktop also includes the CLI. `publish-release.ts` verifies the complete stable platform/product set before publishing a draft. Stable root tags point directly at source commits and commit no generated output; no other release is published. `install/lib.sh` owns checksum verification, archive validation, ownership checks, staged replacement and the best-effort shell profile PATH edit. `install.sh` selects stable, and desktop is the default product unless `--cli-only` is passed. `install-dev.sh` and `install-dev-native.sh` run `install/source.sh`, which resolves a canonical ref, builds it in a locked cache checkout and installs the archive through `lib.sh --channel=source`. The most recent CLI or desktop install owns the `tokeninsights` command. Unrelated commands reject before replacement. Both warn when PATH runs another command. The deployment group uses private personal TCP and real OpenSSH services, with no production container image.
