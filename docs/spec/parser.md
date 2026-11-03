# Sento — Release Parser & Matcher Specification

Component spec for the release-name parser (§8 of `spec.md`). The parser is
the highest-risk component in Sento: every grab decision flows through it,
and the input space is adversarial (two decades of inconsistent human naming
conventions). This spec defines its architecture, the full pattern inventory
grounded in real release names, and the testing regime.

> Real examples in this document were harvested from nyaa.si (manga
> category) and Tachiyomi source chapter lists in June 2026; they seed the
> regression corpus.

---

## 1. Role and boundaries

The parser turns **one name string** into **structured facts**. It is:

- **Pure.** No network, no DB, no global state. `Parse(name, channel)` is a
  deterministic function — this is what makes the corpus testable.
- **Context-free first, context-resolved second.** Two layers:
  1. `parse` — extracts everything derivable from the string alone.
  2. `match` (resolver) — combines the parse with context (library titles,
     ledger bounds, channel, monitored collections) to produce a grab/import
     decision. Separate package, separately tested.
- **Explainable.** Every result carries its token trace and the rules that
  fired. The UI's manual-import screen and the logs show *why* a name parsed
  the way it did. (Sonarr's biggest debugging pain — designed in from day 1.)

Inputs arrive through three **channels**, each with different conventions
and different priors:

| Channel | Source | Typical shape |
|---|---|---|
| `torrent` | Prowlarr results | `Berserk v41 (2022) (Digital) (LuCaZ)` — rich tags, packs, groups |
| `source` | Suwayomi `fetchChapters` titles | `Vol. 5 Ch. 34 - The Sword Master`, `Chapter 1184`, `Episode 12` — terse, marker-heavy, *series already known* |
| `file` | import scans, pack contents | torrent conventions + extensions + path segments |

`source` names are always parsed **anchored** (series known from the
binding); `torrent`/`file` may be anchored (searching for a specific series)
or **blind** (manual import, RSS later).

## 2. Output schema

> **Correction (2026-06):** this sketch predates the implementation and has
> drifted. The source of truth is `internal/parse/result.go`. Known
> renames: `TitleGuess` → `Title`, `IsComplete` → `Complete`, `IsExtra` →
> `Extra`; `Channel` is a `string` (not `uint8`); `ParseAnchored` takes
> `[]string`, not `[]NormTitle`. The field *meanings* below are still
> accurate.

```go
package parse

type Channel uint8 // Torrent, Source, File

// Num is a fixed-point chapter/volume number: 34.5 → 3450, 4 → 400.
// Hundredths; alpha suffixes map a→10, b→20 (Mihon: 4a ≈ 4.1).
type Num int64

type Range struct{ From, To Num } // single unit: From == To

type Result struct {
    TitleGuess  string   // free text believed to be the series title
    AltTitles   []string // dual-title names: "Only I Level Up / Solo Leveling"
    Volumes     []Range  // e.g. v01-03 + v05 → [{1,3},{5,5}]
    Chapters    []Range  // decimals preserved; integer-slot policy applied later
    Season      *int     // webtoon/manhwa season marker (S2)
    Year        *int     // single year  (2022)
    YearRange   *[2]int  // pack year span (2003-2026)
    Group       string   // release group / scanlator
    Language    string   // BCP-47-ish if tagged ([ENG], (Fr)); "" = untagged
    ContentType ContentType // Manga (default), Novel, Audiobook, Artbook, Anime
    Edition     EditionHints  // see §5.4
    Quality     QualityHints  // see §5.5
    IsComplete  bool     // (Complete), (Full), [END], no-number pack
    IsExtra     bool     // extra/omake/special/oneshot marker
    Version     int      // v2 fix releases, (F) → fixed
    Confidence  float64  // structural confidence of the parse itself
    Trace       []Step   // tokens + rules fired (explainability)
}

type EditionHints struct {
    Omnibus  bool // omnibus, 2-in-1, 3-in-1
    Deluxe   bool
    BoxSet   bool
    Colored  bool // (Colored), Digital Colored Comics, FULL COLOR
    Official bool // publisher-rip tags: (Viz), [Seven Seas], (Kobo)…
    Raw      bool // Japanese raws: 第NN巻, (RAW)
    Names    []string // verbatim edition-ish strings for descriptor matching
}

type QualityHints struct {
    Digital    bool // (Digital)
    Scan       bool // (Scanlated), (Scan), c2c
    Web        bool // (Webtoon), (Tapas), web rip platform tags
    Premium    bool // [Premium] = watermark-free
    Resolution int  // 1920, 2048… from (2048px), (x3200)
}
```

API:

```go
func Parse(name string, ch Channel) Result
// Anchored: known candidate titles (title + aliases, pre-normalized).
// Locates the best title occurrence, strips it, parses the remainder —
// numbers inside the title ("Mob Psycho 100") never leak into slots.
func ParseAnchored(name string, ch Channel, titles []NormTitle) Result
```

## 3. Pipeline

```
input
 │ 1. pre-clean      ext strip (.cbz/.cbr/.zip/.pdf/.epub), path split,
 │                   _→space, NFKC (full-width → ASCII), collapse ws
 │ 2. segment        bracket [..] / paren (..) / brace {..} segments vs
 │                   free text; preserve order & position
 │ 3. classify       each segment → tag class (year, group, language,
 │                   quality, edition, content-type, junk) via tag tables §5
 │ 4. anchor (opt)   locate + strip known title (anchored mode)
 │ 5. numbers        marker-bound numbers in free text & segments
 │                   (v/vol/ch/c/tome/t/episode/第N巻/第N話…), then bare
 │                   numbers with positional rules
 │ 6. assemble       title guess = leading free text before first
 │                   structural marker; ranges merged; flags folded
 │ 7. score          confidence from rule quality (marker-bound beats bare,
 │                   classified segments beat unknowns)
 └→ Result + Trace
```

Implementation guidance (not contract): segment classifiers are table-driven
(tag string → class), number extraction is a small set of compiled regexes
per marker family. **No single mega-regex.** Order of rules is part of the
spec and locked by the corpus.

## 4. Number semantics

- **Marker-bound beats bare**: `v05 c034` → volume 5, chapter 34. A bare
  number is only assigned when no marker-bound number of that axis exists.
- **Bare-number axis** is decided by the *resolver* (§7), not the parser.
  Parser emits `BareNumbers []Range` internally and defers; the channel
  priors in `resolveBare` then claim what the string says the chapter way
  (source channel / decimals / ≥1000 / far above marker volumes /
  zero-padded to three or more digits on the torrent channel), and the
  rest stay in `Result.Bare`.
- **Decimals & sub-chapters**: `110.5`, `Ch. 34.2` parsed exactly (`Num`
  hundredths). Alpha suffix: `4a → 4.10`, `4b → 4.20` (Mihon convention).
  `extra/omake/special/side story` after a number → `IsExtra` (Mihon maps
  to .99/.98/.97 — we keep the flag instead; integer-slot policy D8
  decides).
- **Ranges**: `-`, `~`, `–`, `to` between two numbers of the same axis;
  `v01-v38`, `v01-38`, `c001-050`, `[01-270]`, `164-169`. Mixed lists:
  `v01-03 + v05`, `v000-v105 & ch0035-1078` → multiple ranges, both axes.
- **Padding is meaningless to the value, not as a fact**: `001` ≡ `1`.
  Zero-padding to three or more digits is the chapter-number convention:
  the torrent prior above claims such a bare number as a chapter, and the
  padding of those that stay unresolved is kept in `Result.BarePadded`
  for whoever resolves the axis. Leading `v00`/`c000` (prologue
  volume 0) are legal: `Num` ≥ 0.
- **Release version, not number**: `v2` *immediately after* a chapter number
  (`Ch.34v2`) or in a tag segment `(v2)` → `Version=2`, never volume 2.
  Disambiguation: volume markers require word-boundary + no preceding
  number token.
- **Year guard**: a 4-digit number in 1900–2100 inside a tag segment is a
  year (`(2022)`) or year-range (`(2003-2026)`); in free text it is a year
  only if the series context says chapters can't reach it (resolver rule
  R4, §7). `c2024` (marker-bound) is always chapter 2024.

