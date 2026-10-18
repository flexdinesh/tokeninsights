FROM node:26.4.0-trixie-slim@sha256:a1d9d671994fc2d26e297ac56b4b1522a8bc7fa71c43b14cd1b1fe6c5116f7dc AS web
WORKDIR /src
RUN npm install --global pnpm@11.6.0
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

FROM golang:1.26.8-trixie@sha256:aae9c439b447d24c93ee483408992857cf39a3fb6ab7fe53affc65d834891fa8 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY packages/cli/go.mod packages/cli/go.sum ./
RUN go mod download
COPY packages/cli/ ./
COPY --from=web /src/packages/cli/internal/server/static/ ./internal/server/static/
RUN go build -trimpath -buildvcs=false -o /out/tokeninsights-server ./cmd/tokeninsights-server

FROM debian:trixie-20261005-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f
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
