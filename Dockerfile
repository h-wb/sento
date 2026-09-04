# Both build stages are pinned to the *build* platform and cross-compile to
# the target: the Go binary is CGO-free, so a multi-arch image costs one
# native web build plus one `go build` per arch, instead of running npm and
# the compiler under QEMU.

# --- web build (arch-independent output) ---
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- go build ---
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
ARG COMMIT=unknown
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X github.com/h-wb/sento/internal/version.Version=${VERSION} -X github.com/h-wb/sento/internal/version.Commit=${COMMIT}" \
    -o /sento ./cmd/sento

# --- runtime ---
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /sento /usr/local/bin/sento
COPY --from=web /src/web/dist /app/web/dist
WORKDIR /app
ENV SENTO_DB_PATH=/config/sento.db \
    SENTO_STATIC_DIR=/app/web/dist
VOLUME /config /library /downloads
EXPOSE 8484
ENTRYPOINT ["sento", "-config", "/config/config.yml"]