## 5. Pattern inventory

Grounded in the harvested corpus; each row exists as ≥1 corpus case.

### 5.1 Volume markers
`v01` `v1` `Vol. 1` `Vol 1` `Volume 01` `volume64` `Tome 1` `T01` (fr)
`第01巻` `第1-10巻` (raws) — case-insensitive, optional dot/space.

### 5.2 Chapter markers
`c001` `ch. 4` `Ch.34` `Chapter 1184` `Chap 5` `#34` `Episode 12` `Ep. 12`
(webtoons) `第12話` (raws) `- 375` (dash-bound trailing number,
`[Kamizye] Berserk - 375`).

### 5.3 Packs / batch markers
`v01-v38` `Vol. 102-105` `c001-050` `[01-270]` `164-169`
`v001-111 + 1134-1176` `(v000-v105 & ch0035-1078)` `(Complete)` `(Full)`
`Batch` `[END]` `(ongoing)` — and the **implicit pack**: a torrent name with
*no number at all* (`Solo Leveling (2021-2026) (Digital) (1r0n)`) =
whole-series pack, `IsComplete` unless `(ongoing)`.

### 5.4 Edition tags
`(Omnibus)` `2-in-1` `3-in-1` `Deluxe` `Box Set` `(Colored)` `FULL COLOR`
`Digital Colored Comics` publisher rips `[Seven Seas]` `(Viz)` `(Kobo)`
`(Comixology)` `(Tapas)` `[Yen Press]` `(Shonen Jump)`. Verbatim strings are
kept in `Edition.Names` for descriptor matching against MangaBaka
collections (resolver R6).

