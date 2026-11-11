# Naming, folders and names in readers

Where a book's file goes, what it is called, and which series a reader
shows it under are all templates in Settings → Media. New imports follow
them at once. Files already in the library change only when they are
[organized](#organizing-existing-files).

A **copy** below is one collected version of an edition: the card on the
series page. See [editions](editions.md) for what those are.

## Where a file lands

```
<library root>/<top folder>/<copy's folder>/<book folder>/<file name>
```

| Part | Setting | Default |
| --- | --- | --- |
| Top folder | `volumesRoot`, `chaptersRoot` | none |
| Copy's folder | `seriesFolder`, rendered for the copy | `{Series Title} [mangabaka-{seriesId}]` |
| Book folder | `bookFolder`, one level per `/` | `{edition} [{lang}] ({version})` |
| File name | `volumeFile` or `chapterFile` | `{Series Title} v{volume:00} [{edition}][{lang}].cbz`, `{Series Title} c{chapter:000} [{group}][{lang}].cbz` |

With the defaults a volume lands at
`ONE PIECE [mangabaka-377]/Standard Edition [en]/ONE PIECE v112 [Standard Edition][en].cbz`.

**Top folders** put the type above the series: `Volumes/<series>/…` and
`Chapters/<series>/…`. A series gets a separate chapter folder
(`series.chapter_path`) only when the two differ; both empty keeps one
folder per series. The `{type}` token puts the split inside the series
instead (`<series>/Volumes/…`). Scans cover both folders, and storage
availability is judged per folder, so an unreachable chapter tree does not
block volumes.

**The copy's folder** is the series folder template rendered for that
copy, with its edition, its version and its name in readers:

- A render equal to the series' own is the series folder. This is every
  copy under the default template.
- A different render is a sibling folder beside it. Copies that render the
  same folder share it.
- An edition's default version stays in the edition's folder.
- Where the render starts with the series' own, the folder is the series'
  current folder name plus what the copy adds, so a folder name the series
  kept or was adopted with survives.
- A series adopted from outside the library root stays in its one folder.

The series' own folder keeps the review area (`review/`). Scans walk it
and every copy's folder.

**The book folder** nests inside the copy's folder. An empty template puts
books directly in it.

**File names.** A file holding several books is named for its range
(`c001-008`). When a different book already holds the path, the content
fingerprint is added (` [<sha-256>]`); an occupied path is never
overwritten. "Add the content fingerprint to file names" adds it always.
With "Rename imported files" off, a file keeps the name it arrived with.

A series keeps its own folder name when settings change (the name is only
rendered when the folder is first created); a change of top folders
re-roots it. Moving the library root itself is a
[separate operation](../installation.md#moving-the-library).

## Tokens

Tokens are written `{name}`, in any case. Every token works in every path
template. The list lives in one place, `mediafile.Tokens`: it renders the
templates, backs the token table on the Media page
(`GET /settings/media/schema`) and names a misspelled token.

| Token | Renders |
| --- | --- |
| `{Series Title}`, `{Series TitleSlug}` | `ONE PIECE`, `one-piece` |
| `{seriesId}` | MangaBaka id |
| `{Reader Series}` | the copy's [series name in readers](#names-in-readers) |
| `{volume}`, `{chapter}` | the number; `{volume:00}` pads its whole part, `1102.5` keeps its fraction |
| `{edition}`, `{lang}`, `{publisher}`, `{medium}` | the edition's name, language, publisher and medium |
| `{type}` | `Volumes` or `Chapters` |
| `{version}` | the collected version's name; empty for an edition's default |
| `{group}`, `{isbn}`, `{year}` | release group, ISBN, the series' year |
| `{editionId}`, `{versionId}` | internal ids that never change |

A wrapper left empty disappears: `[{group}]` vanishes when there is no
group, and so does ` ({version})` for a default version. Values lose the
characters a path cannot hold.

## What is refused

Saving is refused, and the preview says why under the field, when:

- a template uses a name that is not a token;
- the series folder template is empty, contains a path separator, or
  renders empty for a series on its own (every series would share it);
- the volume file template has no `{volume}`, or the chapter one no
  `{chapter}` (files would overwrite each other);
- the book folder leaves the series folder, or a top folder is not a plain
  folder name;
- a reader template uses `{volume}`, `{chapter}`, `{group}`, `{isbn}` (it
  holds for every book of a copy), uses `{Reader Series}` (the name it
  sets), or renders empty.

## Names in readers

A reader files a book under its ComicInfo `Series`, groups series by
`SeriesGroup`, or goes by folders. Both fields are templates
(`mediafile.PresentedFields`), rendered per copy:

| Field | Default | Result |
| --- | --- | --- |
| `Series` | `{Series Title} {version}` | `ONE PIECE`, and `ONE PIECE Colored` for a second card collected as Colored |
| `SeriesGroup` | `{Series Title}` | `ONE PIECE` for every copy |

An edition can set its own `Series` template in Edit edition → In readers:
the library's, the series and edition name
(`{Series Title} {edition} {version}`), or a custom one. A template that
renders empty for a copy falls back to the default, so a copy always has a
name. The card shows the name when it differs from the series title.

These fields are presentation, written exactly and never read back for
identity ([metadata](metadata.md), D18).

### Setting up for a reader

- **A reader that files by ComicInfo** (Kavita) keeps copies apart by
  `Series`. The default already separates the versions of an edition. To
  show an edition as its own series, give it its own name in Edit edition.
- **A reader that files by folders** (Komga) shows one series per folder.
  The layout "One series folder per edition and card"
  (`{Series Title} [mangabaka-{seriesId}] [{edition}][{lang}] ({version})`,
  flat book folder) gives every copy its own.
- **Both at once**: "One folder per name in readers"
  (`{Reader Series} [mangabaka-{seriesId}]`) makes the folders follow the
  names, so both kinds of reader show the same series.

## Organizing existing files

Organize previews, then applies, what the current settings would change
in existing files: move and rename to the templated path, and rewrite
embedded metadata where the merge would change it.

- **Where**: one series (series → Files → Organize files), several
  (Library table → select → Organize files), or the whole library (Media →
  Organize existing files). Saving Media settings that change naming or
  metadata offers the preview, and so does saving an edition's new name in
  readers.
- **Preview**: each file's move, whether its metadata is rewritten, and
  why a file is left alone (not matched to a book yet, not on disk). The
  library preview lists the series it would change.
- **Apply**: always one background run, one series at a time, with
  progress and per-series failures. Closing the dialog does not stop it.
  A run cut short is resumed by starting it again; files already in place
  drop out of the plan. System → Tasks lists it as *Organize Library
  Files* without a Run button: files move only from a preview.
- **One run converges.** A file moves after the one sitting on its
  destination, and two files on each other's names are settled in a second
  step. The moved count is the files that actually moved. A file on its
  fingerprinted name because a different book keeps the plain one is in
  place and is not planned again.
- **Untouched**: fingerprints, coverage and pins. A seeding payload
  hardlinked to an old library file is never modified. A file whose
  books are all supplied by another file moves to `review/`. Emptied folders are removed.

Planning loads a series' catalog rows in bulk, so its query count is fixed
whatever the file count. Moving and re-tagging reuse a file's stored
fingerprint and only hash its pages when none is recorded.

## API and code

| Route | Purpose |
| --- | --- |
| `GET`, `PUT /settings/media` | the settings |
| `POST /settings/media/preview` | sample paths, reader names, and per-field errors for unsaved settings |
| `GET /settings/media/schema` | tokens and reader fields |
| `POST /series/{id}/reader-names` | each card's names in readers, optionally with an edition's unsaved templates |
| `GET /series/{id}/files/organize` | one series' preview |
| `GET /library/organize?series=1,2` | the preview across series (all when omitted) |
| `POST /library/organize` | start the run (`seriesIds`, `rename`, `retag`) |
| `GET /library/organize/status` | its progress and result |

| Code | Holds |
| --- | --- |
| `internal/mediafile/tokens.go`, `naming.go` | the token list and the renderer |
| `internal/mediafile/presented.go` | the reader fields |
| `internal/service/layout.go` | series and copy folders, token values |
| `internal/service/placement.go` | one series' folders, file names and ComicInfo for a pass |
| `internal/service/mediasettings.go`, `reader_fields.go` | settings, validation, previews |
| `internal/service/organize.go`, `organize_library.go` | the plan, the run and its status |
