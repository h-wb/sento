# What a file says about itself

The database decides what a file is. Each archive Sento writes still
describes itself, for two readers: a comic reader (Komga, Kavita) that
should show it right, and Sento itself, which should adopt it back exactly
after a reinstall or a rescan, without guessing from its name (D17).

Only facts are written, never preferences: the release profile and the card
a file fills are choices you make and can change; what the file is does not
change.

## Where each fact goes

Standard ComicInfo fields first, where readers look for them; one
machine-readable line in `Notes` for the rest. No sidecar file and no
custom XML elements: a second source of truth drifts, and readers drop
elements they do not know.

| Fact | Field | Example |
| --- | --- | --- |
| Series | `Series`, `Web`; `Notes` `mangabaka:` | `Nausicaä of the Valley of the Wind` |
| Edition, a numbering | `Format`: what its name says it is, as `parse.ReadEdition` reads it; nothing for a standard edition. `Notes` `edition:` and, for a catalog edition, `mb-edition:` | `Deluxe`, `Box Set`, `Full Color` |
| Language | `LanguageISO` | `en` |
| Book | `Volume`/`Number` for one book, `Title` for several; `Notes` `ledger:`, `books:`, `work:` | `Volumes 1–3` |
| Published material | `Publisher`: the edition's publisher | `VIZ Media` |
| Fan translation | `Translator`: the group; no `Publisher` | `Some Scans` |
| Group | `ScanInformation` | `1r0n` |
| Variant facts | `Notes` `flags:` (the release's flags, as profiles match them) | `flags:colored,digital` |
| The release | `Notes` `release:`, `via:`, `site:` | `release:"Nausicaa Deluxe v01 (Digital)"` |

`Series` stays the series' title for every edition, so a reader keeps one
series; folders keep editions apart (spec §11.2).

**Published or fan-made.** A file is a fan translation when it came from an
extension source (a scanlator delivered it) and the release does not name a
publisher or platform (the `official` flag). Everything else is taken to be
a copy of the edition it was matched to: a scan or rip of the published
volume is published material. Sento has no evidence of who colored a
colored release, so `Colorist` is never written; `flags:colored` says only
that it is colored.

## The `Notes` line

```
sento:v1 mangabaka:15456 edition:"Deluxe Edition" ledger:volume books:"1 2" release:"Nausicaa Deluxe v01-02 (Digital) (1r0n)" via:torrent site:"Nyaa" flags:deluxe,digital
```

`sento:v1` comes first. Then space-separated `key:value` pairs. A value
with a space or a quote is a Go-quoted string. Lists are comma-separated
(`work:`, `flags:`), except `books:`, whose numbers are space-separated
because a number never holds a space. Unknown keys are ignored, so
the line can grow without a version bump.

| Key | Value |
| --- | --- |
| `mangabaka` | MangaBaka series id |
| `edition` | the edition's name |
| `mb-edition` | MangaBaka's edition id; absent for a local edition |
| `ledger` | `volume` or `chapter` |
| `books` | the book numbers the file holds, as written in file names |
| `work` | MangaBaka work ids of those books, when known |
| `release` | the release name it was downloaded as |
| `via` | `torrent` or `source` |
| `site` | the indexer or source it came from |
| `flags` | the release's flags that hold |

## Reading it back

A rescan opens a CBZ's ComicInfo. When its `Notes` line is Sento's and names
this series, the file is linked from it: the edition by `mb-edition` (or, for
a local edition, its name), language and ledger; the books by number, `work`
telling apart two books that share one. Its facts are the release's: group,
language, flags, provider and site, so a profile judges it as it judged the
download. When any of that does not resolve (an edition renamed or merged
since), the file is matched by its name, as any other.

Not yet: identifying the series of a folder from this line (spec §11.4,
step 1), and reading ComicInfo that another tool wrote.
