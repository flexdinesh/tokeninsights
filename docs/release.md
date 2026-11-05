# Releases and installation

Install prebuilt products from GitHub Releases. No Go, Node, npm, pnpm or repository checkout is required on the user's machine.

## Install

CLI, TUI, browser launcher and their backend, latest stable:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh
```

Latest main build:

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-main.sh | sh
```

Desktop (includes CLI/backend):

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-desktop.sh | sh
```

Product scripts default to stable. Pass `sh -s -- --channel=main` to choose main. Rerun to upgrade. Stable/main replace the selected product. Installers verify a unique SHA-256 checksum, product identity and archive layout, then stage replacement in user-owned storage. An incomplete download or checksum failure leaves the installed product intact. `TOKENINSIGHTS_DOWNLOAD_URL` overrides the download base for local release fixtures.

Product files live under `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/products/`. Command links use `~/.local/bin`; macOS apps use `~/Applications`. Add `~/.local/bin` to PATH if needed. Installation starts no background service. Each client product contains its own backend; desktop installation also exposes the CLI. The most recent CLI or desktop install owns `tokeninsights`, and both warn when PATH runs another command. Removing another client cannot remove the selected product's backend dependency. Close viewers before upgrading an incompatible local-client version. Installers never terminate running viewers.

Homebrew and versioned `go install` delivery are no longer supported. Historical releases/tags remain downloadable. Building source remains a development workflow through mise.

## Publish

Stable: dispatch **Release** from main with an explicit semantic version. The workflow builds the selected source, creates an immutable root `vX.Y.Z` source tag, stages complete archives/checksums in a draft, then publishes the stable release. Reruns must retain the same source identity; published stable bytes are not overwritten.

Main: each push to main builds and publishes the rolling `main-build` prerelease. CI runs no checks; validate locally first and test other platforms by hand from this channel. It omits macOS Intel, whose archives first build during a stable release and cannot be hand-tested beforehand; installers reject that host on main. It is never marked Latest. Failed or obsolete builds cannot replace a newer successful build. A rolling release is not an atomic download snapshot: an installer racing publication may fail checksum verification and can be rerun; its previous installation remains intact.

All products share release version/source metadata. Archives are named `tokeninsights-<cli|desktop>_<darwin|linux>_<amd64|arm64>.tar.gz`, with `checksums.txt`; main has no `darwin_amd64` archives. CLI and desktop archives contain the required Go backend. There is no hosted server product or server installer.

No compiled executables, desktop bundles, archives or generated dashboard assets are committed to any branch or tag. Source tags point to source commits. Build tooling generates embedded browser assets before building Go; release attachments contain compiled output. No generated asset commits, edge branch, tap token or tap PR automation remain. See [ADR 0027](adr/0027-personal-backend-and-explicit-connections.md).

## Runtime and verification

Backend, CLI and desktop use Go with `CGO_ENABLED=0`. The backend embeds browser assets and never invoke JavaScript tools. Each channel's archives are built on matching native runners. Desktop signing/notarization and auto-update are outside this experiment.

Local verification uses `mise run format`, then full `mise run check:pr` and `mise run build:docs` on committed HEAD with clean tracked files. Distribution contracts exercise real product archives, installer rejection/replacement, source metadata and runtime without host build tools. Existing accounting, storage, replay, retained identity boundaries, browser and real SSH/network contracts remain required. Inspect retained validation artifacts; no PR checks run remotely.

Only current database contracts are supported. Collector schema 20 and token schema 1 remain unchanged. Account/application/jobs schemas and their product code are retired; existing user files remain untouched. Installers never migrate, reset or delete usage data. The backend owns local stores while viewers are connected and shuts down after the last viewer's reconnect grace; pending accepted work resumes with the next owner. See [design](design.md).
