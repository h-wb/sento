# Exploratory testing

The agent-facing harness for self-usability testing: how a Paseo session
runs Sento against real upstreams, explores it like a user, and turns
findings into GitHub issues. Human notes live in
[dogfooding.md](dogfooding.md); this document is the machine's brief.

## Kickoff

A pass is one disposable worktree session. From the pod:

```sh
paseo run --provider mistral-vibe --new-workspace worktree \
  --title "Sento exploratory pass" \
  "Run the exploratory-test pass: load the exploratory-test skill (or read docs/exploratory-testing.md) and execute it as your mission brief."
```

In a Vibe session the `exploratory-test` skill (`.vibe/skills/`) routes to
this document; other providers get the path directly.

## The target

Each exploratory run gets a disposable Paseo worktree: `paseo.json` setup
runs `mise install && mise run web:install`, then start the three services
(`paseo script start extensions api web`; `paseo script ls` prints the
ports and the proxy URLs). The stack runs from the worktree's own code —
Go changes rebuild in seconds, the web HMRs — so what you test is the code,
not a built image.

Everything is real. The api service (`paseo.json`) starts the backend with
the upstream env on its command line — cluster-local qBittorrent and
Prowlarr URLs, the `sento-test` category — with credentials fetched from
Proton Pass by fnox for the `run:agent` task only (`fnox.toml`; the
worktree setup runs `mise run pass:login`, idempotent). Seeds apply on a
worktree's first run only: once its DB has settings, env changes are
ignored — wipe `data/` to reseed. You configure nothing by hand except
the Nyaa indexer and the Weeb Central source (below).

Limits — these are hard rules, not preferences:

- **Indexers: Nyaa only.** Add exactly one indexer, Nyaa, in the UI.
  Never enable the rest of Prowlarr's catalog.
- **Extension sources: Weeb Central only.** The sidecar ships with no
  sources: add the repo
  `https://github.com/keiyoushi/extensions/raw/repo/index.pb` in
  Connections, install `WeebCentral` from the Indexers page, and use it
  for the chapter-source flows — search, grab, CBZ assembly — staging in
  the worktree's own downloads dir. Never add another source.
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
- The worktree is disposable: exploratory runs report, they don't rework.
  Findings are the deliverable. A small, obvious fix may ride along as a
  pull request — a branch in your worktree, never a direct push to main,
  opened through the scoped token (below).
- A queue item that fails is a finding to reproduce, not a bug to hide:
  leave the state inspectable and describe it.

## The user you are

Explore as someone who has run Sonarr or Radarr for years but has never
seen Sento. The *arr instincts are the quality bar: every place Sento
deviates is either a finding or a delight worth reporting. This user
expects to:

- **Add a series the way they added a TV show**: search a title, tell the
  right entry from lookalikes (cover, year, type, status), monitor it,
  done — without reading any docs.
- **See "what am I missing" at a glance**: Wanted and the series page
  must answer it the way Sonarr's episode table does, including the
  difference between *missing* and *not out yet*.
- **Trust automatic grabs, and audit them**: manual search must show
  *why* a release was rejected, as plainly as Sonarr's rejection reasons.
- **Read the queue like a user, not a log**: every state means an action
  they can take; every failure says what to do next.
- **Never lose a file**: the review flow explains a mismatch and always
  offers a way out, like Sonarr's interactive import.
- **Get a library Kavita can serve**: predictable paths, sane naming,
  ComicInfo intact — files that a manga server indexes without handholding.
- **Configure once**: connections self-test, settings persist, and errors
  name the field that's wrong.

## The corpus

Add these (real MangaBaka entries; the worktree DB is disposable). Each
stresses a different shape — that's the point of the list:

- **Nausicaä of the Valley of the Wind** — the boxset quirk: the catalog
  models the deluxe box as unit 0, torrents carry 0/1/2. The known case
  (see dogfooding.md); sharpen it, don't rediscover it.
- **One Piece** — 100+ volumes, chapter-centric, ongoing: latest-first
  expectations, very long lists, cutoffs.
- **Berserk** (+ note the **Berserk of Gluttony** collision) — long
  series with multiple physical editions: edition disambiguation and
  near-duplicate search results.
- **Hunter×Hunter** — on hiatus: nothing new for months; Wanted and the
  calendar must handle "quiet" without looking broken.
- **Dragon Ball** — completed classic: complete-pack torrents, cutoff
  behavior, and the "nothing left to do" states.
- **A one-volume one-shot** — the smallest edge: one book, complete,
  nothing to monitor.
- **Something you can only get via Weeb Central** — the chapter-source
  path end to end: search, grab, CBZ assembly, import.
- **Whatever search hands you that looks wrong** — the corpus is also
  bait: odd catalog entries are findings about the search experience,
  not detours from it.

## How to explore

Use the web UI like a user would — through the pod's headless Chromium or
the Playwright suite in `web/e2e` (against `cmd/e2estub`) when a flow needs
determinism. Work the corpus above and judge every screen against the
expectations above. For every hesitation, ask: what did I expect (the
charter), what actually happened, what would a first-time *arr user do
here? Report delights too — surprising things that work are worth as much
as friction.

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

## Filing issues (and PRs)

One issue per finding, with the
[exploratory finding](.github/ISSUE_TEMPLATE/exploratory-finding.md)
template, labeled `usability`. Every GitHub write goes through the scoped PAT — `GH_TOKEN` from Proton
Pass (fnox.toml), contents + pull requests + issues read/write on this
repo only — not the pod's broader token; gh reads `GH_TOKEN` first, and
the granted value shadows the pod env for granted commands:

```sh
mise x --secrets GH_TOKEN -- gh issue create --label usability \
  --title "<one-line finding>" --body-file <filled template>
mise x --secrets GH_TOKEN -- gh pr create ...
```

Before filing, read [dogfooding.md](dogfooding.md): if the finding is
already known, do not file a duplicate — file an issue only when you can
add a repro, a screenshot, or a case the entry lacks, and reference the
entry in the issue body. A finding that matches no entry gets a fresh
issue; when triaged, it graduates into the dogfooding log.
