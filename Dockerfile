FROM node:26.12.1-trixie-slim@sha256:36793bcbb160a6a16cede1e62b93f84e26b78a972a8a4c0b879c1c12f22761ec AS web
WORKDIR /src
RUN npm install --global pnpm@12.15.0
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/cli/package.json packages/cli/package.json
COPY packages/web/package.json packages/web/package.json
COPY packages/plugin-pi/package.json packages/plugin-pi/package.json
COPY packages/plugin-opencode/package.json packages/plugin-opencode/package.json
COPY tools/build/package.json tools/build/package.json
RUN HUSKY=0 pnpm install --frozen-lockfile
COPY packages/web/ packages/web/
COPY tools/build/ tools/build/
RUN pnpm run build:web

FROM golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY packages/cli/go.mod packages/cli/go.sum ./
RUN go mod download
COPY packages/cli/ ./
COPY --from=web /src/packages/cli/internal/server/static/ ./internal/server/static/
RUN go build -trimpath -buildvcs=false -o /out/tokeninsights-server ./cmd/tokeninsights-server

FROM debian:trixie-20261020-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 tokeninsights \
    && useradd --uid 10001 --gid 10001 --no-create-home --home-dir /data tokeninsights \
    && install -d -m 0700 -o 10001 -g 10001 /data /run/tokeninsights
COPY --from=build /out/tokeninsights-server /usr/local/bin/tokeninsights-server
RUN /usr/local/bin/tokeninsights-server --version
USER 10001:10001
WORKDIR /data
EXPOSE 8766
ENV TOKENINSIGHTS_ADMIN_SOCKET=/run/tokeninsights/admin.sock
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/usr/local/bin/tokeninsights-server", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/tokeninsights-server"]
