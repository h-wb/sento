# Editions, books, collected versions, and files

Sento distinguishes what a book **is**, which releases you want to **keep**, and which files you actually **have**. An English omnibus, a French standard volume, and two release groups' copies of an English digital volume are different cases.

| Entity | Meaning | Identity |
| --- | --- | --- |
| Series | The manga represented in your library | Local series ID; MangaBaka reference |
| Edition | What is collected: MangaBaka's edition for one series, language and type, or a local one | Local edition ID; MangaBaka edition ID |
| Collection | A MangaBaka collection: one product line of an edition (publisher, medium, format) | MangaBaka collection ID |
| Book | A volume, chapter, extra, special, or revision within an edition | Stable local book ID |
| Work | A MangaBaka work: one copy of a book in one collection, with its own ISBN and date | MangaBaka work ID |
| Collected version | An independently collected set of releases of one edition | Version ID |
| Profile | Reusable release eligibility, scoring, and upgrade policy | Profile ID |
| Queue item | A provider download with a frozen coverage plan and decision snapshot | Queue ID; provider and offer key |
| File / asset | One physical archive with observed facts and a content hash | File ID and path; SHA-256 identifies bytes |
| File contents | Confirmed books inside an archive, independent of edition rules | File ID + book ID |
| Fulfillment | Whether a present, verified asset meets a version’s current rules | Derived; explicit selections and pins use file ID + version ID + book ID |

```mermaid
flowchart LR
  Series --> Edition
  Edition --> Collection
  Edition --> Book
  Collection --> Work
  Book --> Work
  Edition --> Version
  Profile --> Version
  Release[Release evaluation] --> Job[Download job]
  Version --> Release
  Book --> Release
  Job --> Asset[File asset]
  Asset --> Contents[Verified file contents]
  Book --> Contents
  Contents --> Fulfillment
  Version --> Fulfillment
```

## Catalog identity and local editions

The catalog is mirrored under MangaBaka's own names and fields (`collections`, `works`, and its edition ID and name on `editions`), so a change on their side lands in one place. MangaBaka files a series' books as **collections** — a language, a publisher, an edition, a type, a format and a medium — each holding **works**. The digital and the paperback run of a series are two collections with separate works, ISBNs and dates, and the same edition.

An **edition** here is that MangaBaka edition for one series, language and type. It groups the collections that differ only in medium or publisher, and a **book** is the works of those collections that carry the same number, kind and revision: the digital and the paperback volume 3 are two works and one book, wanted once. A volume that exists in only one medium is a book all the same. The preferred copy (digital, then paperback, then hardcover) supplies a book's ISBN and page count and the edition's publisher and medium; a book's date is the earliest any copy came out. Which kind of release is downloaded is the profile's business, not this order's.

Refresh matches collections and works by their MangaBaka IDs. A new collection joins the series' edition of the same language, type and MangaBaka edition, or starts one. Editions that predate grouping are folded together by a refresh when only one of them holds files, pending downloads or additional versions; when both do, **Merge into** does it by hand. Grouping also merges two publishers' "Standard Edition" that are different translations: **Separate into its own edition** takes a collection out, and a separated edition is never grouped again. When MangaBaka files a collection under another edition, the edition here follows.

Names, numbers, dates, publishers, and ISBNs are facts, not primary keys. A book keeps its local ID whatever happens to the works behind it. Sento does not treat books of different editions as equivalent file coverage.

