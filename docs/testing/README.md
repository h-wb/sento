# Testing

Use `mise run <task>` so local checks use the same toolchains as CI.
The standard setup is `mise install && mise run web:install`. Downloading
tools, dependencies and Chromium needs network access; installing Chromium
system libraries also needs an administrator on Linux. The Go and browser
regression tests use local fixtures after that setup.

## Choose a check

| Task | What it checks |
| --- | --- |
| `mise run test` | Go unit, service and store tests using SQLite |
| `mise run test:race` | Go suite with the race detector; requires a C compiler |
| `mise run test:pg` | Go suite against a disposable PostgreSQL database |
| `mise run test:harness` | Offline upstream simulator through real protocol clients |
| `mise run test:tooling` | Repository workflow helpers |
| `mise run e2e` | Chromium, real Go backend and simulated upstreams |
| `mise run lint` | Go formatting and static checks |
| `mise run lint:web` | Web lint rules (ESLint only, see below) |
| `mise run lint:ci` | Actions YAML and embedded shell scripts |
| `mise run lint:docs` | Local file links in maintained Markdown |
| `mise run gen:check` | Generated TypeScript matches Go API structs |
| `mise run extensions:test` | Kotlin sidecar tests against directory fixtures |
| `mise run extensions:contract` | Built JVM sidecar and Go client contract |
| `mise run test:contract` | Live MangaBaka API contract; requires network access |

`mise run lint:web` runs only ESLint (`npm run lint` in `web/`); it does not
type-check. The type check runs as part of `mise run web:build`
(`tsc -b && vite build`, the `build` script in `web/package.json`), so run
`web:build` before pushing web changes.

`mise run ci` combines the standard lint, generated types, harness, race and
browser checks. GitHub Actions runs PostgreSQL as a separate job and sidecar
contracts in the Sidecar workflow when relevant files change. Live source and
metadata checks are separate because upstream availability varies.

To focus a Go test run, forward flags to the task, for example:

```sh
mise run test -- -run TestName ./internal/service
```

`test:pg` starts a mise-managed PostgreSQL daemon (version 17, see
`[daemons.postgres]` in `mise.toml`) and gives tests its URL through
`SENTO_TEST_DB_URL`. Stop the server when done with
`mise daemons stop postgres`; do not point the suite at a production database.

## PostgreSQL

`internal/db/dbtest` opens a throwaway database per test. Without
`SENTO_TEST_DB_URL` that is a SQLite file in a temp dir. With it, each test
creates its own schema on the shared server (`sento_test_<pid>_<random>`),
scopes the connection to it with `options=-csearch_path=<schema>`, runs the
full PostgreSQL migration set there, and drops the schema with `CASCADE` on
cleanup. The same Go suite therefore doubles as the dialect conformance
run: `db.Open` migrates whatever schema its URL points at. PostgreSQL 16 or
later is required (migration 0030 uses `IS JSON OBJECT`); mise pins 17.

The data-migration tests for 0014, 0024 and 0034
(`internal/db/migration_test.go`) replay the SQLite files only, by hand.
The tests for 0027, 0028 and 0030 (`store/settle_migration_test.go`,
`archive_range_migration_test.go`, `blocklist_migration_test.go`) and the
other store suites go through `dbtest`, so under `test:pg` they run on both
dialects. Write new migration tests with `dbtest` for the same reason.

## Browser tests

Install Chromium once, then run:

```sh
mise run e2e:install
mise run e2e
```

In the unprivileged Paseo agent image, use the hidden `mise run e2e:paseo`
task with its shared browser dependency cache. See the
[Paseo environment reference](paseo.md) for the missing-library workaround
and setup assumptions. This does not change ordinary development or CI.

Playwright starts `bin/sento` and `bin/e2estub`, built by the task, with the
real UI and temporary SQLite/library state under `web/.playwright/`. The
simulator stands in for MangaBaka, the extension runtime, Prowlarr, Torznab
and qBittorrent. It returns tiny real images for CBZ imports. Nothing is
sent to real download clients.

The suite uses one worker because tests share mutable application state.
Each invocation clears its run directory and starts new upstreams; it never
reuses an already-running server. Ports are derived from the checkout path,
so worktrees can test alongside one another and the dev stack. If necessary,
override `SENTO_E2E_PORT` and `SENTO_E2E_STUB_PORT`. Run one suite per checkout
at a time. Local database URLs, path mappings and extension repository
overrides are cleared before starting the backend.

Playwright records traces and screenshots on failure. In CI it uploads the
HTML report; locally, check `web/test-results/`. Use `mise run e2e:ui` for an
interactive run. Add test files under `web/e2e/`; inject failures at the
upstream boundary, rather than replacing Sento's API responses.

For manual exploration, `mise run explore` starts an independent supervised
stack with the same controllable upstreams. See [the exploratory guide](../exploratory-testing.md)
and [scenario reference](exploratory-scenarios.md).

## Regression tests

Put parser cases in `internal/parse/testdata/corpus/`. Use service/store tests
for edition matching, book coverage, migrations and acquisition decisions;
use browser tests when the user interaction or persisted UI state matters.
Every schema migration needs a matching numbered file in both dialect
directories, and its test should use `dbtest` so the migration runs on both
(see [PostgreSQL](#postgresql)). Sidecar behavior belongs
in Kotlin fixtures and the Go contract tests at the JVM boundary.

Keep fixture results deterministic and credentials fictional. An upstream
outage is a blocked live contract check, not evidence of a local regression.
The doc-link check verifies local file targets in the documentation; it does
not check remote URLs or heading anchors.
