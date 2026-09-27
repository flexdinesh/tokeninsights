# Build tools

Private native TypeScript tooling for repository builds, schema validation, tests, releases, fixture-backed development, and controlled copying/comparison of staged web assets. Node 26+ executes these erasable TypeScript files directly.

CI runs `node tools/build/src/publish-dev.ts` after successful verification on `main`. It publishes the checked-out `GITHUB_SHA` to `dev`, skips superseded runs, and uses an explicit Git lease to avoid overwriting concurrent branch updates. Tests use isolated local Git repositories.

Production does not depend on this package. The Go binary embeds prebuilt browser assets and runs without Node, npm, pnpm, or `node_modules`.

Run these tools through the root package scripts. Use `pnpm run format`, `pnpm run format:check`, and `pnpm run lint` for repository-wide formatting and linting.
