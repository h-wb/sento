# Releases

Sento follows [semantic versioning](https://semver.org/): `MAJOR.MINOR.PATCH`.
While the project is pre-1.0 (`0.x.y`), minor bumps may carry breaking
changes; patches are fixes only.

## What a release publishes

Pushing a tag `vX.Y.Z` triggers everything:

- **GitHub Release** ([`release.yml`](../.github/workflows/release.yml)):
  server binaries (`linux/amd64`, `linux/arm64`, `darwin/amd64`,
  `darwin/arm64`), each as a `tar.gz` containing the `sento` binary and the
  prebuilt web UI (`web-dist/` — serve it with `SENTO_STATIC_DIR`), plus a
  `checksums.txt`. Notes are generated from the commit history between tags.
- **Container images** ([`image.yml`](../.github/workflows/image.yml)):
  `ghcr.io/h-wb/sento:X.Y.Z` and `ghcr.io/h-wb/sento-extensions:X.Y.Z`,
  multi-arch, also tagged `X` and `latest` (server only). The image tags are
  `X.Y.Z` without the `v` prefix, matching `docker/metadata-action`'s semver
  handling.

## Cutting a release

```sh
git tag -a v0.1.0 -m "First public release"
git push origin v0.1.0
```

That's the whole process: both workflows do the rest. Tag a commit
that is on `main` and has passed CI — the release workflow verifies the tag
but does not re-run the test suite.

The version shown in the UI and logs comes from the tag (`internal/version`,
injected at link time). Development builds are stamped by `mise run build`
with `git describe` output instead (`v0.1.0-3-gabcdef` or a bare commit).

## What counts as a version bump

- **PATCH** — bug fixes, parser/test changes, no new user-facing behavior.
- **MINOR** — new features, settings, API surface; anything a user notices.
- **MAJOR** — reserved for 1.0 and breaking config/library/DB migrations.
