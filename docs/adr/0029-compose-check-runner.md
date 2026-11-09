# Docker Compose check runner

Status: Accepted. Date: 9 November 2026. User approved replacing the managed
Dagger engine with Docker Compose projects.

## Context

ADRs 0022 and 0028 ran container groups through one explicitly managed Dagger
engine per repository. Every repository kept a long-lived privileged engine and
cache volume, engine ownership needed manual coordination, and admission
counted heavy/light groups per user rather than whole runs across repositories.

## Decision

This supersedes ADR 0022's Dagger engine ownership and per-group heavy/light
admission, and ADR 0028's Dagger cache mechanics. Committed clean HEAD,
selected inputs, receipts, run retention, mandatory correctness groups, the
format barrier and opt-in benchmarks remain.

`tools/validation/cmd/validate` builds one multi-stage
`tools/validation/Dockerfile`:

- `base`: pinned Debian digest and snapshot apt packages, including optional
  tooling, Xvfb/GTK/WebKit and OpenSSH packages, behind the apt guard.
- `toolchain`: sha-pinned mise (`min_version`) installs Go, Node and pnpm from
  `mise.lock`; `check:toolchain` rejects other image tool pins.
- `deps`: frozen pnpm install, Go modules (baked offline) and Playwright Chromium.
- `web-built`, `go-built`: shared web assets and pure-Go CLI/backend binaries.
- `source`: group files plus a baseline commit before any build output.
- `assets`, `binaries` and per-group variants, plus `keys`, `sshd` and `personal`
  service images for deployment.

Named contexts `ctx-group`, `ctx-deps`, `ctx-web` and `ctx-go` are hardlink
trees materialized from the `groups.json` predicates that also compute group
fingerprints. A fingerprint hashes every tracked entry of every context the
group declares in `uses`; undeclared contexts are empty, so an undeclared use
fails the build. Container inputs and receipt keys cannot diverge. One shared
`deps` context means dependency manifests and Go module files invalidate every
container group.

Each executing group runs Compose project `tokeninsights-check-s<slot>-<group>`
from `tools/validation/compose.yaml`: a runner with 2 CPUs, 6g memory, init and
shared named cache volumes `tokeninsights-check-go-build` and
`tokeninsights-check-golangci-lint`; deployment adds one-shot keys, real OpenSSH
and the personal TCP fixture with healthchecks. The project network is
`internal` with the isolated IPv4 gateway mode (Docker 28+): no egress and no
host address on the bridge. No ports are published; no host network, Docker
socket or host path is mounted.

Images are tagged `tokeninsights-check:s<slot>-<target>`. Output goes to
per-group log files; evidence leaves through `docker cp`. Success, failure and
cancellation all run `docker rm -f` and `compose down -v`, then untag images.

Admission holds one of two slots per run in `~/.cache/local-validation/slots`,
shared by participating repositories, so at most two runs execute machine-wide.
After acquiring a slot the runner removes stale `s<slot>` resources of this
repository only. Status, cache hits and format-only runs never queue. A run
executes up to three groups at once, longest first. A failure lets other
groups finish; only passed groups receive receipts and every failure is
reported. Format stays on the host and still stops the run on failure.

## Consequences

No engine needs setup, ownership checks or coordinated upgrades. BuildKit cache
mounts and stage sharing replace engine caches; they remain accelerators, never
evidence, and the runner never prunes them. Docker's builder garbage collection
bounds them. Worst-case memory is two runs times three 6g runners. Checks that
need network egress fail loudly. `compose.yaml` requires Docker 28+ for the
isolated gateway mode. Existing Dagger engines and volumes are left untouched;
operators remove them manually.

Unresolved questions: none.
