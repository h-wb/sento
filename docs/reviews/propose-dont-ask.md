# Propose, don't ask: review and series page

Written 2026-10-29 after the first agent exploratory pass (issues #20–#28)
and the open editions entries in [dogfooding.md](../dogfooding.md). The
pass filed nine usability issues. Fixing them one by one kept producing the
same kind of patch — a clearer message, another button — so this review
looks for what they share before building more.

## Diagnosis

The issues are different symptoms of two causes.

**1. The model is the interface.** The series page is a stack of edition
cards, each with Version settings, Search releases, a menu and a Collect
switch. A user who wants "Nausicaä, in English, every volume" has to
translate that into editions, collected versions and books, and every
quirk of the catalog becomes theirs to reconcile:

- MangaBaka files Viz's paperback box as an edition with one book,
  "volume 0"; the Deluxe hardcovers are not listed at all. Releases name
  the Deluxe edition and are correctly ruled out of the box set — and the
  user has to work out that the fix is to add an edition MangaBaka does not
  list (#22).
- A one-shot gets an automatic "Chapters" edition whose only chapter is
  paused by a rule meant for series that also collect volumes (#25).
- The domain guide already says what an edition *is* is read from its
  name. The page still asks the user to manage editions first and books
  second.

**2. The app knows the answer and asks an empty question.** Sento refuses
to act on a guess — right for automatic downloads — but it also refuses to
*propose* one, even where the evidence is in hand:

- The file name says "Kagurabachi 066"; the parser reads chapter 66. The
  file goes to review because matching only considers collected editions
  (`scan.go` `linksFor(…, monitored, …)`; the importer's `fileBooks` only
  considers the download's plan). The review dialog then opens empty — 57
  times (#24).
- Three releases say "Deluxe" and the series has no deluxe edition (#22).
- Every file of a download has been confirmed, but the job stays "needs
  review" with a stale "verified 0 of 1", and the only exit is Cancel
  (#23). Completion is only checked when the importer runs the job again.
- An all-books chapter search ticks all 136 releases (#27).

The [second edition-model review](edition-model-second-pass.md) asked for
review entries that carry "evidence, a proposed action, and its effect".
The single inbox landed; the proposals did not.

## Principles

1. **Never act on a guess; always propose one.** Every review item that
   can be resolved from evidence arrives with a proposed resolution, the
   evidence, and one-click accept. Similar items accept together.
   Automatic acquisition stays exactly as strict as it is.
2. **Lead with what you collect, not with editions.** The series page
   answers: what am I collecting, what is missing, what needs me. Editions
   are something you open, or that a proposal brings to you.
3. **Defaults follow the series as it is.** A series without a volume
   edition collects its chapters. A broad search starts from what is
   wanted.
4. **Close every loop.** When the last blocker clears, the item finishes
   by itself and its history says what happened.

## A. Review proposes

### A1. File proposals (#24)

A file waiting for review gets a proposal when its own name, read against
**every** edition of the series (collected or not), points at exactly one
edition and resolves to books in it:

- Edition: `editionCompatible` + `preferByPackaging`, then only editions
  that hold all the numbered books; if several remain, collected ones win;
  still several → no proposal.
- Books: `resolveBooks` on the file's own ranges; ambiguous numbers → no
  proposal.
- A file from a download whose name has no series title ("1 [hash].cbr"
  inside a Nausicaä torrent) borrows the series from the download: its
  bare number is read as a book of the download's type. The evidence line
  says so.

The proposal is computed when review is read, from the parse stored with
the file, so it follows today's editions (add the Deluxe edition and the
quarantined Deluxe files propose themselves). Nothing is written until the
user accepts.

```
┌ File review ────────────────────────────────────────────────┐
│ Kagurabachi 066 (2025) (Digital) (LuCaZ).cbz                │
│ Proposed: Chapters · chapter 66 — from the file name        │
│ [Accept]  [Review file]                                     │
└─────────────────────────────────────────────────────────────┘
  [Accept 57 proposed matches]   ← above the list, one action
```

"Review file" opens the mapping dialog pre-filled with the proposal.

### A2. Downloads close themselves (#23)

A job in `needs_review` is resolved when no file it delivered still awaits
review. That is checked whenever a file mapping is saved, not only when the
importer runs the job again. The job becomes `imported`, its error is
cleared, and History records what happened ("imported after review: 7
files confirmed by hand"). Books of its plan that no file fulfilled are
released and wanted again; they are named in the history entry.

### A3. Missing editions (#22) — next

Releases ruled out because they state an edition the series does not have
("deluxe") are evidence of a missing edition. Search results are not
stored today, so this needs a small record of such observations before it
can become a review proposal ("Releases exist for a Deluxe Edition this
series doesn't have — add it?" → Add edition pre-filled). Until then the
rejection reason says what differs and points to Add edition (done:
`73b3b9d`).

## B. Series page

### B1. Defaults (#25)

The automatic chapter edition is paused when the series collects volumes.
A series with no volume edition collects its chapters.

### B2. All-books search (#27)

An all-books search pre-selects the wanted books only, and offers a number
range like the book grid's "Select range". Downloading everything stays one
click away, never the default.

### B3. Collecting first

The page leads with one row per kind the series is collected as, edition
detail one level down:

```
Nausicaä of the Valley of the Wind
  Volumes   Standard Edition · en · 7 of 7 on disk          [Search]
  Chapters  not collected (you collect volumes)             [Collect]
  ─ Needs you: 1 download to review ───────────────────────────────
  Editions ▸ Standard Edition, Box Set Edition, Chapters    [Add]
```

This reshapes `EditionManager.tsx` and is the largest change; it is
sketched here and built after A, B1 and B2, which do not depend on it.

## Status

Built on 2026-10-29, on the branch of the exploratory pass:

| Part | Commit | Issue |
| --- | --- | --- |
| A1 file proposals, accept one or all, pre-filled mapping | `289fec0` | #24 |
| A2 held downloads close on their last confirmed file | `224300d` | #23 |
| B1 a series without volumes wants its chapters | `3084117` | #25 |
| B2 all-books search proposes only wanted books; Select range | `bedb1f3` | #27 |
| B3 Collecting summary; edition cards open on demand | `4b017fa` | — |
| A3 missing-edition proposals | not built | #22 |

B3 is the first step of the reshape sketched above: the summary leads and
editions are the detail, but each edition card is unchanged. What the
cards should still carry once the summary exists (Version settings and
Search releases on every card, the menu's three levels) is the next
question, best answered by a think-aloud session on the new page.

## Out of scope

Automatic acquisition rules, the facet vocabulary, and the
edition/version/book model itself. The model is sound
([collection-model.md](collection-model.md),
[edition-model-second-pass.md](edition-model-second-pass.md)); this is
about what the interface asks of the user.
