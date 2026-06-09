# Sento — Decision Engine & Release Profiles Specification

Component spec for grab decisions (spec.md §9). Consumes parser/resolver
output (`parser.md`); feeds the download clients and the import pipeline.
Philosophy carried over from the parser: **every decision is traced** —
each grab and each rejection records its rule/score breakdown, surfaced in
the UI and logs.

---

## 1. Role and triggers

For a *decision request* — (series, ledger, slot set) — the engine:

```
gather candidates → parse+resolve (R1–R7) → hard rejections → score →
select → grab → record decision
```

Triggered by:

| Trigger | Slot set |
|---|---|
| Interactive search (UI, per slot / per series) | chosen slots; returns *all* candidates incl. rejected, with reasons |
| Automatic wanted sweep (§10 spec.md, with backoff) | all wanted slots of a series, batched |
| New-chapter event (source check raised upperBound) | the new chapter slots |
| Volume release-date trigger | the released volume slot |
| Manual force-grab | bypasses soft rules (see §7) |

## 2. Candidate gathering

Both ledgers draw from **both protocols**; eligibility is decided by what
the name *parses as*, not where it came from (a Madokami "chapter" named
`Berserk v41.cbz` is a volume candidate; a nyaa torrent `One Piece 1184`
is a chapter candidate).

- **Torrent (Prowlarr)**: `GET /api/v1/search?query=&cat=` with manga
  categories. Query construction per request: primary title, then aliases
  on zero results; volume requests also issue plain-title queries (packs
  rarely carry `v01` markers in indexable form). Results cached 15 min per
  normalized query `[default]`.
- **Source (Suwayomi adapter)**: `GetChapters` on each active binding for
  the series. Cached 1 h per binding; the new-chapter check and interactive
  search force-refresh `[default]`.

All candidates run through `ParseAnchored` with the series' titles and the
resolver (parser.md §7) before entering the funnel.

## 3. Hard rejections

Ordered; first failure wins; all are recorded with machine-readable codes
(`rejected.content_type`, `rejected.seeders`, …). Hard = never auto-grabbed.

1. Resolver gates R1 (content-type), R2 (language) and series-match
   threshold (parser.md §6).
2. **No wanted-slot coverage** in the requested set (after R5 slot mapping).
3. **Edition routing** failure (R6) for volume candidates.
4. **Pack rule** (D12): `wantedCovered / totalCovered < threshold`
   (default 0.5) → manual queue, not auto (soft-ish: still shown,
   never auto-grabbed).
5. **Size sanity** per profile: volume 20–1500 MB, chapter 2–200 MB
   `[default]` (computed per covered unit for packs; torrents only —
   source downloads have no advertised size).
6. **Torrent health**: seeders < min (default 1).
7. **Already handled**: slot pending in queue; release blocklisted;
   identical release (infohash / source chapter id) already grabbed.
8. Profile **block terms** match (must-not-contain).
9. Profile **require terms** miss (must-contain, when configured).

## 4. Scoring

Transparent weighted sum; weights live in the release profile so they're
user-tunable, with these shipped defaults:

| Dimension | Default score |
|---|---|
| Per wanted slot covered | **+1000** (dominant: more missing content wins) |
| Per unmonitored/extra slot covered (overshoot) | −10 |
| Protocol preference | volume ledger: torrent +200 / source +0 · chapter ledger: source +200 / torrent +0 |
| Edition descriptor exact match (vs merely compatible) | +150 |
| Scored terms (profile) | shipped: `(Digital)` +100, `Premium` +50, `(LQ)` −100, `Compressed` −50; user-extensible `(pattern, score)` list |
| Scanlator preference (chapter) | preferred list: +100 by rank; `Official` +50 unless listed |
| Source priority (chapter, per-series override > global order) | +50 by rank |
| Indexer priority (Prowlarr value) | ±25 normalized |
| Seeder tiers (torrent) | 6–20: +25 · >20: +50 |
| Parse confidence < 0.8 | −100 (prefer names we understood fully) |

Tie-breakers, in order: fewer overshoot slots (most targeted release),
higher seeders, newest publish date.

Selection: per ledger, greedily pick the highest-scoring candidate; remove
the slots it covers from the request; repeat until no wanted slots remain
or no eligible candidates (those slots keep their backoff schedule). A pack
and singles thus compete naturally: the pack's coverage bonus wins exactly
when it covers more wanted slots than any single.

## 5. Release profiles

One entity, assignable per series; an instance **default profile** applies
otherwise. No quality *ladder* in v1 (no upgrades, D5); the profile is
filters + scores:

```yaml
name: default
volume:
  protocols: [torrent, source]        # both eligible; order = preference bonus
  size_mb: {min: 20, max: 1500}       # per volume
  min_seeders: 1
  delay_minutes: 0                    # wait before auto-grab of newly seen release
chapter:
  protocols: [source, torrent]
  size_mb: {min: 2, max: 200}
  min_seeders: 1
  delay_minutes: 0
terms:
  block: []                           # hard reject
  require: []                         # hard require (empty = off)
  scored: [{pattern: "(Digital)", score: 100}, …]
scanlators:
  preferred: []                       # ranked
  blocked: []                         # hard reject
sources:
  priority: []                        # ranked source ids; unlisted = 0
weights: {…}                          # the §4 table, overridable
```

Forward-compat note: post-v1 upgrades (roadmap #1) add ranked *quality
groups* + cutoff to this same entity; nothing in v1 storage precludes it.

## 6. Queue interaction & failure handling

- A grabbed candidate creates a `QueueItem` holding its expected slot set;
  those slots become **pending** (excluded from sweeps and other grabs).
- **Torrent failures**: stalled beyond `stalled_timeout` (default 60 min)
  or failed import → blocklist (infohash, reason; permanent until user
  clears, Sonarr-style) → slots return to wanted → immediate re-search
  excluding blocklisted.
- **Source failures**: Suwayomi retries internally (`tries`); on terminal
  error → blocklist (source, chapterId) for 24 h `[default]` → next
  candidate (other scanlator/source) considered immediately.
- **Import failures** (unparseable archive, verification fail): manual
  import queue + blocklist entry with reason.
- Per-slot search backoff per spec.md §10 (immediate → decaying to weekly).

## 7. Interactive search & force-grab

Interactive search returns the **full funnel**: every candidate with its
parse trace, rejection codes, and score breakdown — the engine's debugger
UI. Force-grab from there bypasses soft rules (pack threshold, size,
seeders, terms, delay) with a confirm dialog; hard gates that survive
force: content-type and series-match (grabbing those routes through manual
import anyway, where the user can override identification explicitly).

## 8. Decision records

Every run persists: request (slots), candidate count, top-N candidates
with scores, chosen release(s) with full breakdown, rejection histogram.
Retention 90 days `[default]`. Powers: activity feed, "why this file"
on every imported file (joins grab context → decision → parse trace), and
future scoring tuning.

## 9. Deferred (post-v1)

- Upgrade engine: quality groups + cutoff + supersede policies
  (roadmap #1; needs vol↔chapter mapping for cross-ledger upgrades).
- RSS-driven passive decisions (roadmap #4) — same funnel, push-fed.
- Learned group/source reliability scores from import history.
- Delay-profile sophistication (per-protocol delays, "prefer official
  within N hours" windows).
