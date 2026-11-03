# Profiles and acquisition state

Profiles decide which releases are acceptable; the shared service ledger decides which books need a copy. Both search and the UI use the same evaluation. See [the edition model](editions.md) for identity and recovery rules.

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
equality (`fr` matches `fr-ca` via `baseLang`), or `*` for any stated language; flag looks up a boolean
parse fact; protocol is case-insensitive equality; resolution is a
minimum. `Validate` rejects malformed conditions before persistence
(bad regex, unknown flag, non-torrent/source protocol, non-positive
resolution).

`Input` is the candidate's view a format matches against — note it is the
*already-parsed* facts (Title, Group, Language, Flags…), not a raw name.
This is the clean layer boundary: `parse` produces facts, `profile`
matches facts. (It does NOT re-tokenize names — that's why `profile`
does not depend on I/O.)

### Profiles — scores + funnel knobs

A `Profile` does three jobs:

1. **Scores formats**: `FormatScores` maps format id → score; the funnel
   sums the scores of every format a candidate matches. Below
   `MinFormatScore` is a *soft* reject (force-grab overrides);
   `MinScoreOff` (-10000) disables the floor.
2. **Requires or refuses formats**: every ID in `Required` must match, and no ID in `Refused` may match. These are hard restrictions that force-grab cannot override. `Profile.Judge` supplies both the score and the first broken rule to search, import, and existing-file fulfillment. Scores remain independent per profile, including for required formats.
3. **Parameterizes the funnel per ledger**: `Volume` and `Chapter`
   `LedgerRules` carry size min/max, min seeders, and a preferred
   protocol (a +200 scoring bonus, not a hard filter — both protocols
   stay eligible). `DefaultRules` are the constants that applied before
   profiles existed: volumes 20–1500 MB / prefer torrent, chapters
   2–200 MB / prefer source.

## Shared ledger

`internal/service/versions.go` loads books, collected versions, per-book overrides, verified file contents, selections/pins, stored presence, pending jobs, and effective profiles in batches. `targetSlots` evaluates each version/book pair. GETs never initialize chapters or check paths.

Monitoring is gated by the series, parent edition, work, version, extras rule, exclusive chapter cutoff, and individual overrides. Retired or catalog-missing publications stop automation. Availability stays released, upcoming, or unknown; unknown dates follow the version’s explicit policy.

On-disk contents are independent of fulfillment. A confirmed archive can satisfy multiple versions of the same edition when it meets each version’s language, group, and profile requirements. Policy changes may leave an existing copy unsatisfactory. Pins prevent upgrades; changing or deleting edition preferences does not delete knowledge of an archive’s contents.

The book's Files tab and file-matching dialog show each file's current custom-format score, effective profile, and matched formats with their individual scores. This uses the same stored facts, release claims, and confirmed edition context as fulfillment and upgrade decisions, evaluated against today's profile settings. The matching dialog evaluates the selected collected version; unsaved corrections do not change the displayed current score. Its separate copy-choice preview scores proposed corrections before Apply choices, which saves the facts and selections together. A zero score is displayed explicitly, and unknown facts are identified as potentially incomplete.

Pending downloads and jobs awaiting review keep reservations. Storage uncertainty blocks acquisition. Wanted therefore means the book is collectible, available under policy, lacks a satisfactory copy (or qualifies for an enabled upgrade), and has no pending claim or unresolved storage/content issue. Upgrade eligibility alone never drives an unattended search: Sento's own scheduled sweeps plan only missing and unsatisfactory books, and a better copy is grabbed when a person searches the book or the edition.

Acquisition identity is `(version ID, book ID)`, never the displayed volume/chapter number. The earlier number-keyed slot model (`internal/ledger`, `/series/{id}/slots`, number-only search requests) has been removed.
