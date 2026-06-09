# Sento — Specification (v1 draft)

Sonarr/Radarr-style automation for manga: monitor series, find releases on
torrent indexers and Tachiyomi/Mihon sources, download, rename, organize into a
clean library for readers like Komga/Kavita.

> Status: working draft. Settled decisions are in §1. Defaults picked
> without deep discussion are tagged `[default]` — challenge freely.
> Deferred work is collected in §13 (post-v1 roadmap).

---

## 1. Settled decisions (log)

| # | Decision | Choice |
|---|----------|--------|
| D1 | Metadata source of truth | MangaBaka only (series / collections / works) |
| D2 | Library entity | **Series + monitored editions**: entry = MangaBaka series; user monitors ≥1 collections (edition × language × medium) under it |
| D3 | Extension engine | **Stock Suwayomi as headless sidecar**, accessed via its GraphQL + HTTP API, hidden behind a `SourceProvider` interface |
| D4 | Tracking model | **Dual ledgers of numbered slots**: volume slots (from MangaBaka works, per monitored collection) + chapter slots (`1..total_chapters`). Both independently monitorable |
| D5 | Vol↔chapter mapping | **None in v1.** No auto-supersede, no auto-delete. Schema stays mapping-ready (files record parsed vol/ch ranges) |
| D6 | Chapter monitoring | Cutoff-based: "monitor chapters from N onward", default N = latest at add time |
| D7 | Chapter slot bound | `max(MangaBaka total_chapters, highest chapter seen on bound sources)` |
| D8 | Slot numbering | Integer slots in v1; non-integer chapters (110.5, omake) ignored by default |
| D9 | Stack | Go backend (single binary), TypeScript web UI (SPA served by the binary) |
| D10 | Chapter language | Single language per series |
| D11 | Extras/specials | Follow MangaBaka's model: volume-side extras = `count_extra` works (exposed, default unmonitored); non-integer chapters stay out of scope (MangaBaka doesn't model them) |
| D12 | Pack auto-grab | Ship conservative thresholds + manual confirm; tune with real-world use |
| D13 | MangaBaka volatility | MangaBaka (esp. collections/works) is WIP and may change: Sento isolates it behind a `MetadataProvider` anti-corruption layer, owns its identity model, caches raw payloads, and degrades gracefully (§5.1) |
| D14 | Existing-library import | First-class, Sonarr-style: scan root folders, match series, link files to slots **in place** — non-destructive by default, renaming/organizing only as an explicit separate action (§11.4) |

## 2. Goals / non-goals

**Goals**

- "Add series → it stays complete forever" automation, volume-first.
- Adopt an existing library without re-downloading anything (§11.4).
- First-class support for *both* official volumes (torrents, high quality) and
  weekly chapters (Tachiyomi sources), per series, at the same time.
- Library output directly consumable by Komga/Kavita (CBZ + ComicInfo.xml +
  sane naming).
- Solid, regression-tested release-name parser.
- Self-hosted, docker-compose friendly, one Go binary + sidecars.

**Non-goals (v1)**

