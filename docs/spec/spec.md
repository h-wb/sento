# Sento — Specification (v1 draft)

> Current edition, acquisition, and recovery behavior is specified in [the domain model](../domain/editions.md). Sections marked `[default]` remain proposals.


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
| D1 | Metadata source of truth | MangaBaka supplies catalog metadata; independent local editions and field overrides are supported. |
| D2 | Library entity | **Series + monitored editions**: entry = MangaBaka series; user monitors ≥1 collections (edition × language × medium) under it |
| D3 | Extension engine | **Stock Suwayomi as headless sidecar**, accessed via its GraphQL + HTTP API, hidden behind a `SourceProvider` interface |
| D4 | Tracking model | Stable edition/work IDs with independent acquisition targets; one shared evaluator for search and UI. |
| D5 | Vol↔chapter mapping | **None in v1.** No auto-supersede, no auto-delete. Schema stays mapping-ready (files record parsed vol/ch ranges) |
| D6 | Chapter monitoring | Skip through chapter N: number > N, with per-book overrides including chapter zero. |
| D7 | Chapter slot bound | Explicit chapter memberships; optional series-count inference is capped at 10,000. |
| D8 | Slot numbering | Integer hundredths support fractional numbering and true zero; specials may be unnumbered. |
| D9 | Stack | Go backend (single binary), TypeScript web UI (SPA served by the binary) |
| D10 | Chapter language | Language belongs to each edition; several chapter languages can coexist. |
| D11 | Extras/specials | Regular, bonus/special, and other books; Include extras and per-book monitoring controls. |
| D12 | Pack auto-grab | Ship conservative thresholds + manual confirm; tune with real-world use |
| D13 | MangaBaka volatility | MangaBaka (esp. collections/works) is WIP and may change: Sento isolates it behind a `MetadataProvider` anti-corruption layer, owns its identity model, caches raw payloads, and degrades gracefully (§5.1) |
| D14 | Existing-library import | First-class, Sonarr-style: scan root folders, match series, link files to slots **in place** — non-destructive by default, renaming/organizing only as an explicit separate action (§11.4) |
| D15 | Metadata/download separation | Verified source observations enter review or an explicitly trusted binding; never expand numbering from source maxima. |
| D16 | Review proposes | Review items carry a proposed resolution read from evidence (a file's name, the download it came in), accepted one by one or together; nothing is recorded until accepted, and automatic acquisition stays as strict as before ([propose-dont-ask.md](../reviews/propose-dont-ask.md)). |

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

Series contain independently identified editions and books. Acquisition targets describe which copies to keep, with profile inheritance and per-book overrides. Files record intrinsic contents and observed facts separately from release claims. Fulfillment is derived from present verified files and current target rules; selections and pins are explicit overrides.

### 4.2 Ledger semantics

The shared series ledger gates collection on parent/catalog state, target intent, availability, extras/cutoff rules, pending reservations, storage health, and optional upgrades. Dates may be unknown. New targets can reuse existing qualifying files. Unique books on disk and wanted copies are separate totals. Volume and chapter zero and fractional labels have stable work IDs.

### 4.3 Files standing alone

Unmatched files stay on disk and enter the shared review inbox. Confirmed contents survive target deletion. Policy-incompatible files remain visible as on disk. No automatic cleanup or chapter-to-volume replacement is performed.

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
- **Stable local identity.** Provider references and aliases are resolved first. Exact descriptors are fallback evidence only for a unique already-linked edition. Local editions are linked explicitly, never silently adopted. Missing catalog editions retain books and files while automation pauses.
- **Raw payload cache.** Every series/collections/works response is stored
  as raw JSON with a schema-version tag. If MangaBaka changes shape, a new
  adapter version re-derives our columns from cached payloads — no data loss,
  no full re-crawl.
- **Drift detection.** Contract tests run against recorded fixtures; a
  scheduled live smoke-check (one known series) compares actual responses to
  the expected shape and surfaces "metadata provider drift" as a health
  warning instead of crashing sync.
- **Degraded mode.** A failed catalog refresh keeps existing metadata. A successful snapshot marks absent entries stale. Explicit local continuation preserves the publication while detaching its upstream references. The default chapter edition may infer a bounded series count; local chapter editions start with explicit books.

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
- Refresh cadence `[default]`: releasing and on-hiatus series every 24h; completed/cancelled
  every 7d; manual refresh button. Works re-paged only when collection
  `count_main` or `updated_at` changes.
- `merged_with` handling: if a series was merged, follow the pointer,
  remap the library entry, notify.
- Per-series overrides: title, language, cutoff, etc. stored separately and
  reapplied after refresh (never clobbered).

## 6. SourceProvider interface (Suwayomi adapter)

The only part of Sento that knows Suwayomi exists. **Full component spec,
live-validated against a real instance: [`suwayomi-adapter.md`](suwayomi-adapter.md).**

> Implementation note: this generalized into `internal/provider` — a
> `Provider` interface (Sites/Search/Grab/State/Fetch/Cleanup) + registry,
> with `suwayomi` and `torrent` (Prowlarr+qBittorrent) implementations.
> The decision funnel and importer are provider-agnostic; candidates carry
> uniform `provider/site/protocol` tags.

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
and singles compete naturally. Profiles inherit from version to edition to series to the instance default. Optional score-based upgrades respect pins and retain earlier copies. Every grab and rejection records its decision details.

## 10. Scheduler

Reuses one internal cron/queue system (Go, persisted jobs):

| Task | Default cadence |
|------|-----------------|
| Metadata refresh (releasing or hiatus) | 24h |
| Metadata refresh (ended) | 7d |
| New-chapter check per series (verified bindings propose individual observations for review or configured trust) | 2h, releasing or hiatus only |
| Wanted/missing automatic search (volumes + chapters), with per-slot exponential backoff (new releases searched immediately, old misses decay to weekly) | 12h sweep |
| Volume release-date trigger (search on/after `release_date`) | daily |
| Queue/import processing | event-driven + 1m poll |
| Library rescan (new/changed/deleted files, §11.4) | on refresh + daily sweep |
| Health checks (peripherals reachable, bindings valid) | 15m |

## 11. Import, naming, post-processing

### 11.1 Pipeline

```
completed download → identify files → install without overwriting →
record per-file facts and intrinsic contents atomically → verify expected coverage → cleanup
```

Ambiguous or partial imports retain files and reservations in the review inbox. Packs remain one physical archive where possible. Original torrent payloads stay in place for seeding. Cleanup follows completed imports; a retry does not submit another download.

### 11.2 Library layout and naming

New files use stable IDs and a content hash:

`{series folder}/collections/{edition ID}/versions/{target ID}/{book name} [{sha256}].cbz`

Names may use configured title, number, language, edition, and group tokens. Existing paths remain intact. Hash-addressed destinations and exclusive installation prevent one release from overwriting another. Filename organization is independent of intrinsic content mappings.

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
*Correction (2026-10-29):* latest-at-add applies when a volume edition is
collected; a series added without one (a one-shot, a chapter-only series)
wants every chapter.

**Non-destructive by default**: import never moves, renames, or rewrites
anything — files are linked where they are, ComicInfo is *read* but not
written. "Organize" (rename/move to template + write ComicInfo) is a
separate, explicit, per-series or bulk action with a dry-run preview.

**Formats**: CBZ/CBR/ZIP/(PDF/EPUB pass-through) in v1. Folder-of-images
chapters (Mihon local downloads, HakuNeko) are detected and surfaced but
not imported; a "pack to CBZ" assist is roadmap (§13).

**Rescan** checks stored file presence and imports newly discovered paths in place. Known paths are retained; ordinary page reads perform no filesystem access. Missing archives under healthy roots become wanted, while unavailable storage blocks acquisition and appears in review.

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

1. **Cross-edition replacement and retention** — automatic chapter-to-volume replacement and destructive cleanup remain deferred. Score-based upgrades within an edition are available.
2. **Suwayomi management from Sento UI** — extension install/update/health
   via GraphQL, so users never open the Suwayomi WebUI; longer-term, a
   possible slim replacement engine behind the same `SourceProvider`.
3. **MangaBaka database mode** — local weekly-dump mirror replacing API
   calls (§5.2).
4. **RSS sync** via Prowlarr for passive release discovery (§7).
5. **Folder-of-images adoption** — "pack to CBZ" assist for Mihon/HakuNeko
   style libraries detected during import (§11.4).
6. **Richer content relationships** — multilingual chapter editions already exist; automatic equivalence or translation inference remains out of scope.

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
