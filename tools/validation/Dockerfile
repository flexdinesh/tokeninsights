# Isolated check images. The runner materializes the named build contexts
# (ctx-group, ctx-deps, ctx-web, ctx-go) from the predicates in groups.json that also compute
# each group's fingerprint, so container inputs and receipt keys cannot diverge.
# Shared stages (base, toolchain, deps, web-built, go-built) depend only on the
# shared contexts and are reused by every group and worktree through BuildKit.

FROM debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 AS base
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ENV DEBIAN_FRONTEND=noninteractive
# The apt guard bounds apt under snapshot rate limiting, where it has flooded
# gigabytes of "delayed item" warnings and exhausted host memory. Each attempt
# gets a 4 GiB address space cap and a 10 minute timeout; failures retry with
# backoff, reusing packages already in the cache. It shadows apt-get on PATH,
# including the apt-get that Playwright's --with-deps starts.
COPY --chmod=0755 <<'EOF' /usr/local/bin/apt-get
#!/bin/bash
for wait in 30 90 0; do
  (ulimit -v 4194304; exec timeout 600 /usr/bin/apt-get "$@") 2>&1 | grep --line-buffered -v 'Tried to start delayed item'
  code=${PIPESTATUS[0]}
  [ "$code" -eq 0 ] && exit 0
  [ "$wait" -eq 0 ] && exit "$code"
  echo "apt-get exited $code; retrying in ${wait}s" >&2
  sleep "$wait"
done
EOF
# Packages come from a pinned snapshot. Apt verifies signatures, so plain HTTP
# works before ca-certificates exists. Change the cache ids with the date.
RUN sed -i 's|http://deb.debian.org/debian-security|http://snapshot.debian.org/archive/debian-security/20261031T000000Z|g; s|http://deb.debian.org/debian|http://snapshot.debian.org/archive/debian/20261031T000000Z|g' /etc/apt/sources.list.d/debian.sources \
  && rm -f /etc/apt/apt.conf.d/docker-clean \
  && printf 'Acquire::Check-Valid-Until "false";\nBinary::apt::APT::Keep-Downloaded-Packages "true";\n' > /etc/apt/apt.conf.d/99validation-snapshot
# Optional-group packages live here too so every group shares one base:
# libarchive-tools (tooling), Xvfb/GTK/WebKit (distribution), OpenSSH (deployment).
RUN --mount=type=cache,id=tokeninsights-check-apt-archives-trixie-20261031,target=/var/cache/apt,sharing=locked \
  --mount=type=cache,id=tokeninsights-check-apt-lists-trixie-20261031,target=/var/lib/apt/lists,sharing=locked \
  apt-get update \
  && apt-get install -y --no-install-recommends \
    bash ca-certificates curl gcc git libc6-dev libarchive-tools libgtk-3-0 libwebkit2gtk-4.1-0 \
    openssh-client openssh-server procps x11-utils xauth xvfb xz-utils

FROM base AS toolchain
# check-toolchain.ts keeps this equal to mise.toml min_version. Go, Node and
# pnpm come only from the committed mise.lock checksums.
ENV MISE_VERSION=2026.10.18
RUN curl -fsSL "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-linux-x64.tar.gz" -o /tmp/mise.tar.gz \
  && echo '0d41b60745a6273a80089c86f80d8729042d2f57f87fda70aecc7d1c3ffbd3be  /tmp/mise.tar.gz' | sha256sum -c - \
  && tar xzf /tmp/mise.tar.gz -C /tmp && cp /tmp/mise/bin/mise /usr/local/bin/mise && rm -rf /tmp/mise /tmp/mise.tar.gz
ENV MISE_DATA_DIR=/opt/mise MISE_TRUSTED_CONFIG_PATHS=/src MISE_TASK_RUN_AUTO_INSTALL=false MISE_ALWAYS_KEEP_DOWNLOAD=1 \
  GOTOOLCHAIN=local GOFLAGS="-mod=readonly -buildvcs=false -modcacherw" GOMODCACHE=/opt/gomod \
  GOMAXPROCS=2 TOKENINSIGHTS_CHECK_CONCURRENCY=2 PLAYWRIGHT_BROWSERS_PATH=/opt/playwright \
  HOME=/private/home XDG_CONFIG_HOME=/private/config XDG_DATA_HOME=/private/data XDG_CACHE_HOME=/private/cache \
  TMPDIR=/private/tmp HUSKY=0
RUN mkdir -p /private/home /private/config /private/data /private/cache /private/tmp
WORKDIR /src
COPY --from=ctx-deps mise.toml mise.lock ./
RUN --mount=type=cache,id=tokeninsights-check-mise-downloads,target=/opt/mise/downloads,sharing=locked \
  mise install \
  && for tool in go gofmt node npm npx pnpm; do ln -sf "$(mise which "$tool")" "/usr/local/bin/$tool"; done \
  && go version && node --version && pnpm --version

