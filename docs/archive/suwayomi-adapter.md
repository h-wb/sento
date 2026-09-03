# Sento — Suwayomi Adapter Specification

Component spec for the `SourceProvider` implementation backed by Suwayomi
(spec.md §6). Unlike the rest of the spec, this document is **empirical**:
every claim marked ✅ was validated live against a real instance.

> Validated 2026-06-09 against Suwayomi-Server **v2.2.2166 (Preview)**,
> GraphQL at `/api/graphql`, REST at `/api/v1`. Source used: Weeb Central
> (`2131019126180322627`). Cross-referenced with the Suwayomi-Server source
> tree where noted.

---

## 1. The validated flow

The entire Sento chapter path, end to end, with no Suwayomi library
involvement:

```
sources ──► fetchSourceManga(SEARCH) ──► fetchChapters ──► enqueue ──►
poll isDownloaded ──► GET CBZ over REST ──► deleteDownloadedChapter
```

### 1.1 List sources ✅

```graphql
{ sources { nodes { id name lang isNsfw extension { pkgName } } } }
```

Source IDs are stringified int64s (e.g. `"2131019126180322627"`). ID `"0"`
is the special **Local source** — exclude it from Sento's source list.

### 1.2 Search / bind ✅

```graphql
mutation { fetchSourceManga(input: {type: SEARCH,
    source: "<sourceId>", query: "frieren", page: 1})
  { hasNextPage mangas { id title url sourceId inLibrary initialized } } }
```

- Returns source results **and persists them** as manga rows in Suwayomi's
  DB (`id: 195`, `inLibrary: false`). The numeric `id` is the handle for
  everything else; `(instance, sourceId, mangaId)` is Sento's
  `SourceBinding`.
- ✅ **No library membership is ever needed** — search, chapter fetch,
  download and CBZ retrieval all work with `inLibrary: false`. Suwayomi
  stays a pure engine; its library should simply stay empty (then its
  library-scoped automations like `autoDownloadNewChapters` are inert).
- Consequence: search-result manga rows accumulate in Suwayomi's DB.
  Harmless; not Sento's problem to GC.

### 1.3 Chapter list (release candidates) ✅

```graphql
mutation { fetchChapters(input: {mangaId: 195})
  { chapters { id sourceOrder name chapterNumber scanlator uploadDate url } } }
```

- Returned 147 chapters with DB ids, `name` (`"Chapter 1"`),
  `chapterNumber` (Mihon `ChapterRecognition` output, float), `scanlator`,
  `uploadDate` (epoch-millis **string**).
- ✅ **IDs are stable across re-fetches** (verified: identical ids on second
  call). Safe to hold a chapter id through the grab lifecycle; still
  re-fetch before enqueue to pick up source-side changes.
