# Build tools

Private native TypeScript tooling for repository builds, schema validation, tests, releases, fixture-backed development, and controlled copying/comparison of staged web assets. Node 26+ executes these erasable TypeScript files directly.

Production does not depend on this package. The Go binary embeds prebuilt browser assets and runs without Node, npm, pnpm, or `node_modules`.

Run these tools through mise tasks (`mise tasks ls`). Use `mise run format`, `mise run format:check`, and `mise run lint` for repository-wide formatting and linting.

`mise run check:pr` validates committed HEAD before PR publication. Formatting remains on the host; lint, tests and other checks run through Dagger. Container-only `check:internal:*` tasks must not call public validation wrappers. See [local validation](../../docs/development.md).
