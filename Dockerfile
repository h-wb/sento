# --- web build ---
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- go build ---
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build \
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
