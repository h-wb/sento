# syntax=docker/dockerfile:1

# The sento-host sidecar: the JVM extension runtime Sento talks to for
# Tachiyomi/Mihon sources. Build context is this directory (host/).
#
# The build output is JVM bytecode, identical for every architecture, so
# the build stage runs once on the build platform and only the JRE layer
# differs per target.

# --- build ---
FROM --platform=$BUILDPLATFORM gradle:9.8.0-jdk21 AS build
ARG BUILDARCH
# Keep in step with the vendir pin in mise.toml (Renovate bumps both).
ARG VENDIR_VERSION=0.46.2
RUN curl -fsSL -o /usr/local/bin/vendir \
      "https://github.com/carvel-dev/vendir/releases/download/v${VENDIR_VERSION}/vendir-linux-${BUILDARCH}" \
    && chmod +x /usr/local/bin/vendir
WORKDIR /src
# The vendored Suwayomi tree first, exactly as pinned by the lock file:
# this layer only rebuilds when the pin moves.
COPY vendir.yml vendir.lock.yml ./
RUN vendir sync --locked
COPY settings.gradle.kts build.gradle.kts ./
COPY app/ app/
RUN --mount=type=cache,target=/home/gradle/.gradle/caches \
    gradle --no-daemon :app:installDist

# --- runtime ---
FROM eclipse-temurin:21-jre
COPY --from=build /src/app/build/install/app /opt/sento-host
# Stateless by design: Sento re-installs extensions and replays source
# settings onto a fresh host. The volume only saves that work on restart.
ENV SENTO_HOST_PORT=8901 \
    SENTO_HOST_DATA=/data \
    JAVA_TOOL_OPTIONS=-Djava.awt.headless=true
VOLUME /data
EXPOSE 8901
ENTRYPOINT ["/opt/sento-host/bin/app"]
