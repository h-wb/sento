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
| `mise run lint:web` | Web lint rules |
| `mise run lint:ci` | Actions YAML and embedded shell scripts |
| `mise run lint:docs` | Local file links in maintained Markdown |
| `mise run gen:check` | Generated TypeScript matches Go API structs |
| `mise run extensions:test` | Kotlin sidecar tests against directory fixtures |
| `mise run extensions:contract` | Built JVM sidecar and Go client contract |
| `mise run test:contract` | Live MangaBaka API contract; requires network access |

`mise run ci` combines the standard lint, generated types, harness, race and
browser checks. GitHub Actions runs PostgreSQL as a separate job and sidecar
contracts in the Sidecar workflow when relevant files change. Live source and
metadata checks are separate because upstream availability varies.

To focus a Go test run, forward flags to the task, for example:

```sh
mise run test -- -run TestName ./internal/service
```

`test:pg` starts a mise-managed PostgreSQL daemon and gives tests its URL.
Tests create isolated schemas. Stop the server when done with
`mise daemons stop postgres`; do not point the suite at a production database.

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
Every migration needs SQLite and PostgreSQL coverage. Sidecar behavior belongs
in Kotlin fixtures and the Go contract tests at the JVM boundary.

Keep fixture results deterministic and credentials fictional. An upstream
outage is a blocked live contract check, not evidence of a local regression.
The doc-link check verifies local file targets in the documentation; it does
not check remote URLs or heading anchors.