A local edition stands in for a publication the catalog does not list yet. It is never adopted silently: when a refresh brings a catalog edition of the same type and language (and the same MangaBaka edition, when the local one was named from the catalog's list) with at least half of the local books matching, **Review** offers to link the two (best candidate only). "Not the same edition" records the answer so the offer is not repeated; the manual **Link to catalog** stays available.

Linking turns the local edition into that catalog edition. Its ID, files, collected versions and hand-edited fields survive; it takes the catalog edition's collections and works, its name, publisher and medium, without sending files back to review. Books are matched by number (by label when unnumbered), kind, revision and ISBN; catalog books it lacked are added at once and unmatched local books remain. Verified source mappings move over and the separate catalog row is deleted, so the edition exists once. Linking refuses pending downloads, a catalog edition that already owns files, and one with additional collected versions.

An edition is either local (`custom`, `sync_state = local`) or catalog-backed (`linked`, or `stale` when its entry disappears); a linked edition is a catalog edition whoever created it.

Metadata edits override only changed fields. Other fields continue receiving catalog updates. **Use catalog values** clears overrides and refreshes metadata. There is no list of traits. What an edition is, is what its names say — MangaBaka's name for it and the one it is shown under — read with the vocabulary release names are read with (omnibus and N-in-1, deluxe, coloured, box set). Renaming an edition cannot take away what the catalog says it is, and can add what the catalog leaves out: a "Box Set Edition" whose books are the deluxe ones is renamed "Deluxe Box Set Edition". Add edition offers MangaBaka's edition names, so a local edition can carry the name the catalog will use.

A collection or work the catalog no longer lists becomes `stale`; an edition is stale when all its collections are, a book when all its works are. Identity and files are retained. A retired linked book is retained so the next refresh cannot silently recreate it. A stale parent edition suspends automatic acquisition even when its books are still linked. Continue locally explicitly detaches the upstream references while preserving books, files, and edition preferences. Stale and retired books are excluded from automatic acquisition.

## Numbers and chapter discovery

Volumes and chapters are collected independently: a release or a file is one kind or the other, never both (`ledgerOf`). Only volume numbers make a volume and only chapter numbers make chapters; with both, a single chapter ("Vol.5 Ch.34") is a chapter that notes its volume, and several chapters ("v01 c001-008") are a volume that lists what is inside it. A volume is never used to fill chapters, or the reverse.

A book has a display label and an optional number. Numbers use integer hundredths: volume 0 is `0`, chapter 12.5 is `1250`. `number_known` distinguishes actual zero from an unnumbered special. Main, extra, and other books can share a number without sharing ownership.

New local chapter editions start with **explicit books**. Numbering modes are:

- `manual`: add individual books or accept source observations.
- `catalog`: use published MangaBaka memberships.
- `series_count`: explicitly infer consecutive chapters from the series chapter count. The default chapter edition uses this mode. It skips through the chapter count at add when a volume edition is collected (those chapters are in the volumes) and wants every chapter otherwise (a one-shot, a chapter-only series). Inference is capped at 10,000 books and records `inferred` provenance. Counts are not proof that numbering has no gaps.

Source discovery is opt-in per chapter edition and requires a verified source listing → edition mapping. Individual source observations enter review. Search records at most 100 new observations per source batch; accepting an observation adds one book, never every intervening integer. A verified active binding can be trusted to accept compatible individual chapter observations automatically; changing its edition clears that trust. Source maxima never generate intervening chapters. Accepted chapters become available on the next evaluation.

## Collected versions and profiles

Every edition has one implicit default version. Its selector is hidden until there are additional versions. Only collected versions persist edition intent; edition-level monitoring and cutoff in API responses are projections. Additional versions share its books and carry independent monitoring, per-book intent, required release group, profile assignment, chapter cutoff, and handling of unknown language/dates. Include extras enables bonus/other books; individual overrides can select them separately. “Skip through chapter N” means number > N. Chapter zero remains selectable with an override.

Profile inheritance is **version → edition → series → instance default**. An unset profile follows its parent; selecting a profile creates an explicit assignment. Profile deletion checks version assignments too.

Availability is `released`, `upcoming`, or `unknown`. Missing dates stay unknown. The version decides whether unknown-date books may be searched. Actual volume 0 never means “search the whole edition”; a search names a collected version, plus `bookId` for one book or `all` for the whole version. Widened to all books (`allBooks`), a search lists every release for every book but its plan proposes only the books the version wants; Select range picks releases inside a number range. A book's identity everywhere — wanted, reserved, grabbed, owned — is its id within a collected version, never its displayed number.

Whether a release belongs to an edition is decided by one table of **facets** (`internal/service/edition_fit.go`), each something a release name or an edition name can state. A *content* facet changes what is on the pages (omnibus, deluxe, coloured): release and edition must agree, and a mismatch is a hard rejection. A *packaging* facet changes only how the same pages are wrapped or produced (box set, digital rip, print scan): it never excludes a release and only breaks ties between editions of the same language that are both collected. An edition that exists in digital and in print is home to both kinds of release. A new term is a new row of that table, of one kind or the other. A MangaBaka edition name the vocabulary does not know ("Fullmetal Edition", "Colossal Edition") is applied as written: the edition takes only releases that carry the name, and a release that names another of the series' editions in full belongs to that one.

A catalog box set is a single item with one ISBN, while releases are named after the books inside. **This is a box of several volumes** (book menu) adds volumes 1..N carrying the box's ISBN and date and moves the box out of the main run so it is no longer collected. A torrent named for the series with no number at all is treated as an inferred whole-edition pack (`rejected.pack_unverified`, soft): reviewable, never grabbed automatically, and checked against its actual contents at import. Only collected editions compete for a release. A release that still fits several of them is `rejected.edition_ambiguous`: never grabbed automatically, downloadable after review from the edition the user searched. A name that states no language takes it from the indexer section it is filed under ("English-translated", "Raw"), then an English-only publisher's tag, then the indexer's default language setting. Torrent searches are restricted to the Books category and use the normalised title. A verified source listing can establish publication identity, while contradictory explicit language or edition tags still reject the release. Required groups and strict unknown-language policy are hard gates.

Upgrades are off by default. When enabled, Sento evaluates stored file facts against the **current** profile, requires the configured score improvement, and stops at the cutoff score (`0` means no cutoff). Pins prevent automatic upgrades. Historical files without trustworthy facts are not automatically upgraded. Earlier copies are retained; there is no automatic deletion or chapter-to-volume replacement.

The ledger is computed from books, monitoring intent, eligible coverage, file presence, pending jobs, availability, and optional upgrades. The Editions view manages both volumes and chapters, using the same model.

## Downloads and imports

Several torrent download clients can be configured (Connections), the default one first. Each torrent indexer may name the client its releases go to — a private tracker that must download through the VPN client, say; indexers that name none use the default. An indexer whose client no longer exists refuses to grab instead of falling back, and is reported on System → Health. The queue stores only the torrent's hash, so status and import look the download up in every client.

Acquisition reserves version/book pairs transactionally before contacting the provider. A second evaluation of the same active provider offer within a series attaches its coverage to the existing job instead of starting another transfer. Other offers cannot reserve the same pending book. Only terminal failure, completion, or cancellation releases reservations. Nonterminal `needs_review`, `retry_wait`, and `dispatch_unknown` retain them. The importer freezes and rereads the combined plan before importing.

Queue snapshots retain the parsed release, covered memberships, rejection/score details, profile, custom-format definitions, edition, and version settings. They explain the decision without relying on today's settings.

Every import is post-processed into a self-contained book when **Embed metadata** is on (the default): a clean CBZ (zip/cbz and rar/cbr accepted) with pages byte-for-byte as downloaded, a wrapping folder flattened, OS junk dropped, and a full `ComicInfo.xml` — series, number, volume, title, summary, dates, credits, publisher, genres, tags, language, reading direction, age rating, ISBN, release group, and a notes line (`sento:v1 mangabaka:… edition:… work:… release:…`) that re-identifies the file without the database. An existing ComicInfo is merged (Sento wins on fields it knows, unmodelled elements are kept) and preserved once as `ComicInfo.orig.xml`. Torrent payloads are copied, never modified, so they keep seeding; with embedding off they are hardlinked untouched.

A book's identity is its **content fingerprint**: the SHA-256 of the sorted SHA-256s of its page images. Container, names, order and ComicInfo do not enter it, so a payload and its processed copy are the same asset and re-tagging never changes identity. Non-archives fall back to a byte hash.

**Organize files** (series → Files) previews and applies what the current settings would change in existing files: move and rename to the templated path, and rewrite embedded metadata where the merge would change it. Fingerprints, coverage and pins are unaffected; emptied folders are removed; a seeding payload hardlinked to an old library file is never modified.

A series has a folder for its volumes (`series.path`) and, when chapters are kept apart, one for its chapters (`series.chapter_path`). Optional top folders per type (`volumesRoot`, `chaptersRoot`) put the type above the series: `Volumes/<series>/…` and `Chapters/<series>/…`; equal values (both empty by default) keep one folder per series. A series keeps its own folder name when re-rooted, and one adopted from outside the library root is never moved. Scans cover both folders, and storage availability is judged per folder, so an unreachable chapter tree does not block volumes. The `{type}` token puts the split inside the series instead (`<series>/Volumes/…`).

Folder layout inside a series folder is a template (`bookFolder`): by edition (`Standard Edition [en]`, the default, with `(version)` for additional copies), flat, by internal id (`editions/42/versions/81`), or custom. The fingerprint enters a file name only when configured or when a different book already holds the path.

Installation does not overwrite an occupied path with different bytes. Existing library paths are preserved. A pack stays **one archive** with several coverage links. Identical bytes from one job may satisfy several confirmed versions through the same asset. Different releases remain separate assets.

Scans consider paused editions too, check parsed language and edition evidence, and leave ambiguous archives for review. Unmatched downloaded archives are retained in the review area. Partial imports record verified files and hold the job in `needs_review` until unresolved coverage is reviewed; saving the mapping of its last waiting file imports the job, releases its reservations (planned books no file held are wanted again) and records "imported after review" in History.

Review proposes what a waiting file holds when its own name resolves it, read against every edition of the series, collected or not (D16). A number with no volume or chapter marker ("Kagurabachi 067") takes its kind from the download it came in — the volumes or chapters the release names, or the kind of books it was grabbed for — and stays unread outside a download. A name without the series title ("Volume_01.cbz") is only proposed inside a download of the series. Among editions holding the books, the collected one wins; still several, or a number shared by two books, and nothing is proposed. Proposals are computed when review is read and recorded only on Accept (one, or all at once); Review file opens the mapping pre-filled. Retrying a fully reviewed payload completes from its recorded files without fetching it again. Partial retries preserve already confirmed bytes and mappings.

File review can map an archive to several books and versions. Editing one version's mappings preserves the others. Clearing coverage keeps the file without claiming ownership. Coverage pins survive edits to retained mappings. Removing a version preserves intrinsic content knowledge and files; deleting a pinned file requires unpinning first. Duplicate-edition merges require unique book matches and preserve version policies, pins, coverage, and paths. Pending downloads block destructive identity changes.

## Presence, file facts, and recovery

Scans and acquisition checks record `present`, `missing`, `unavailable`, or `unknown` presence. An unreadable/missing series root blocks automatic acquisition for the series; a missing archive under a healthy root remains wanted. Page reads do not probe the filesystem or initialize models. Review provides an explicit recheck action.

Offer claims and per-file observed facts are stored separately. Filename observations take precedence over advertised language/group/quality; stream imports retain offer evidence. The file editor supports manual language/group corrections with provenance. Retrying or scanning identical bytes preserves manual corrections and content confirmation. Group contradictions remain visible for review.

Transient client errors back off from 30 seconds up to 32 minutes. If dispatch succeeded but saving its handle was interrupted, reconciliation checks the stable provider key without submitting again. Unresolved dispatch after ten minutes enters review, where a user can attach the client ID or cancel the reservation. Cancelling leaves remote downloads and files intact; an active import must finish before cancellation. Pending plans and edition identity edits use a common transactional parent lock.

## Screens and evaluation

The Editions tab opens on a **Collecting** summary: per kind of book, the collected editions with what is on disk and wanted, a search for what is wanted, the chapter cutoff in words, and the files waiting for review. Your editions contains selected editions, local publications, and editions with files; with several, each opens on demand. Other editions is a collapsed catalog chooser. Additional copies and metadata operations live in the edition action menu. The profile summary explains inheritance. Book lists load every book of a version at once: state counts double as filters, long editions open as a colour-coded grid (a dense list is the alternative view), and monitoring actions apply to a filter, a number range, or a selection. The series page has one tab row: Editions, Files, Review, Sources, History. Review combines files, jobs, and chapter observations and shows a count when something is waiting; file mappings are edited from Files; each source match carries its edition mapping under Sources.

Where information comes from is marked the same way everywhere (`web/src/components/Origin.tsx`): **MangaBaka**, shown as a small “MB” mark that opens the series’ MangaBaka page (listed in the catalog, updated by a refresh; the mark alone labels values pre-filled from MangaBaka), **Local edition** / **Added by you** (never touched by a refresh), **Edited** (a catalog value changed by hand, which a refresh keeps), **Counted**, also with the MB mark (a local edition numbered from a count MangaBaka gives, not listed one by one) and **No longer in MangaBaka**. An edition always carries its marker; a book carries one only where it differs from its edition. Edit book lists the copies MangaBaka has behind the book, one per medium, and says which supplies the ISBN and that the date is the earliest. When a refresh groups editions on its own, History records it.

MangaBaka knows some series without listing their volumes (it covers a handful of publishers). The Editions tab then says so and gives the volume count from the series record. **Add edition** is the one place an edition is added: first the editions MangaBaka lists that are not collected yet, then the form for one it does not list, with the publisher (for the chosen language) and the volume count pre-filled from the series record and labelled as coming from MangaBaka. Publisher and medium sit behind a toggle in Add edition and Edit edition, since they rarely need a hand. Nothing is created without the form being submitted.

The shared ledger batches books, overrides, facts, coverage, profiles, and pending work per series. It computes a complete consistent series view per request; the book list is returned whole and filtered in the browser. A book on disk can be unsatisfactory for a particular copy without disappearing from the library. New versions reuse qualifying verified files. Series totals count unique books on disk and wanted copies separately.

## Migration and compatibility

Migration 14 intentionally resets the old library and its downloads, file mappings, bindings, and decision history. Connections, profiles, custom formats, application settings, and physical files remain. Add series again after upgrading and review existing files before counting them as owned.

Migration 15 preserves the current library. It backfills intrinsic asset contents from confirmed coverage, moves edition intent entirely to versions, and adds cached presence, review/retry states, and independent edition traits. Existing file presence starts unknown and is checked during scan, search, or explicit review recheck.

Canonical ownership uses version and book IDs. The old number-based ownership tables and queued-plan adapters are removed. Migrations remain transactional on SQLite and PostgreSQL.

Migrations 22 and 23 add the `collections` and `works` tables, turn each catalog edition into an edition with one collection, and replace `external_refs`, the trait list and the content variant. A trait set by hand that the name did not carry moves into the name. Editions are grouped by the next refresh, not by the migration.

Migration 20 makes a linked edition a catalog edition and removes the leftover duplicate rows earlier links kept. Migration 21 renames the schema to the vocabulary used everywhere else: tables `editions`, `books`, `versions`, `book_reservations`, `book_overrides` and `book_observations`, with `edition_id`, `book_id` and `version_id` columns. "Collection" and "work" name MangaBaka's own records, in `internal/metadata` and in the tables that mirror them.

Deliberate limits: upgrades retain earlier copies; completed downloads are not automatically reused across unrelated editions or series. Explicit file mapping can share confirmed assets. A comprehensive story/chapter ontology and destructive retention rules remain separate future features.

Calendar entries use the same version/book ownership as the ledger, including per-version monitoring and actual file presence. A language correction invalidates affected coverage for review. Manual file mapping records explicit confirmation of edition coverage; release-group requirements still apply. A catalog rename is followed: the edition is what MangaBaka says it is.
