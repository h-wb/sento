# Edition rules: what an edition is, measured

Investigated 2026-10-29 against live data: MangaBaka's edition list (all
60 names) and the English collections of 30 well-known series, Nyaa's
English-translated literature category for the same series, and Sento's
matcher (`internal/service/edition_fit.go`) run on the release names
found. Question: is "Deluxe" an edition, should omnibus or box set fold
into it, and what rules between editions and profiles hold in general —
not just for Nausicaä.

## What the data says

**MangaBaka edition names carry no meaning.** The 60 names ("Deluxe",
"Omnibus", "3-in-1", "VIZBIG", "Perfect", "Colossal", "Collector's",
"Eternal", "Fullmetal", "Black", "Definitive", "Box Set", …) are labels
from publisher feeds; their descriptions read "Added by processing
[VIZ]". What an edition is has to be measured, not looked up.

**Measured, most of them are the same thing: a regrouping.** English
collections, books per edition against the standard run:

| Series | Standard | Regrouped editions |
| --- | --- | --- |
| Vinland Saga | 15 | Deluxe 8 |
| Battle Angel Alita | 9 | Deluxe 5 |
| Nausicaä | 7 | (Deluxe 2, unlisted) |
| One Piece | 113 | Omnibus 37 |
| Attack on Titan | 34 | Omnibus 12, Colossal 7 |
| Dragon Ball | 26 | 3-in-1 14, VIZBIG 9 |
| Fullmetal Alchemist | 27 | 3-in-1 9, Fullmetal 18 |
| 20th Century Boys | 22 | Perfect 11 |
| Death Note | 13 | Black 6, All-in-One 1 |
| Vagabond | (37 JP) | VIZBIG 12, Definitive 8 |
| Sailor Moon | 12 | Eternal 10, Collection 12 |

"Deluxe" is one of about fifteen names for "the story cut into fewer,
bigger books". It differs from "Omnibus" only in the marketing.

**Box Set is not a regrouping.** Its count is boxes (One Piece 5, Attack
on Titan 7, most series 1), each holding standard volumes: the same books
in another wrapper. It stays a MangaBaka edition (it is one in the
catalog), but no release is ever "a box set layout".

**Color is two different things.** Bleach and One Piece "Digital Colored
Comics" keep the standard numbering (Bleach v01-74): same books, colored.
Dragon Ball's "Full Color Edition" has its own numbering: another edition.

**Release names and catalog names disagree.** Kodansha's English Vinland
Saga is the 2-in-1 hardcovers; MangaBaka files them as the *Standard*
Edition (15), releases call them "Omnibus Edition" (v01-14). Dragon Ball
"Kanzenban" (Japanese for perfect edition, 34 volumes) matches no
MangaBaka name. Some match exactly ("Perfect Edition", "Eternal Edition",
"Fullmetal Edition", "Collector's Edition").

## How today's rules fare

The edition matcher of the time on the real names (probe, not a fixture):

| Release | Edition | Today | Right |
| --- | --- | --- | --- |
| Vinland Saga v14 (Omnibus Edition) | Standard (15) | ruled out | fits |
| same | Deluxe (8) | ruled out | no: v14 > 8 |
| Dragon Ball V13 Kanzenban | Standard | **fits** | no: another numbering |
| Dragon Ball Full Color Vols 01-16 | Full Color | fits | fits |
| Bleach Digital Colored Comics v01-74 | Standard | ruled out | fits (a variant) |
| 20th Century Boys (The Perfect Edition) | Perfect | fits | fits |
| Sailor Moon Eternal Edition | Standard | ruled out | ruled out |

Two root causes:

1. **Words decide, hard.** "omnibus", "deluxe" and "colored" are content
   facets that must agree exactly, so a synonym (Omnibus for a catalog
   Deluxe, or for a Standard that is itself 2-in-1) is a hard rejection,
   while a word outside the vocabulary (Kanzenban) is silently ignored.
2. **Numbers are not evidence.** A release naming v14 is never checked
   against an edition that has 8 books.

The "Add edition" offer (`otherEdition`) inherits both: it would offer
"Add Omnibus Edition" on Vinland Saga, and nothing for Kanzenban.

