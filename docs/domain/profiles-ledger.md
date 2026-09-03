# Profiles & the ledger

> Tour session 2 — covers `internal/profile` and `internal/ledger`.
> Specs: [decision-engine.md §5](../spec/decision-engine.md),
> [spec.md §4.2](../spec/spec.md). Both are pure domain (no I/O); the store
> persists, the service validates, the funnel/UI consume.

## Why these two together

They are the two halves of "what should I grab?":

- **`ledger`** answers *what do I want?* — derives the list of numbered
  slots (volumes, chapters) a series should own, and marks which are
  still missing (`Wanted`). Pure function of stored metadata + file links.
- **`profile`** answers *which release is good enough?* — a Sonarr-style
  custom-format + scoring model plus the per-ledger funnel knobs. Pure
  predicate/score over a candidate.

The funnel (session 6) sits between them: ledger says slot 5 is wanted,
the providers return candidates, profile scores them, the best covering
candidate gets grabbed.

## `internal/profile`

### Custom formats — Sonarr's matching semantics

A `Format` is a named list of `Condition`s. The matching rule
(`Format.Matches`) is the subtle part and worth stating precisely:

- **AND across condition *types*, OR within a type.** For each condition
  *type* that has non-required conditions, at least one must hit; every
  such type must be satisfied. So "(title~digital OR title~webrip) AND
  (protocol=torrent)".
- **Required conditions are hard** — any required condition that fails
  rejects the whole format immediately, regardless of the OR groups.
- **Negate inverts** a single condition (`satisfied = test() != Negate`).
- **An empty format matches nothing** — it can't mean "everything"
  (guard at the top of `Matches`).

Condition types (`CondTitle`, `CondGroup`, `CondLanguage`, `CondFlag`,
`CondProtocol`, `CondSite`, `CondResolution`) each have a `test`:
title/group/site are case-insensitive regex; language is base-language
equality (`fr` matches `fr-ca` via `baseLang`); flag looks up a boolean
parse fact; protocol is case-insensitive equality; resolution is a
minimum. `Validate` rejects malformed conditions before persistence
(bad regex, unknown flag, non-torrent/source protocol, non-positive
resolution).

`Input` is the candidate's view a format matches against — note it is the
*already-parsed* facts (Title, Group, Language, Flags…), not a raw name.
This is the clean layer boundary: `parse` produces facts, `profile`
matches facts. (It does NOT re-tokenize names — that's why `profile`
imports nothing.)

### Profiles — scores + funnel knobs

A `Profile` does two jobs:

1. **Scores formats**: `FormatScores` maps format id → score; the funnel
   sums the scores of every format a candidate matches. Below
   `MinFormatScore` is a *soft* reject (force-grab overrides);
   `MinScoreOff` (-10000) disables the floor.
2. **Parameterizes the funnel per ledger**: `Volume` and `Chapter`
   `LedgerRules` carry size min/max, min seeders, and a preferred
   protocol (a +200 scoring bonus, not a hard filter — both protocols
   stay eligible). `DefaultRules` are the constants that applied before
   profiles existed: volumes 20–1500 MB / prefer torrent, chapters
   2–200 MB / prefer source.

## `internal/ledger`

The one architectural invariant: **slots are computed, never stored**.
The DB holds metadata (collections, works), file↔slot links, and
monitoring overrides; the ledger is a pure function of those. This makes
refresh/reconcile cheap and conflict-free.

### `BuildVolumeSlots` — one per collection

A volume slot per `count_type=main` work (extras exposed but unmonitored
by default, D11). Three derived booleans per slot:

- **Monitored** = collection monitored AND `count_type==main`, unless an
  explicit per-slot `override` says otherwise.
- **Released** — the tricky one, because MangaBaka dating is patchy. A
  dated work is released iff its date ≤ today; an *undated* work is
  released iff the collection has no dates at all, or its sequence sits
  at/below the highest *dated* sequence (spec.md §4.2). So undated early
  volumes count as released, undated volumes beyond the dated frontier
  don't.
- **Wanted** = monitored AND released AND no files linked. This is the
  output the funnel searches for.

### `BuildChapterSlots` — per series, not per collection

Slots `1..totalChapters`, integer-only, from MangaBaka `total_chapters`
*only* — sources never feed the chapter bound (D7-rev/D15). Monitoring is
**cutoff-based** (D6): a slot is monitored iff the collection is monitored
and its number is *above* `CutoffNum` (cutoff itself excluded — "monitor
from N onward"). Overrides apply on top. Chapter slots have no `Released`
field — a chapter's existence implies release, so Wanted = monitored AND
unowned. Zero total → nil (no slots).

`SlotKey(ledger, collectionID, number)` (`"volume/7/500"`) is the join key
between a slot and its linked files — the same string the importer writes
when it links a file.

## Session log

- [x] profile walkthrough — Sonarr matching semantics (OR-within-type,
      AND-across-types, required-is-hard, negate-inverts, empty-matches-
      nothing), condition `test` dispatch, the Input-is-parsed-facts layer
      boundary, Validate gatekeeping, scores + per-ledger funnel knobs
      (preferred protocol = +200 bonus not a filter).
- [x] ledger walkthrough — computed-not-stored invariant; BuildVolumeSlots
      (monitored/released/wanted, the undated-release frontier heuristic);
      BuildChapterSlots (cutoff-based D6, total_chapters-only D15, no
      Released field); SlotKey as the slot↔files join.
- [x] verified against all 12 tests (green); no bugs. One efficiency lead
      logged: `matchRegexp` recompiles per call (hot in the funnel) —
      improvements.md, deferred to session 6.

**Session 2 complete.** profile + ledger fully toured. Both pure, clean,
well-tested; the "what do I want / is this good enough" pair the funnel
joins in session 6.
