# Reviewing a Sento pull request

Sento is a Go server with a React web app that finds, downloads and organizes manga.
Most pull requests are authored by agents from GitHub issues; CI (lint, tests, e2e,
PostgreSQL conformance, images) has to pass on its own. Your review covers what CI
cannot check. If this pull request only edits the release-please files (`CHANGELOG.md`,
`.release-please-manifest.json`, version files), only the release rule applies.

## 1. Read the issue and the intent

Open the linked issue (`Refs #N`, `Fixes #N` or the body) and read what was agreed.
The diff should implement that behavior and nothing else: an unexplained extra change,
a renamed concept or a "cleanup" unrelated to the issue is a finding even when correct.
When the issue contradicts the domain docs (`docs/domain/`, `docs/spec/`), report it
rather than picking a side.

## 2. Check the repository's stated rules

From AGENTS.md and CONTRIBUTING.md, verified against the diff:

- **Schema migrations come in pairs**: a new migration file must exist under both
  `internal/db/migrations/sqlite/` and `internal/db/migrations/postgres/` with the
  same number. One dialect alone is blocking.
- **API struct changes ship their types**: a change to a `internal/server` request or
  response struct must include the matching regenerated `web/src/types` files. A
  hand-edited type file is a finding.
- **Behavior changes update the domain docs**: the matching page under `docs/domain/`
  must be updated in the same pull request. Preserve the spec filenames, section
  numbers and decision IDs that code and docs cite; a renumbered section without
  updating the citing code is a finding.
- **Bug fixes carry a regression test** that fails without the fix and asserts the
  behavior described in the issue, not the implementation's shape. A test that merely
  restates the code, or assertions weakened to go green, is a finding.

## 3. Look past the diff

- Read the changed service code around the hunks: acquisition, review and cleanup
  flows have protection rules (pins, verified mappings, recoverable trash, preserved
  client payloads). A change that bypasses one of those guards is blocking, even when
  the tests still pass.
- Destructive paths must stay recoverable: deletions go through trash with retention,
  imports preserve original files on failure. A new hard delete is blocking.
- Error classification must stay honest: a new "cannot happen" fallback that hides a
  real error, or a swallowed error return, is a finding.
- No credentials, tokens, private tracker URLs or library paths in code, tests, docs
  or fixtures. The repository is public.
- Third-party code keeps its notices; original code is GPL-3.0-only (see LICENSE).

Report as **blocking** anything that contradicts the domain docs or spec, a missing
migration dialect, generated-types drift, or a removed protection. Report as
**important** a missing doc update, a weak or absent regression test for a bug fix,
or a scope deviation. If the linked issue is missing or does not match the change,
say so instead of reconstructing an intent.
