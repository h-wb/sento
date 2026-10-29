# Exploratory testing

The agent-facing harness for self-usability testing: how a Paseo session
runs Sento against real upstreams, explores it like a user, and turns
findings into GitHub issues. Human notes live in
[dogfooding.md](dogfooding.md); this document is the machine's brief.

## The target

Each exploratory run gets a disposable Paseo worktree: `paseo.json` setup
runs `mise install && mise run web:install`, then start the three services
(`paseo script start extensions api web`; `paseo script ls` prints the
ports and the proxy URLs). The stack runs from the worktree's own code —
Go changes rebuild in seconds, the web HMRs — so what you test is the code,
not a built image.

Everything is real. The api service starts the backend under the prod
profile (`MISE_ENV=prod` → `mise.prod.toml`: qBittorrent and Prowlarr URLs,
the `sento-test` category) with credentials fetched from Proton Pass by
fnox for the `run:agent` task only (`fnox.toml`; the worktree setup runs
`mise run pass:login`, idempotent). Seeds apply on a worktree's first run
only: once its DB has settings, env changes are ignored — wipe `data/` to
reseed. You configure nothing by hand except the indexers (below).

Limits — these are hard rules, not preferences:

- **Indexers: Nyaa only.** Add exactly one indexer, Nyaa, in the UI.
  Never enable the rest of Prowlarr's catalog.
- **qBittorrent category: `sento-test`.** Seeded via
  `SENTO_QBITTORRENT_CATEGORY`. Grabs must land under
  `/data/Downloads/qbittorrent/complete/sento-test`. Never the live
  instance's `sento` category.
- **Library: worktree-local.** Leave `SENTO_LIBRARY_PATH` at its default
  (the worktree's `data/`). Imports from `/data` copy instead of
  hardlinking — accepted tradeoff: no worktree can touch another worktree's
  files, or the real library at `/data/Literature/Manga/Sento`.

## Guardrails

- The live instance (`sento.example.invalid`, the `sento` category,
  `/data/Literature/Manga/Sento`) is off-limits: read nothing, cancel
  nothing, grab nothing.
- Cancel stuck grabs from the test instance's queue UI; do not manage
  qBittorrent directly beyond the `sento-test` category.
- The worktree is disposable: exploratory runs report, they never fix.
  Findings, not patches.
- A queue item that fails is a finding to reproduce, not a bug to hide:
  leave the state inspectable and describe it.

## How to explore

Use the web UI like a user would — through the pod's headless Chromium or
the Playwright suite in `web/e2e` (against `cmd/e2estub`) when a flow needs
determinism. Walk the real journeys: search and add a series, browse its
editions/versions/books, grab something from Nyaa, watch the queue, work
the review inbox, check Wanted, change settings. For every hesitation,
ask: what did I expect, what actually happened, what would a first-time
user do here?

Judge by these classes, in order of severity:

1. **Dead ends** — an action leaves state that no offered action can
   resolve (a review item nothing fixes, a stuck queue item).
2. **Wrong labels** — a control that doesn't do what it says (see
   "Keep an additional version", dogfooding.md).
3. **Hidden narratives** — the app knows the full story but shows
   fragments (queue snapshot says one thing, error another, no summary).
4. **Friction** — more steps, retries, or re-reading than the task
   deserves.
5. **Raw internals** — paths, Go errors, or SQL states shown to a user.

## Filing issues

One issue per finding, with the
[exploratory finding](.github/ISSUE_TEMPLATE/exploratory-finding.md)
template, labeled `usability`:

```sh
gh issue create --label usability --title "<one-line finding>" \
  --body-file <filled template>
```

Before filing, read [dogfooding.md](dogfooding.md): if the finding is
already known, do not file a duplicate — file an issue only when you can
add a repro, a screenshot, or a case the entry lacks, and reference the
entry in the issue body. A finding that matches no entry gets a fresh
issue; when triaged, it graduates into the dogfooding log.