## Proposed rules

**R1. An edition is a numbering.** Two copies of "volume 3" are the same
book when they hold the same pages. What changes the numbering makes an
edition: the original run, or a regrouping (any of the names above).
What does not — medium, box, hardcover, scan or digital, colorization of
the same volumes, the group — is a variant within an edition, and
choosing between variants is the profile's job, not the edition's.

**R2. Release words are evidence, not verdicts.** One vocabulary of
regrouping words (omnibus, deluxe, N-in-1, VIZBIG, perfect, kanzenban,
colossal, collector's, eternal, definitive, master, …, plus any
"<X> Edition" phrase) says "this release is a regrouping". Evidence, in
order of strength:
1. a verified source listing, or the release naming one of the series'
   editions ("Perfect Edition", "VIZBIG") → that edition;
2. volume numbers: a release naming v14 does not fit an edition of 8;
3. regrouped vs original: a regrouping word points at the regrouped
   editions, none at the original layout.

A release fits an edition when nothing contradicts it and the evidence
picks it alone. When the evidence leaves several, or contradicts only
weakly (a word, not a number), it is `edition_ambiguous`: never grabbed
automatically, downloadable after review from the edition searched — so
Vinland "Omnibus" releases can be taken into the Standard edition by a
person, and are never auto-grabbed into the wrong one. Language and
numbers stay hard.

**R3. A release can name an edition the series lacks.** A regrouping the
series has no edition for (Kanzenban on Dragon Ball; Deluxe on Nausicaä)
is offered in the search: "Add <name>", the name in the release's own
words, the language from the release, and the volume count from the
highest number those releases name (asked only when none does).

**R4. Box set stays a catalog edition**, holding boxes of the original
numbering: whole-run packs and original-numbered releases are its
content. It never competes as a layout.

**R5. Color follows R1.** Colored releases of the original numbering are a
variant of the standard edition: a profile prefers or refuses them with
the existing `colored` custom-format flag. A series edition whose name
says color and that has its own numbering (Dragon Ball Full Color) takes
the releases that name it (R2.1).

**R6. Preferences between editions are not scores.** Which edition you
collect is a choice — collect the omnibus, the standard, or both as two
editions. Within an edition, profiles already rank variants with custom
formats (flags: digital, scan, colored, official, …). No new setting.

## What changes in the code

- `edition_fit.go`: omnibus and deluxe stop being separate content facets;
  one "regrouped" notion with the wider vocabulary; colored moves to
  packaging (a variant) except where an edition's own name claims it.
  Volume numbers against the edition's book count join the evidence.
  Mismatches by word become `edition_ambiguous` (soft), by number or
  language stay hard.
- `otherEdition`: the name comes from the release's own words, the count
  from its numbers.
- Parser: the regrouping vocabulary grows (kanzenban, vizbig, perfect,
  colossal, collector's, eternal, definitive, N-in-1, "<X> Edition").
- No new setting, no new screen.

Each row of the probe table above becomes a test with the real name.

## Status

Built 2026-10-29 (`TestEditionRules`, `TestEditionRulesOnRealNames`). One
refinement from the first run: numbers past an edition's last book only
say no when another edition of the series reaches them — the test catalog,
like MangaBaka for some series, lists fewer books than exist. Checked live
on Vinland Saga: its "Omnibus Edition" releases, ruled out before, are now
offered for its Standard Edition after review, with "Add Omnibus Edition"
beside them.

Then refactored so the design matches the rules:

- **One vocabulary**: `parse.ReadEdition` reads release names and edition
  names alike into a `parse.EditionClaim` (regroupings named, a name of
  its own, color, box, and the edition's name as written). Every parse
  result carries its claim; the parser's edition flags for custom formats
  come from the same table.
- **The series is the context**: a `shelf` holds a series' editions with
  their claims read once. `shelf.judge(release, edition, evidence)` is the
  one decision; `fits`, `prefer` and `offer` build on it. Nothing about
  sibling editions is stored on an edition.
- **One soft code**: unsure and ambiguous are both `rejected.edition_ambiguous`,
  the reason in its text.
