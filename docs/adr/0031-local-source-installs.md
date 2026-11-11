# Local source installs replace rolling CI builds

Status: Accepted. Date: 12 November 2026. User directed that non-stable installs
build locally from any branch and that CI stop prebuilding them.

## Context

Each push to main ran two CI workflows. `ci.yml` published the rolling
`main-build` prerelease (CLI and desktop), and `edge.yml` published the rolling
`edge-build` prerelease (desktop-native only, plus manual dispatch for any ref).
`install-main.sh` and `install-edge.sh` downloaded them. Both channels omitted
macOS Intel, could race publication against installers, and tied testing any
branch to a CI run and a single shared rolling release.

## Decision

Stable stays the only published release: manual **Release** builds every
platform, `install.sh` downloads it, and its behavior is unchanged.

Every other install builds on the installing host:

- `install-dev.sh [--ref=<branch|tag|sha>] [--cli-only] [--no-modify-path]`
  builds desktop, or the CLI product with `--cli-only`.
- `install-dev-native.sh [--ref=...]` builds only TokenInsights Native and
  rejects `--cli-only`.
- The default ref is `main`. Refs come only from the canonical repository and
  resolve once to a commit with `git ls-remote`; a branch wins over a
  same-named tag and a 40-hex SHA is used as is.
- Both wrappers run `install/source.sh`. It requires `git` and mise, and builds
  under a `mkdir` lock in a persistent `${XDG_CACHE_HOME:-~/.cache}/tokeninsights/source`
  checkout that keeps only `node_modules` between builds. mise installs the
  locked toolchain. The build runs `pnpm install --frozen-lockfile`, then
  `build:web`, then `build-products.ts src.<sha12> <dir> <product> --ref=<ref>`.
- `install/lib.sh --channel=source --product=<product>` installs that local
  archive with the same verification, staging and rollback as stable. `.channel`
  records `source`; `build-info.json` records `ref` beside `commit`.
- Refs that predate `install/source.sh` reject before building.
  `TOKENINSIGHTS_SOURCE_URL` overrides the repository for test fixtures only.

`ci.yml`, `edge.yml`, `install-main.sh`, `install-edge.sh`, the `main` and `edge`
installer channels, and the `main.<sha12>`/`edge.<sha12>` versions are removed.
`publish-release.ts` publishes stable only. Existing `main-build` and `edge-build`
prereleases are left untouched and are no longer updated.

This supersedes the rolling `main-build` prerelease in ADR 0027 and ADR 0024, and
the `edge` channel clause in ADR 0030. The native app remains the experiment ADR
0030 describes; only its distribution changes.

## Consequences

- Non-stable installs need git, mise and network access for the toolchain and
  dependencies, and take minutes on first build. Later builds reuse the cache.
- Any macOS or Linux host on amd64/arm64 can install any ref, including macOS
  Intel. Locally built apps carry no download quarantine.
- Building a ref runs that ref's build tooling and pnpm lifecycle scripts, which
  is acceptable only because refs come from the canonical repository.
- Old `install-main.sh`/`install-edge.sh` URLs stop resolving.
- Stable and source installs replace the same product directory.
- The cache is never pruned automatically.

## Verification

Installer contract tests use real Git against a local repository with stubbed
mise: default ref, branch/tag/SHA resolution, unknown/invalid/legacy refs,
missing mise, lock contention, failed builds preserving the product and cache,
an unowned cache path, native command isolation and piped helper fetching.
The distribution group installs a real native archive through the source channel.
Validation containers have no network, so a full source build is verified by hand
in an isolated HOME rather than in the distribution group.
