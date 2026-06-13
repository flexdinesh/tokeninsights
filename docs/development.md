# Development

```bash
# Check out the repo.
git clone git@github.com:flexdinesh/tokeninsights.git && cd tokeninsights

# Run tests.
pnpm run test

# Build the binary.
pnpm run build

# Install the local CLI build as a binary.
pnpm run install:cli
```

The Go CLI lives in `packages/cli`. Equivalent direct Go commands are:

```bash
cd packages/cli

go test ./...
go build -o ../../bin/tokeninsights ./cmd/tokeninsights
go install ./cmd/tokeninsights
```

## Skipping Actions

`[skip ci]` can be used as a temporary escape hatch when a commit should skip
GitHub Actions, such as a docs-only change that should not run release
automation.

```bash
git commit -m "docs: update readme [skip ci]"
```
