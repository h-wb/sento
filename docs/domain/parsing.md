# The parser & title matching

> Tour session 1 — **in progress**. Covers `internal/parse` and
> `internal/match`. Spec: [parser.md](../spec/parser.md).
> Progress and next steps at the bottom.

## Role

Every grab decision starts from an unstructured release/file/chapter name.
`parse` turns one name string into structured facts; `match` resolves a
parsed title guess back to a library series. The parser is the
highest-risk component in Sento (parser.md's own words): the input space
is two decades of inconsistent human naming conventions.

Two design commitments shape everything:

- **Purity.** `Parse(name, channel)` does no I/O — which is what makes
  the regression corpus possible (`testdata/corpus/*.jsonl`, one case per
  line, partial expectations, CI gates on 100%).
- **Explainability.** Every rule that fires appends a `Step` to
  `Result.Trace` — "why did it parse that way?" is always answerable.

The `Channel` (`torrent` / `source` / `file`, result.go) is the one piece
of context: it biases interpretation of ambiguous numbers but never
changes the mechanics.

## File layout (follows the pipeline)

| File | Stage |
|---|---|
| `preclean.go` | canonicalize the string before any pattern runs |
| `segment.go` | split into `[...]`/`(...)`/`{...}` segments + free text |
| `tags.go` | classify segments against vocabulary tables |
| `numbers.go` | marker-bound and bare number extraction |
| `parse.go` | entry points + orchestration + confidence |
| `anchor.go` | title-aware entry point (`ParseAnchored`) |
| `num.go` | the number domain: `Num`, `Range`, merging |
| `result.go` | `Result`, `Channel`, hint structs, trace |

## The number domain (`num.go`)

`Num` is a **fixed-point hundredths** integer: chapter 34.5 = `Num(3450)`.
No floats anywhere (exact comparisons, exact DB round-trips); the only
float lives at the JSON boundary, immediately rounded. Alpha sub-chapters
follow the Mihon convention: `4a` → 4.1 (`alphaSuffixNum`). `Range{From,
To}` is inclusive; a single number is `From == To`. Custom JSON renders
`3450` as `34.5` and ranges as `[from,to]` so corpus files stay readable.

## preClean (`preclean.go`)

Strips a *recognized* archive extension (only `archiveExts` members — a
`.5` in "v1.5" survives), then canonicalizes: `_` → space, full-width →
ASCII (`width.Fold`), Unicode NFKC (visually-identical characters become
byte-identical), whitespace collapsed to single spaces. Every later stage
assumes this boring representation.

## Pipeline overview (detail TBD as the tour progresses)

1. **Segmentation** — delimited segments + free text, order preserved.
2. **Segment classification** (`tags.go`) — vocabulary tables for
   language, content type, publisher (→ `Edition.Official`), quality,
   year(-range); hits fold into the Result, first-wins/OR-merge.
   No-hit, no-number segments become group candidates.
3. **Number extraction** (`numbers.go`) — a priority cascade over a
   mutable buffer, consuming by blanking: version suffix `34v2` first,
   then JP markers (`第N巻`/`第N話`), volume markers, chapter markers, the
   trailing-dash convention ("Berserk - 375"), finally bare numbers with
   axis deferred. Channel priors (`resolveBare`) then claim bare numbers
   (source channel / decimals / ≥1000 / far-above-volumes → chapters).
4. **Free text** — season, language words (two tiers: unambiguous words
   anywhere; bare codes like `FR` only standalone-UPPERCASE), edition
   phrases, keywords. Consumed spans pull `titleEnd` left; the title is
   what remains. ` / ` and ` | ` split dual titles.
5. **Closing moves** — group election (last unrecognized number-free
   segment, leading one as fallback), implicit-pack rule (torrent + no
   numbers + year range → whole-series pack, flagged `CompleteInferred`:
   the search funnel shows it but rejects it as `rejected.pack_unverified`), confidence scoring.

## Anchored parsing (`anchor.go`)

Sento usually knows which series it's searching: `ParseAnchored` finds
the longest known title/alias in the name, blanks it, then parses —
numbers inside titles ("Mob Psycho 100") can never leak into slots.
`foldRunes` folds case/diacritics **1:1 rune-for-rune** so offsets stay
valid for blanking (that's why it isn't `match.Normalize`).
`TitleLeftover` = unexplained words around the anchor — the signal for
"a different work that merely contains the title" ("WANTED! Eiichiro Oda
Before One Piece").

## `internal/match`

The reverse direction: which library series does this title guess belong
to? `Normalize` folds aggressively (NFKC, diacritics, punctuation,
`&`→"and", `×`→"x", apostrophes deleted). `Index` holds every
title/alias; exact hits score 1.0, otherwise order-tolerant token-set
similarity with Jaro-Winkler per token, averaged over the longer title
(extra tokens dilute). Thresholds: ≥0.90 auto-accept, ≥0.75 shown for
manual confirm. Ties break to the longest title (containment rule).

## Session log

Covered in detail so far:

- [x] Contract, channels, corpus harness
- [x] `num.go` line-by-line (fixed point, receivers, JSON interfaces)
- [x] `preclean.go` line-by-line (Go syntax: if-init, slicing, map-as-set)
- [x] Split `parse.go` along its banners → `preclean.go`, `segment.go`,
      `numbers.go` (corpus green before and after)
- [x] `segment.go` line-by-line (`strings.Builder`, closures, comma-ok,
      byte-vs-rune indexing; unbalanced brackets degrade to free text,
      nesting unsupported by design — contract documented on `segments`
      and pinned by `segment_test.go`)
- [x] Table-driven stage tests: `preclean_test.go`, `segment_test.go`
      (pin the documented contracts; corpus remains the pipeline-level
      truth)
- [x] `tags.go` deep pass — vocab tables (map=exact/whole-segment,
      slice=containment: the data structure *is* the match strategy);
      `segmentHints` return bag (`*int`/`*[2]int` as optionals,
      `recognized` flag); three-stage `classifySegment` (anchored regex
      w/ early return → whole-segment lookup → accumulating token/phrase
      scan); noise tags recognized-but-hintless on purpose; `fold` merge
      policy *per field* — first-wins (language), last-wins
      (contentType), max (version), OR (editions), and the order-
      dependent `ongoing` that clears `complete`. Bug: `full` token
      fires inside "full color" phrase → spurious `complete=true`;
      refactor idea (one consuming `{phrase,apply}` table) logged in
      improvements.md. Reuse check: tables are NOT duplicated elsewhere
      today; `profile.CondLanguage` operates on parsed facts, not raw
      names — correct layer separation, don't merge.
- [ ] `numbers.go` — the cascade in detail, regex group plumbing
- [x] `numbers.go` deep pass — regex capture-group plumbing
      (`FindAllStringSubmatchIndex` returns flat `[]int` index pairs;
      `m[2n]/m[2n+1]` = group n, `-1` = didn't participate); alternation
      means from-number lives in whichever of {1,3,5} fired → `group` /
      variadic `firstGroup` accessors. Consume-by-blanking engine: a
      `[]byte` copy blanked in place (length-preserving, keeps indices
      valid), priority cascade version→JP→vol→ch→dash→bare where order
      IS the correctness argument (version-first keeps `34` from `34v2`).
      `cleanBoundary` gates bare numbers (reject letter/dot-glued).
      Smell: one-element JP-vol loop (logged).
- [x] `parse.go` orchestration deep pass — `parseInto` execution order:
      segment loop sorts each bracket into three fates (recognized /
      number-bearing / group-candidate, recorded in `segInfo`); free-run
      concatenation with `freePos` pinned to first run; the **`titleEnd`
      cursor** (title = front text surviving after every recognizable
      token drags the cursor left; season/keywords/numbers cut, editions
      don't); `resolveBare` channel priors line-by-line (source / decimal
      / ≥1000 / far-above-vols → chapter, else stays bare — never
      guessed as volume); anchored-vs-blind title fork + `splitDualTitle`;
      group election (trailing wins, leading fallback, only non-recognized
      number-free segments eligible); implicit-pack; `scoreConfidence`
      dock list. Full end-to-end trace recorded. Go: cap'd `make`, slice
      spread `...`, `switch{}` = switch-true, `&&` > `||` precedence,
      `max` builtin, blank-don't-delete via `strings.Repeat`.
- [x] `anchor.go` deep pass — `ParseAnchored` finds the longest known
      title via `findTokenSeq` (whole-word, in-order, whitespace-tolerant
      rune matching), blanks its span in the original, then runs the
      normal pipeline → numbers-in-titles ("Mob Psycho 100 v03") become
      structurally impossible, not patched. Misses degrade to blind
      parse. `foldRunes` is the THIRD normalizer and the reason is its
      contract: rune-for-rune length-preserving so the located span stays
      valid against the original (vs `preClean` canonicalize /
      `match.Normalize` compare — same idea, three contracts).
      `TitleLeftover` via `leftoverWords`: drop edition stopwords + chunks
      that fuzzy-match another alias (`titleSimilar`, integer ≥75%
      overlap) → only genuinely-unexplained words survive (the "different
      work containing the title" signal). Full Mob-Psycho trace recorded.
- [x] Applied parse improvements (session 1): fixed the `full`/"full
      color" complete bug, rewrote `mergeRanges` (sort + transitive
      merge), hoisted `rePureRange`, unwrapped the JP-vol loop, corrected
      parser.md §2 drift. Added `num_test.go` (`parseNum`, `mergeRanges`)
      and a corpus regression lock. Full suite green. Remaining lead: the
      `classifySegment` consuming-scanner refactor (improvements.md).
- [x] Finding A applied: unified edition/quality vocab into shared
      `scanTags` (one source of truth for both bracketed segments and free
      text), switched phrase matching to word-boundary `indexWord` (also
      kills raw-in-drawback FPs). Locked by `tags_test.go`. Residual blind-
      parse title-collision FP (`Raw Hero`) documented; anchored path
      correct. B/C/D still pending.
- [x] Finding B applied: extra/omake/special now trim the title
      (`titleEnd`) when preceded by title text, guarded by `i > 0` so
      title-leading markers (`Special A`) survive. Two corpus locks added.
- [x] Findings C & D documented (no behavior change): bracketed
      year/chapter collision noted at `reYear` + parser.md §9; bare-number
      coverage-inertness commented in `resolveBare` + parser.md §9.
- [x] Live-harvest corpus expansion: built `cmd/harvest` (build-tagged)
      to pull real chapter names off the running sento-host sidecar
      (~110 sources / ~14 langs / 497 records), ran them through the real
      parser, and compared against each source's own `number` field. Found
      + fixed a real bug (bare alpha sub-chapters `1150a` → nothing) and a
      real gap (CJK markers require `第`), and added 12 real multilingual
      corpus cases (75 → 87). Most names parsed correctly — the parser is
      robust; the wins were at the margins real data exposes.
- [x] `match.go` deep pass — Normalize (3rd folder, compare-contract:
      apostrophe-delete, `&`→and, `×`→x), Index (exact map + flat entries:
      map-vs-slice = match strategy again), Lookup (exact→fuzzy→dedup
      best-per-series, containment tie-break), Similarity (token-set,
      greedy one-to-one assignment, dilution ÷longer), Jaro-Winkler math
      (prefix bonus; byte-level so CJK fuzzy is approximate but exact-map
      covers it). Worked example recorded.
- [x] `match` validation via `cmd/matchcheck` (1148 real titles): self-
      match flawless, order/romanization tolerance proven, ~0.3% inherent
      fuzzy false-positives documented (logged). Locked 3 properties in
      `match_test.go::TestSimilarityProperties`.

**Session 1 complete.** internal/parse + internal/match fully toured,
documented, hardened (3 bugs fixed, Findings A–D, 2 harvest-driven fixes),
corpus 75→87, new tests, two build-tagged harvest tools.

Improvement leads from this session are in
[improvements.md](../improvements.md).