- Not a reader, no read-progress tracking (Komga/Kavita's job).
- No metadata editing/scraping beyond MangaBaka + per-series overrides.
- No anime, light novels OK only if they happen to work (type comes from
  MangaBaka; we don't special-case them).
- No multi-user/permissions (single admin, API key auth). `[default]`
- No automatic upgrade/replace logic (quality upgrades, volume-supersedes-
  chapters) — requires mapping and/or quality ladder maturity; post-v1.

## 3. System architecture

```
                 ┌────────────────────────────────────────────┐
                 │                Sento (Go)                   │
                 │  REST API + TS SPA │ scheduler │ SQLite     │
                 │  metadata svc │ decision engine │ importer  │
                 └──┬──────────┬───────────┬─────────┬────────┘
                    │          │           │         │
        MangaBaka API/DB   Prowlarr    Suwayomi   qBittorrent
        (metadata)        (torrent     (Tachiyomi  (torrent DL)
                           indexers)    sources:
                                        search+DL)
```

- **Sento core (Go)**: owns all state and logic. Everything else is a
  replaceable peripheral spoken to over HTTP.
- **Suwayomi sidecar**: unmodified Suwayomi-Server. Used as *both* an indexer
  (manga search, chapter lists) and a download client (chapter download →
  CBZ pulled over HTTP). Never used as a library/reader.
- **Prowlarr**: torrent/usenet indexer aggregation, same role as in Sonarr.
- **qBittorrent**: v1's torrent client `[default]` (client interface kept
  small so others can follow).
- **FlareSolverr**: optional, configured inside Suwayomi (not Sento's concern).

Deployment: docker-compose file shipping sento + suwayomi; prowlarr/qbit
optional externals. Sento also runs bare (single binary, SQLite).

## 4. Domain model

### 4.1 Entities

```
Series                      — mirror of MangaBaka series (id, titles, status,
                              total_chapters, relationships, …) + Sento state
MonitoredCollection         — a MangaBaka collection the user monitors for a
                              series (edition × language × medium × publisher)
VolumeSlot   (derived)      — one MangaBaka work in a monitored collection:
                              sequence number, ISBN, release date, monitored?
ChapterLedger (per series)  — chapter range config: cutoff, upper bound,
                              language, monitored?
ChapterSlot  (derived)      — integer n in [cutoff..upperBound]
File                        — an imported file: path, size, hash, parsed
                              {volume range | chapter range}, source, group,
                              link to slots it fills
SourceBinding               — series ↔ (Suwayomi source id, source manga id),
                              with state (active/broken) and priority
Grab / QueueItem            — an in-flight download (torrent or suwayomi),
                              with the slots it is expected to fill
```

Slots are *derived*, not rows: volume slots materialize from cached works;
chapter slots from the ledger range. Only deviations are stored (per-slot
unmonitored flag, slot↔file links).

### 4.2 Ledger semantics

- **Volume ledger** (per monitored collection): slot for every `count_type =
  main` work. Default monitored: all, incl. future works discovered on
  metadata refresh. `count_extra/other` works exposed but unmonitored
  (monitorable per slot) — extras exist exactly where MangaBaka models them
  (D11).
- **Chapter ledger** (per series, *not* per collection — scanlation chapters
  don't belong to a publisher edition): slots `cutoff..upperBound`,
  upperBound per D7. One language per series for chapter searching
  (default: instance language, e.g. `en`) (D10).
- A slot is **satisfied** when ≥1 file is linked to it. A monitored,
  unsatisfied, released slot is **wanted**.
- "Released": volume slots use the work's `release_date` (if absent: released
  iff sequence ≤ max sequence with a date `[default]`); chapter slots are
  always considered released (their existence implies it).
- Completion display: volumes `owned/total` per collection; chapters
  `owned/(upperBound − cutoff + 1)` plus an "all-time" bar against
  `total_chapters` for context.

### 4.3 Files standing alone

Imports that match the series but no monitored slot (e.g. chapter below
cutoff, volume of an unmonitored edition) are still imported and linked as
**unrequested files** — visible, never auto-deleted. `[default]`

## 5. Metadata service (MangaBaka)

### 5.1 Volatility & anti-corruption layer (D13)

MangaBaka is actively WIP — the collections/works endpoints are undocumented
and their shapes/IDs may change. Sento treats the *series* endpoint as
semi-stable and everything edition-related as **unstable surface**:

- **`MetadataProvider` interface**: all MangaBaka knowledge lives in one Go
  package (`metadata/mangabaka`). The rest of Sento only sees Sento domain
  types (`Series`, `Edition`, `VolumeWork`); no MangaBaka field names or
  response shapes leak past the adapter.
- **Sento owns identity.** Library entries, monitored collections, files and
  slot links all key on Sento's own IDs. MangaBaka IDs are stored as
  *external references* that can be re-resolved.
- **Collection identity = descriptor, not UUID.** A monitored collection is
  identified by its descriptor tuple `(edition name, language, publisher,
  medium, type)`. The MangaBaka collection UUID is cached for API calls but
  treated as a re-resolvable cache key: on refresh, collections are matched
  by descriptor first; a changed UUID is silently re-bound, a vanished
  descriptor raises a health warning (never destructive — slots and files
  stay).
- **Raw payload cache.** Every series/collections/works response is stored
  as raw JSON with a schema-version tag. If MangaBaka changes shape, a new
  adapter version re-derives our columns from cached payloads — no data loss,
  no full re-crawl.
- **Drift detection.** Contract tests run against recorded fixtures; a
  scheduled live smoke-check (one known series) compares actual responses to
  the expected shape and surfaces "metadata provider drift" as a health
  warning instead of crashing sync.
- **Degraded mode.** If collections/works become unavailable or unparseable,
  the volume ledger falls back to count-based slots `1..final_volume` from
  the documented series endpoint (no dates/ISBNs, single implicit edition);
  the chapter ledger is unaffected (`total_chapters` + source-seen bound).
  Existing slot↔file links are preserved through any fallback or recovery.

### 5.2 API usage

- **API mode** (v1 default): `api.mangabaka.org/v1`, honoring rate limits
  (search 30/min, lookup 120/min). Endpoints used:
  - `GET /v1/series/search?q=` — add-series flow
  - `GET /v1/series/{id}` — refresh
  - `GET /v1/series/{id}/collections` — editions (undocumented but live)
  - `GET /v1/series/{id}/works?page=` — volume slots (paginated, 25/page)
- **Database mode** (v1.x): weekly DB dump mirrored locally for search and
  refresh without rate limits. Schema kept compatible from day one (we store
  raw MangaBaka JSON per series alongside extracted columns).
- Refresh cadence `[default]`: releasing series every 24h; completed/cancelled
  every 7d; manual refresh button. Works re-paged only when collection
  `count_main` or `updated_at` changes.
- `merged_with` handling: if a series was merged, follow the pointer,
  remap the library entry, notify.
- Per-series overrides: title, language, cutoff, etc. stored separately and
  reapplied after refresh (never clobbered).

## 6. SourceProvider interface (Suwayomi adapter)

The only part of Sento that knows Suwayomi exists. **Full component spec,
live-validated against a real instance: [`suwayomi-adapter.md`](suwayomi-adapter.md).**

Validated flow (no Suwayomi library membership needed at any step):
`fetchSourceManga(SEARCH)` → `fetchChapters` (stable chapter ids; list =
transient release candidates) → `enqueueChapterDownload` (self-processing;
completion signal = `chapter.isDownloaded`, *not* the queue) →
`GET /api/v1/chapter/{id}/download` (CBZ stream incl. Suwayomi's own
ComicInfo.xml, merged per §11.3) → `deleteDownloadedChapter` after import.
Suwayomi configured with `downloadAsCbz = true`.

Key semantics:

- **Binding**: adding/searching a series resolves it per source: search by
  title + aliases, score candidates (title similarity, language), require
  manual confirm below a threshold `[default: auto-bind ≥ 0.9 similarity]`.
  Bindings are persisted and re-validated on failure.
- **Chapter lists are transient release candidates**, never ledger state:
  fetched, parsed, matched against wanted slots, then discarded.
- Extension install/update management is *out of scope for Sento v1*: user
  manages extensions in Suwayomi's own WebUI. Sento surfaces only "source
  list + health". Roadmap: v2 manages extensions from Sento's UI via
  Suwayomi GraphQL; long-term we may ship a slim replacement engine — the
  `SourceProvider` interface is the contract that makes both possible.
- Suwayomi auth (if enabled) via basic auth config. Multiple Suwayomi
  instances: not in v1.

## 7. Indexers & download clients (torrents)

- **Prowlarr** integration identical in spirit to Sonarr's: Sento queries
  Prowlarr's search API (`/api/v1/search?query=&cat=`) with manga categories;
  Prowlarr handles the per-indexer adapters.
- Interactive search (UI button per slot/series) + automatic search for
  wanted slots (see §10).
- RSS sync `[v1.x]`: poll Prowlarr recent releases, match against wanted —
  needed at scale, not for first release.
- **qBittorrent** client: add by URL/magnet with category `sento`, poll
  completion, then import. Seeding respected: import = **hardlink** (fallback
  copy), never move, until ratio rules (qBit's own) allow cleanup.
- Suwayomi downloads need no seeding: import = move (or pull-over-HTTP then
  delete from Suwayomi). `[default: HTTP pull, keeps Suwayomi stateless for
  us; shared-volume hardlink mode as a config optimization later]`

## 8. Release parser

The make-or-break component. **Full component spec: [`parser.md`](parser.md).**

Summary of the contract:

- Pure two-layer design: context-free `Parse(name, channel)` → structured
  facts (title guess, volume/chapter ranges, group, edition/quality/content-
  type hints, season, year), then a `match` resolver applying context
  (library title index, ledger bounds, channel priors, monitored-collection
  descriptors) via ordered, traced rules R1–R7.
- Anchored mode (known series → title stripped first) for all source-channel
  and targeted-search parsing; blind mode only for manual import.
- Content-type gate: `(Novel)`/`[Audiobook]`/`(Artbook)` releases of the
  same title are hard-rejected.
- Explainability: every result carries its token/rule trace, surfaced in
  logs and the manual-import UI.
- Golden **regression corpus** (JSONL, partial expectations, CI-gated at
  100%, append-only) seeded with real nyaa/source names; round-trip property
  test guarantees Sento always parses its own naming templates; Go fuzzing
  for robustness.

## 9. Decision engine

**Full component spec: [`decision-engine.md`](decision-engine.md).**

Summary: per decision request (series × ledger × slot set), candidates from
*both* protocols (Prowlarr torrents and source chapter lists — eligibility
by what the name parses as, not the channel) run through the resolver, then
ordered hard rejections (content-type, coverage, edition routing, pack rule
D12, size, seeders, blocklist, term rules), then a transparent weighted
score (coverage-dominant: +1000/wanted slot; protocol preference per
ledger; term/scanlator/source/indexer bonuses). Greedy selection lets packs
and singles compete naturally. Release profiles (per-series, instance
default) hold all filters/scores; no quality ladder until the post-v1
upgrade engine. Every grab *and* rejection is recorded with its full score
breakdown ("why this file" UI). No upgrades in v1: a satisfied slot is
never auto-replaced (D5); manual "replace with…" exists.

## 10. Scheduler

Reuses one internal cron/queue system (Go, persisted jobs):

| Task | Default cadence |
|------|-----------------|
| Metadata refresh (releasing) | 24h |
| Metadata refresh (ended) | 7d |
| New-chapter check per series (bound sources `GetChapters` diff → extends upperBound, enqueues wanted search) | 2h, releasing only |
| Wanted/missing automatic search (volumes + chapters), with per-slot exponential backoff (new releases searched immediately, old misses decay to weekly) | 12h sweep |
| Volume release-date trigger (search on/after `release_date`) | daily |
| Queue/import processing | event-driven + 1m poll |
| Library rescan (new/changed/deleted files, §11.4) | on refresh + daily sweep |
| Health checks (peripherals reachable, bindings valid) | 15m |

## 11. Import, naming, post-processing

### 11.1 Pipeline

```
completed download → identify (parse + grab context) → verify (archive opens,
image count > 0) → for each covered slot: hardlink/move into library path →
write/merge ComicInfo.xml → link File↔slots → notify (webhook) → cleanup
```

- Torrent volume packs: each inner file parsed and imported individually.
- Archives are kept as-delivered (CBZ); folder-of-images sources are zipped
  to CBZ by Suwayomi (`downloadAsCbz`). No image re-encoding in v1.
- Manual import queue for anything unidentifiable.

### 11.2 Library layout & naming `[default, fully templatable]`

```
{library}/
  {Series Title} [mangabaka-{seriesId}]/
    {Series Title} v{volume:00} [{edition}][{lang}].cbz        ← volumes
    chapters/
      {Series Title} c{chapter:000} [{group}][{lang}].cbz      ← chapters
```

- Tokens: `{Series Title}`, `{Series TitleSlug}`, `{volume}`, `{chapter}`,
  `{edition}`, `{lang}`, `{group}`, `{seriesId}`, `{collectionId}`,
  `{isbn}`, `{year}` with padding syntax `{volume:000}`.
- Default keeps IDs in the *folder* name and richness in ComicInfo, not a
  parenthesis-soup filename — readers parse `v01`/`c001` natively. Your
  draft's `(key:value)` filename standard remains possible via templates.
- Chapters in a `chapters/` subfolder so volume rows stay clean in readers;
  configurable to flat. (Komga: keep "oneshot in subfolder" behavior in
  mind; verify during implementation.)

### 11.3 ComicInfo.xml `[default: on]`

Written into each CBZ on import (merged if present, Sento fields win,
original preserved as `ComicInfo.orig.xml` once):

- `Series`, `Title`, `Number` (chapter) / `Volume`, `Count` (final_volume
  when series completed), `LanguageISO`, `Writer`/`Penciller` (authors/
  artists), `Summary`, `Genre`, `Web` (mangabaka URL), `AgeRating` (from
  content_rating), `Notes` = `mangabaka:{seriesId}
  edition:{name}/{lang}/{medium} sento:v1` (machine-readable provenance —
  deliberately the series ID + edition *descriptor*, not the collection UUID,
  since UUIDs are volatile (D13) and CBZs are effectively immutable).

### 11.4 Existing-library import & rescan (D14)

Sonarr-style adoption of a pre-existing library. Two entry points: **bulk**
("import library" on a root folder — each subfolder is a series candidate)
and **per-series** ("add series, I already have files at …").

**Root folders** are a first-class config concept (multiple allowed); the
`{library}` token in naming templates resolves against the series' root.

**Series identification**, evidence in trust order (first hit wins, all
recorded in the import trace):

1. Sento provenance in a contained CBZ's ComicInfo (`Notes:
   mangabaka:{id}`) — re-adoption after reinstall is exact and instant.
2. MangaBaka ID in the folder name (`[mangabaka-377]`, our own layout).
3. `ComicInfo.xml` `Series` field from contained archives (komf-tagged and
   reader-managed libraries have good data here).
4. Blind parse of the folder name (parser §titles) → MangaBaka search.

Bulk search is throttled to MangaBaka's 30 searches/min — the import runs
as a background job with progress UI (500 series ≈ 20 min, fine); DB mode
(§13) removes the wait later. Matches ≥ auto-threshold are accepted;
the rest land in a review screen (suggestions + manual search), exactly like
Sonarr's import mapping page. Unmatched folders are listed, skippable,
revisitable.

**Edition assignment**: imported volume files need a monitored collection.
Heuristics, in order: edition hints in filenames (omnibus/colored/publisher
tags vs collection descriptors), file count vs collection `count_main`,
else default to the instance-language standard edition. Always shown and
editable in the review step `[default]`.

**File linking**: every archive in the series folder is parsed (`file`
channel, anchored) + ComicInfo `Volume`/`Number` fields used as
corroborating evidence; files link to slots as satisfied, exactly as if
Sento had downloaded them. Unparseable files → manual import queue;
slot-less matches → unrequested files (§4.3). **Chapter cutoff default on
import**: lowest owned chapter if chapter files exist (so owned chapters are
all in-ledger and the frontier continues), else latest-at-add (D6).

**Non-destructive by default**: import never moves, renames, or rewrites
anything — files are linked where they are, ComicInfo is *read* but not
written. "Organize" (rename/move to template + write ComicInfo) is a
separate, explicit, per-series or bulk action with a dry-run preview.

**Formats**: CBZ/CBR/ZIP/(PDF/EPUB pass-through) in v1. Folder-of-images
chapters (Mihon local downloads, HakuNeko) are detected and surfaced but
not imported; a "pack to CBZ" assist is roadmap (§13).

**Rescan**: triggered on series refresh, manual button, and a daily sweep
`[default]` — detects new files (→ identify + link), changed files (size/
mtime → re-hash), and deleted files (→ unlink; the slot becomes wanted
again and *may re-download* — surfaced prominently in the activity feed,
same semantics as Sonarr).

## 12. Application layer

- **API**: REST/JSON (OpenAPI spec in repo), API-key auth; SSE stream for
  queue/import events. The TS SPA is a pure API client (no server-rendered
  state), so third-party clients get full power.
- **UI (TypeScript SPA)** `[default: React + Vite]`: views — Library (series
  grid), Series detail (collections + the two ledgers, slot status, search
  buttons), Add series (MangaBaka search → pick collections → options),
  Queue, Wanted, Manual import, Settings, System/health.
- **DB**: SQLite via `sqlc` or `ent` `[default: sqlc + migrations]`; raw
  MangaBaka JSON blobs cached per series with schema-version tags (§5.1).
- **Config**: file + env (12-factor-ish); secrets (API keys for peripherals)
  in config; no DB-stored credentials in v1.

## 13. Post-v1 roadmap

All former open questions are resolved (answers folded into D10–D13 and the
sections above). What's deliberately deferred, roughly in order:

1. **Upgrade engine** — quality ladder within a ledger (web→digital→
   remaster) and volume-supersedes-chapters; both blocked on the
   vol↔chapter mapping/confidence design (D5).
2. **Suwayomi management from Sento UI** — extension install/update/health
   via GraphQL, so users never open the Suwayomi WebUI; longer-term, a
   possible slim replacement engine behind the same `SourceProvider`.
3. **MangaBaka database mode** — local weekly-dump mirror replacing API
   calls (§5.2).
4. **RSS sync** via Prowlarr for passive release discovery (§7).
5. **Folder-of-images adoption** — "pack to CBZ" assist for Mihon/HakuNeko
   style libraries detected during import (§11.4).
6. **Multi-language chapter ledgers** — explicitly rejected for v1 (D10);
   revisit only on demand.

Explicitly rejected (not deferred): Komga/Kavita scan webhooks — files on
disk are the whole integration surface.


## 14. Milestones (proposal)

1. **M0 — skeleton**: Go binary, SQLite, config, REST scaffold, SPA shell,
   docker-compose with Suwayomi.
2. **M1 — metadata**: MangaBaka client, add-series flow, collections/works
   sync, ledgers + slots visible in UI.
3. **M2 — parser**: release parser + regression corpus (seeded with names
   scraped from real indexers/sources), series matcher.
4. **M3 — chapters path**: Suwayomi adapter, bindings, chapter wanted-search,
   download, import pipeline, naming, ComicInfo.
5. **M4 — volumes path**: Prowlarr search, qBittorrent client, pack handling,
   hardlink import.
6. **M5 — automation**: scheduler loops, backoff, health, notifications.
7. **M6 — adoption & polish**: existing-library import + rescan (§11.4),
   manual import, unrequested files, overrides, docs. (Library import only
   needs M1+M2 — pull it earlier if real-library testing is wanted sooner;
   it's also the best parser-corpus generator we have.)
