# Build tools

Private native TypeScript tooling for repository builds, schema validation, tests, releases, and fixture-backed development. Node 26+ executes these erasable TypeScript files directly.

Production does not depend on this package. The Go binary embeds prebuilt browser assets and runs without Node, npm, pnpm, or `node_modules`.

Run these tools through the root package scripts. Use `pnpm run format`, `pnpm run format:check`, and `pnpm run lint` for repository-wide formatting and linting.
