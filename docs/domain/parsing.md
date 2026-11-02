# The parser & title matching

Release parsing and title matching live in `internal/parse` and
`internal/match`. The full parser contract is in [parser.md](../spec/parser.md).

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

## Pipeline overview

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
   the chapter way (source channel / decimals / ≥1000 / far above marker
   volumes / zero-padded to three or more digits on the torrent channel);
   only digits before the decimal point count toward padding (`01.0` is
   not three-digit padding, `001.0` is). The padding of the ones that stay
   unresolved is kept as a fact
   (`BarePadded`) for whoever resolves the axis.
4. **Free text** — season, language words (two tiers: unambiguous words
   anywhere; bare codes like `FR` only standalone-UPPERCASE), edition
   phrases, keywords. Consumed spans pull `titleEnd` left; the title is
   what remains. ` / ` and ` | ` split dual titles.
5. **Closing moves** — group election (last unrecognized number-free
   segment, with curly-brace notes used only when there is no other trailing
   candidate, then a leading candidate as fallback), implicit-pack rule (torrent + no
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
