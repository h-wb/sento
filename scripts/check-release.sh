#!/usr/bin/env bash
# Both publication workflows call this before building a tagged release.
set -euo pipefail
: "${SENTO_RELEASE_TAG:?Provide the release tag}"
if [[ ! "$SENTO_RELEASE_TAG" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "Expected a stable vMAJOR.MINOR.PATCH tag, got $SENTO_RELEASE_TAG" >&2
  exit 1
fi
sha=$(git rev-parse HEAD)
git fetch origin main
git merge-base --is-ancestor "$sha" origin/main || {
  echo 'Releases must point to a commit on main.' >&2
  exit 1
}
passed=$(gh run list --repo "$GITHUB_REPOSITORY" --workflow ci.yml --event push \
  --commit "$sha" --limit 100 --json status,conclusion \
  --jq 'any(.[]; .status == "completed" and .conclusion == "success")')
if [[ "$passed" != true ]]; then
  echo "No successful main CI run for $sha. Wait for CI and rerun this workflow." >&2
  exit 1
fi