- `fetchChapters` hits the source live (it's a mutation: it syncs rows).
  Per-series new-chapter checks (spec.md §10) call exactly this.
- Suwayomi's `chapterNumber` is *corroborating evidence* for our parser
  (`source` channel), not a replacement — our parser owns ranges, decimals
  and trace.

### 1.4 Download ✅ — with two gotchas

```graphql
mutation { enqueueChapterDownload(input: {id: 9328}) {
  downloadStatus { state queue { chapter { id } state progress tries } } } }
```

Observed behavior:

- The mutation's *returned snapshot* said `state: STOPPED, queue: [QUEUED]`
  — yet the chapter downloaded ~10 s later with no further calls.
  **Enqueue effectively self-processes; the snapshot is pre-start.**
- **Gotcha 1**: `startDownloader` requires an argument (`input: {}`), and
  when called with an already-drained queue it blocked and returned a 30 s
  `TimeoutCancellationException` GraphQL error. Adapter rule: *don't* call
  `startDownloader` routinely; call it only as a recovery action when items
  sit `QUEUED` with `state: STOPPED` for > N seconds, and tolerate the
  timeout error response.
- **Gotcha 2**: completed items **vanish from the queue** — the queue is
  not a history. Completion signal is the chapter itself:

```graphql
{ chapter(id: 9328) { isDownloaded pageCount } }   # ✅ poll this
```

- Failure visibility: queue items carry `state` (incl. `ERROR`) and `tries`
  while in-flight. Adapter poll loop: `chapter.isDownloaded` for success,
  queue entry for in-flight/error, absence-from-queue + `isDownloaded:
  false` after timeout = failed silently → retry once, then surface.
- `downloadChanged` GraphQL subscription exists (websocket) — optimization
  only; polling is the contracted mechanism `[default: poll 2 s during
  active grabs]`.

### 1.5 CBZ retrieval ✅

```
GET /api/v1/chapter/{chapterId}/download
→ 200, content-type: application/vnd.comicbook+zip
  content-disposition: attachment;
    filename="Frieren - Beyond Journey's End - [Official] Chapter 1.cbz"
```

- Archive contents: zero-padded page images (`001.jpg … 039.jpg`) **plus a
  Suwayomi-generated `ComicInfo.xml`** (observed fields: `Title`, `Series`,
  `Number`, `Summary`, `Writer`, `Translator` (= scanlator), `Genre`,
  `Web` (source chapter URL), nonstandard `ty:PublishingStatusTachiyomi`,
  `Day/Month/Year`). Our importer's merge policy (spec.md §11.3) applies:
  Sento/MangaBaka fields win, theirs preserved once as `ComicInfo.orig.xml`.
- Identification: Sento already knows what it requested (grab context);
  the disposition filename is parseable but **must not** be the identify
  path.
- Instance had `downloadAsCbz: true`. Per source code
  (`ChapterDownloadHelper.getCbzForDownload` → `getAsArchiveStream`), the
  endpoint streams an archive even from folder storage — so Sento works
  either way, but setting `downloadAsCbz: true` is still the recommended
  install default (avoids on-the-fly zipping cost). *(folder-mode not live-
  validated)*

### 1.6 Cleanup ✅

```graphql
mutation { deleteDownloadedChapter(input: {id: 9328})
  { chapters { id isDownloaded } } }
```

Deletes the stored files (manga/chapter rows remain). Sento calls this
after successful import verification — Suwayomi's disk usage stays bounded
by in-flight grabs only.

## 2. Adapter contract (refined from validation)

```go
type SourceProvider interface {
    ListSources(ctx) ([]Source, error)                       // excludes Local source
    SearchManga(ctx, sourceID string, q string, page int) (MangaPage, error)
    GetChapters(ctx, mangaID int) ([]SourceChapter, error)   // live re-sync
    EnqueueDownload(ctx, chapterID int) error
    DownloadState(ctx, chapterID int) (DownloadState, error) // composite: chapter.isDownloaded + queue entry
    FetchCBZ(ctx, chapterID int) (io.ReadCloser, ArchiveInfo, error)
    DeleteDownload(ctx, chapterID int) error
    Recover(ctx) error                                       // startDownloader(input:{}), timeout-tolerant
}
```

Changes vs the spec.md §6 sketch: no opaque `JobID` (the chapter id *is*
the job id — queue entries are keyed by chapter); `DownloadState` is a
composite read; `Recover` is explicit and rarely used.

Concurrency: Suwayomi has its own download queue/parallelism settings;
Sento limits in-flight grabs per instance `[default: 3]` and treats the
instance as a shared, stateful resource (one adapter, serialized enqueues).

## 3. Operational notes

- **Auth**: the validated instance was unauthenticated. Suwayomi supports
  basic-auth/simple-login via settings; adapter config carries optional
  credentials. Sento health-checks `aboutServer { version }` (15 m, §10).
- **Version pinning**: validated against a *Preview* build; Preview moves
  fast and the GraphQL schema is not frozen. Sento's docker-compose pins a
  known-good Suwayomi version; the health check logs (not blocks) a version
  outside the tested range. Schema drift here is the same class of risk as
  MangaBaka (D13) — same answer: one adapter package, contract tests
  against recorded GraphQL fixtures.
- **Timeout discipline**: internal Suwayomi operations can block ≥ 30 s
  (observed). All adapter calls carry context deadlines (search/chapters
  60 s — sources behind FlareSolverr are slow; mutations 30 s; CBZ
  streaming none).
- **Errors arrive as GraphQL `errors` with partial `data`** — the adapter
  must check both, and must treat the `startDownloader` timeout error as
  benign during `Recover`.
- `uploadDate` is epoch-millis as a string; parse defensively.

## 4. Not yet validated (tracked, not blocking)

- Folder-storage (`downloadAsCbz: false`) REST streaming — code-verified
  only.
- Login-required sources (Madokami) and FlareSolverr-fronted sources —
  configuration lives in Suwayomi; adapter just sees slower/failing calls.
- `fetchSourceManga` POPULAR/LATEST browse modes and source filters — not
  needed for v1 (Sento is search-driven).
- `downloadChanged` subscription payloads.
- Multi-instance Suwayomi — out of scope v1 (spec.md §6).
