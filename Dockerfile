# syntax=docker/dockerfile:1

# Both build stages are pinned to the *build* platform and cross-compile to
# the target: the Go binary is CGO-free, so a multi-arch image costs one
# native web build plus one `go build` per arch, instead of running npm and
# the compiler under QEMU.

# --- web build (arch-independent output) ---
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
RUN npm run build

# --- go build ---
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
ARG COMMIT=unknown
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags "-s -w -X github.com/h-wb/sento/internal/version.Version=${VERSION} -X github.com/h-wb/sento/internal/version.Commit=${COMMIT}" \
    -o /sento ./cmd/sento

# --- runtime ---
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /sento /usr/local/bin/sento
COPY --from=web /src/web/dist /app/web/dist
COPY LICENSE /usr/share/licenses/sento/COPYING
LABEL org.opencontainers.image.licenses="GPL-3.0-only"
WORKDIR /app
# Every path the server writes to points at a declared volume. Without the
# library and downloads lines the defaults are relative to /app: imports
# would land in the container's writable layer — invisible on the mounted
# /library and gone with the container.
ENV SENTO_DB_PATH=/config/sento.db \
    SENTO_LIBRARY_PATH=/library \
    SENTO_DOWNLOADS_DIR=/downloads \
    SENTO_STATIC_DIR=/app/web/dist
VOLUME /config /library /downloads
EXPOSE 8484
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO /dev/null "http://127.0.0.1:${SENTO_PORT:-8484}/api/v1/system/health" || exit 1
ENTRYPOINT ["sento", "-config", "/config/config.yml"]
