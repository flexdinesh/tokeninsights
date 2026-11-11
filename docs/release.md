# Releases and installation

Install stable prebuilt products from GitHub Releases. No Go, Node, npm, pnpm or repository checkout is required on the user's machine. Unreleased code builds locally from source; CI publishes nothing else.

## Install

Desktop, CLI, TUI, browser launcher and their backend, latest stable:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh
```

CLI, TUI, browser launcher and their backend, for servers without a display:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh -s -- --cli-only
```

Any branch, tag or full commit SHA, built on this machine (default `main`):

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-dev.sh | sh -s -- --ref=my-branch
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-dev-native.sh | sh -s -- --ref=my-branch
```

`install.sh` installs the desktop product, which bundles the CLI and backend; `--cli-only` selects the CLI product instead. Rerun to upgrade; a rerun without `--cli-only` installs desktop. Installers verify a unique SHA-256 checksum, product identity and archive layout, then stage replacement in user-owned storage. An incomplete download or checksum failure leaves the installed product intact. `TOKENINSIGHTS_DOWNLOAD_URL` overrides the download base for local release fixtures.

`install-dev.sh` (desktop, or CLI with `--cli-only`) and `install-dev-native.sh` (TokenInsights Native only; rejects `--cli-only`) share `install/source.sh`. It requires `git` and mise, accepts refs only from the canonical repository and resolves the ref to one commit with `git ls-remote` (a branch wins over a same-named tag; a 40-hex SHA is used as is). Under a `mkdir` lock it fetches that commit into `${XDG_CACHE_HOME:-~/.cache}/tokeninsights/source`, checks it out detached and cleans every untracked file except `node_modules`. Inside the checkout it runs `mise install` (locked toolchain), `pnpm install --frozen-lockfile` and `mise run build:web` with `HUSKY=0`, then `build-products.ts src.<sha12> <temporary-directory> <product> --ref=<ref>`. The resulting archive installs through `install/lib.sh --channel=source --product=<product>`, so verification, staging and rollback match stable. Refs that predate `install/source.sh` reject before building. A failed build leaves the installed product and a reusable cache. Source and stable installs replace the same product; `.channel` records `stable` or `source`, and `build-info.json` records `ref` beside `commit`. `TOKENINSIGHTS_SOURCE_URL` overrides the repository for local fixtures.

Product files live under `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/products/`. Command links use `~/.local/bin`; macOS apps use `~/Applications`. When `~/.local/bin` is not on PATH, a completed install adds one marked, idempotent line to the `$SHELL` profile: zsh `${ZDOTDIR:-~}/.zshenv`; bash `~/.bashrc` plus `~/.bash_profile` or `~/.profile`; fish `conf.d/tokeninsights.fish`; otherwise `~/.profile`. A failed edit only warns, and `--no-modify-path` opts out. Installation starts no background service. Desktop-native installs to `products/desktop-native` as `TokenInsights Native`, with its own Linux entry `applications/tokeninsights-desktop-native.desktop` (`~/.local/bin/tokeninsights-native` launcher) or macOS `~/Applications/TokenInsights Native.app`. It registers only the `tokeninsights-native` URL scheme, never creates, replaces or owns `~/.local/bin/tokeninsights`, never edits shell profiles, and coexists with an installed desktop or CLI product. Each client product contains its own backend; desktop installation also exposes the CLI. The most recent CLI or desktop install owns `tokeninsights`, and both warn when PATH runs another command. Removing another client cannot remove the selected product's backend dependency. Close viewers before upgrading an incompatible local-client version. Installers never terminate running viewers.

Homebrew and versioned `go install` delivery are no longer supported. Historical releases/tags, including retired `main-build` and `edge-build` prereleases, remain downloadable but are no longer updated.

## Publish

Stable: dispatch **Release** from main with an explicit semantic version. The workflow builds the selected source, creates an immutable root `vX.Y.Z` source tag, stages complete archives/checksums in a draft, then publishes the stable release. Reruns must retain the same source identity; published stable bytes are not overwritten. It is the only workflow; pushes to main build and publish nothing. Validate locally first and test other platforms by hand with `install-dev.sh`, which also builds on macOS Intel. See [ADR 0031](adr/0031-local-source-installs.md).

Stable archives are named `tokeninsights-<cli|desktop>_<darwin|linux>_<amd64|arm64>.tar.gz`, with `checksums.txt`, and share release version/source metadata. Source builds produce one host archive, `tokeninsights-<cli|desktop|desktop-native>_<os>_<arch>.tar.gz`, with version `src.<sha12>`; desktop-native ships only this way. Every archive contains the required Go backend; desktop also bundles the CLI, desktop-native does not. There is no hosted server product or server installer.

No compiled executables, desktop bundles, archives or generated dashboard assets are committed to any branch or tag. Source tags point to source commits. Build tooling generates embedded browser assets before building Go; release attachments contain compiled output. No generated asset commits, edge branch, rolling prerelease, tap token or tap PR automation remain. See [ADR 0027](adr/0027-personal-backend-and-explicit-connections.md).

## Runtime and verification

Backend, CLI and desktop use Go with `CGO_ENABLED=0`. The backend embeds browser assets and never invoke JavaScript tools. Stable archives are built on matching native runners; source builds run on the installing host. `mise run build:desktop-native` builds the desktop-native archive locally. Desktop signing/notarization and auto-update are outside this experiment.

Local verification uses `mise run format`, then full `mise run check:pr` and `mise run build:docs` on committed HEAD with clean tracked files. Distribution contracts exercise real product archives, installer rejection/replacement, source metadata and runtime without host build tools. Existing accounting, storage, replay, retained identity boundaries, browser and real SSH/network contracts remain required. Inspect retained validation artifacts; no PR checks run remotely.

Only current database contracts are supported. Collector schema 20 and token schema 1 remain unchanged. Account/application/jobs schemas and their product code are retired; existing user files remain untouched. Installers never migrate, reset or delete usage data. The backend owns local stores while viewers are connected and shuts down after the last viewer's reconnect grace; pending accepted work resumes with the next owner. See [design](design.md).