FROM toolchain AS deps
COPY --from=ctx-deps . /src/
RUN --mount=type=cache,id=tokeninsights-check-pnpm-store,target=/cache/pnpm,sharing=shared \
  pnpm install --frozen-lockfile --store-dir /cache/pnpm --package-import-method copy
# Modules download into a shared cache mount, then copy offline into the layer
# so runners need no network and no module volume.
RUN --mount=type=cache,id=tokeninsights-check-go-mod,target=/cache/gomod,sharing=shared \
  download() { go mod download && go mod download -modfile=golangci-lint.mod && (cd tools/validation && go mod download); } \
  && GOMODCACHE=/cache/gomod download \
  && GOMODCACHE=/opt/gomod GOPROXY=file:///cache/gomod/cache/download GOSUMDB=off download
RUN --mount=type=cache,id=tokeninsights-check-apt-archives-trixie-20261031,target=/var/cache/apt,sharing=locked \
  --mount=type=cache,id=tokeninsights-check-apt-lists-trixie-20261031,target=/var/lib/apt/lists,sharing=locked \
  pnpm --filter @tokeninsights/web exec playwright install --with-deps chromium

FROM deps AS web-built
COPY --from=ctx-web . /src/
RUN node tools/build/src/build-web.ts

# Browser, SSH and TCP use identical pure-Go builds. Distribution intentionally
# uses build-products.ts instead: its release linker metadata is part of coverage.
FROM deps AS go-built
COPY --from=ctx-go . /src/
COPY --from=web-built /src/packages/core/server/static /src/packages/core/server/static
RUN --mount=type=cache,id=tokeninsights-check-go-build,target=/cache/go-build,sharing=shared \
  export GOCACHE=/cache/go-build CGO_ENABLED=0 \
  && go build -o /out/tokeninsights ./packages/cli/cmd/tokeninsights \
  && go build -o /out/tokeninsights-backend ./packages/backend/cmd/tokeninsights-backend

# The baseline commit precedes every build output, so generated.diff shows only
# what checks change.
FROM deps AS source
COPY --from=ctx-group . /src/
RUN git init -q --initial-branch=validation && git add -A \
  && git -c user.name=validation -c user.email=validation@localhost.invalid commit -q --no-verify --no-gpg-sign -m baseline

FROM source AS assets
COPY --from=web-built /src/packages/core/server/static /src/packages/core/server/static

FROM assets AS binaries
COPY --from=go-built /out/ /src/packages/cli/bin/

FROM source AS web
FROM source AS tooling
FROM assets AS static
FROM assets AS go
FROM assets AS benchmarks
FROM assets AS distribution
FROM binaries AS browser

# OpenSSH reads the passwd home (/root), not $HOME. The key pair comes from
# the project-private keys volume at run time.
FROM binaries AS deployment
RUN install -d -m 700 /root/.ssh \
  && printf 'Host sshd\n  IdentityFile /root/.ssh/id\n  IdentitiesOnly yes\n  StrictHostKeyChecking accept-new\n  UserKnownHostsFile /root/.ssh/known_hosts\n  BatchMode yes\n' > /root/.ssh/config \
  && chmod 600 /root/.ssh/config

FROM base AS keys
CMD ["sh", "-c", "test -f /keys/id || ssh-keygen -q -t ed25519 -N '' -C validation -f /keys/id"]

# Real OpenSSH running the CLI built from source, installed on the remote's
# default non-interactive PATH. Host keys are generated per container start.
FROM base AS sshd
RUN useradd --create-home --shell /bin/sh remote \
  && install -d -m 700 -o remote -g remote /home/remote/.ssh \
  && rm -f /etc/ssh/ssh_host_* && mkdir -p /run/sshd
COPY --from=go-built /out/ /opt/tokeninsights/
RUN ln -s /opt/tokeninsights/tokeninsights /usr/local/bin/tokeninsights
CMD ["sh", "-c", "install -m 600 -o remote -g remote /keys/id.pub /home/remote/.ssh/authorized_keys && ssh-keygen -A && exec /usr/sbin/sshd -D -e"]

# This standard-library-only supervisor has an independent build input, so
# fixture changes never invalidate the shared production binaries.
FROM toolchain AS personal-built
COPY --from=ctx-group tools/validation/fixtures/personal/main.go /fixture/main.go
RUN --mount=type=cache,id=tokeninsights-check-go-build,target=/cache/go-build,sharing=shared \
  GOCACHE=/cache/go-build CGO_ENABLED=0 go build -o /out/personal-fixture /fixture/main.go

FROM base AS personal
ENV HOME=/private/home XDG_DATA_HOME=/private/data XDG_CONFIG_HOME=/private/config
RUN mkdir -p /private/home /private/data /private/config
COPY --from=go-built /out/ /opt/tokeninsights/
COPY --from=personal-built /out/personal-fixture /opt/tokeninsights/personal-fixture
CMD ["/opt/tokeninsights/personal-fixture"]