### 5.5 Quality / provenance tags
`(Digital)` `(Scanlated)` `(Scan)` `(c2c)` `[Premium]` `(Webtoon)`
`(2048px)` `(x3200)` `(HQ)/(LQ)` `(F)`/`(f)` = fixed → `Version`
`{Tapas}` `(Compressed)` `(JPEG)/(PNG)`.
Resolution tokens also accept integer K notation (`2K` → 2048 pixels,
`4K` → 4096) and a `-Repack` suffix (`1800px-Repack` → resolution 1800,
version at least 2). Matching is case-insensitive and consumes the whole
token before group extraction; the highest stated resolution wins.

### 5.6 Content-type tags (rejection-critical)
`(Novel)` `(LN)` `[Audiobook]` `(Artbook)` `(Fanbook)` `(Anthology)`
`(Guidebook)`. Same titles ship as novels/audiobooks (`Berserk of Gluttony
v08 [Audiobook]`, `Solo Leveling (Novel)`); the decision engine **hard-
rejects** `ContentType != Manga` for both ledgers. Light-novel-only series
are out of scope v1 (spec.md §2).

### 5.7 Language tags
`[ENG]` `(English)` `(Fr)` `(VF)` `[JP]` `(RAW)` (raw implies ja unless
stated). Untagged ≠ English — untagged is "unknown" and the resolver applies
the indexer/source language default.
A language word qualifying retained source style (`Japanese Names`, `JP
Honorifics`, `Korean SFX`) is metadata, not a language claim or a group.
Other language tags in the same release still apply.

### 5.8 Group position & delimiters
Trailing parens/brackets (`(LuCaZ)`, `[Stick]`, `(danke-Empire)`,
`(-KS-)`); leading bracket (`[Kamizye] Berserk - 375`, `[0v3r] Berserk
v42`); curly braces (`{4str0}`). Heuristic: the *last* unclassified short
segment is the group; a *leading* unclassified segment is the group when the
remaining text title-matches. Known-group list (learned from history) breaks
ties; never fatal if wrong.

### 5.9 Title hazards
- **Numbers in titles**: `Mob Psycho 100`, `86`, `Steins;Gate 0`,
  `Ranma 1/2`, `22/7` — anchored mode strips the title before number
  extraction; blind mode relies on longest-title-match against the library
  index (§6) and flags `Confidence` down when a bare number could be title.
- **Containment traps**: `Berserk` vs `Berserk of Gluttony` — title matching
  must score *longest normalized match*, and an anchored strip must respect
  word boundaries (`Berserk of Gluttony v01` must not become
  `of Gluttony v01` when searching for Berserk: full-segment match only).
- **Dual titles**: `Only I Level Up / Solo Leveling [Webtoon]`,
  `Solo Leveling | I Alone Level-Up (Novel)` — split on ` / ` and ` | `,
  all parts become title candidates (`AltTitles`).
- **Sequels/seasons**: `Solo Leveling Ragnarok S2 Complete` — `S2` is a
  Season marker, not part of the title; `Ragnarok` stays in the title and
  matching resolves to the sequel series entry (MangaBaka models sequels as
  separate series via `relationships`).

## 6. Title normalization & matching (package `match`)

Normalization (applied to both library titles/aliases and parsed guesses):

1. Unicode NFKC; full-width → half-width
2. lowercase; strip diacritics
3. `&` → `and`; punctuation (`:;,.!?'"`) → space; `-_` → space
4. collapse whitespace; trim articles only for *secondary* keys (keep
   primary exact)

Library index: every series stores all MangaBaka titles + `secondary_titles`
+ user aliases, normalized, in an in-memory index (rebuilt on library
change). Matching:

- exact normalized match → score 1.0
- token-set similarity (Jaro-Winkler per token, order-tolerant) — accept
  ≥ 0.90 auto, 0.75–0.90 manual-confirm queue, < 0.75 no match `[default]`
- always prefer the longest matching title (containment rule §5.9)

## 7. Resolver rules (context pass)

Ordered; first applicable wins per ambiguity. Each fired rule is appended to
the trace.

- **R1 — content-type gate**: `ContentType != Manga` → reject.
- **R2 — language gate**: parsed language incompatible with target ledger
  language → reject (untagged passes with channel default).
- **R3 — bare-number axis**: anchored series gives ledger bounds
  (`final_volume`, chapter upperBound). A bare number `n`:
  - `n ≤ final_volume` and channel `torrent/file` → volume
  - `n > final_volume` → chapter
  - overlap zone with channel `source` → chapter (sources serve chapters)
  - both implausible → unparsed number, confidence hit, manual queue.
  (`One Piece 1184 (Digital)`: 1184 > 114 → chapter. `Berserk v42`: marker-
  bound, no rule needed.)
- **R4 — year vs number**: 4-digit bare number in 1900–2100 → year if the
  chapter upper bound < 1900, else genuinely ambiguous → manual queue.
  Tag-segment year-ranges (`(2003-2026)`) are never chapter ranges.
- **R5 — slot coverage**: map ranges to monitored slots; packs eligible per
  D12 thresholds; already-satisfied covered slots are linked, not re-filled.
- **R6 — edition routing**: `Edition.Names`/hints matched against the
  monitored collections' descriptors (omnibus → omnibus collection, colored
  → colored edition if monitored, else reject `[default: volume releases
  route only to descriptor-compatible collections; plain releases route to
  the sole monitored collection, manual queue if several]`.
- **R7 — season composite**: if Season present and the bound MangaBaka
  series differs per season, re-route to the season's series entry
  (relationships); else season is recorded but ignored for slot math
  `[default — revisit with webtoon corpus growth]`.

## 8. Testing regime

### 8.1 Regression corpus (the core asset)

```
parser/testdata/corpus/
  torrent_volumes.jsonl     torrent_packs.jsonl     torrent_chapters.jsonl
  source_chapters.jsonl     files_import.jsonl      content_type.jsonl
  titles_hazards.jsonl      raws.jsonl              i18n_fr.jsonl
```

One JSON object per line:

```json
{"in": "One Piece v001-111 + 1134-1176 (2003-2026) (Digital) (1r0n)",
 "ch": "torrent",
 "want": {"title": "one piece", "volumes": [[1,111]],
          "chapters": [[1134,1176]], "yearRange": [2003,2026],
          "group": "1r0n", "quality": {"digital": true}}}
```

Rules of the corpus:

- **Golden**: CI gates on 100% pass. A failing line is a build failure.
- **Append-only in spirit**: every production misparse, every bug report,
  every weird name seen in a log becomes a line *before* the fix lands.
- Expectations are *partial* (only asserted fields compared) so adding new
  Result fields doesn't rewrite history.
- Anchored-mode cases carry `"titles": ["..."]` for `ParseAnchored`.

Seed set: the ~80 names harvested for this spec (nyaa One Piece / Berserk /
Solo Leveling pages, Mihon `ChapterRecognition` doc cases) plus Sento's own
template outputs.

### 8.2 Property & fuzz tests

- **Round-trip property**: for every naming template (spec.md §11.2) and a
  generator of series/slot values: `render(template, slots)` then
  `ParseAnchored` must recover exactly the slots. Sento must never fail to
  parse its own output.
- **Go native fuzzing** on `Parse`: no panics, no quadratic blowups
  (pathological bracket nesting), always-valid `Result` invariants
  (`From ≤ To`, `Num ≥ 0`).
- **Benchmark**: ≤ 50µs/name median on corpus (pack scans parse thousands).

### 8.3 Drift watch

The Mihon `ChapterRecognition` conventions (alpha suffixes, extra/omake)
are vendored as corpus cases, not as code. If upstream Mihon changes
recognition behavior, our corpus documents *our* behavior independently.

## 9. Deferred / known-unsolved

- Webtoon per-season chapter numbering reset (R7 records, doesn't solve) —
  needs corpus evidence before designing.
- Learned group list & per-group trust scores — post-v1, feeds the decision
  engine, not the parser.
- Non-Latin release names beyond markers (full CJK titles) — matching works
  via MangaBaka native titles; corpus file exists (`raws.jsonl`) but
  coverage is thin until real use.
- OCR-grade garbage (`0n3 P13c3`) — out of scope, manual import exists.
- Bare bracketed number in the year range (`[1960]`) reads as a year, not
  a chapter (`reYear` in `tags.go`). Accepted: no series has reached
  chapter 1800+, and marked chapters (`Chapter 1960`) never take this
  path; revisit only if a real collision appears.
- Unresolved bare numbers (axis undecidable from the string) are kept in
  `Result.Bare` but feed nothing downstream except confidence — an
  ambiguous number conservatively covers no slot (`resolveBare`).
  Zero-padding to three or more digits stays visible as `BarePadded`:
  the review proposer reads it as the chapter convention and never
  reads a padded number as a volume.
