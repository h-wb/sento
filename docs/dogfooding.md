# Dogfooding log

Friction and usage notes captured while actually using Sento — the
self-usability study. Same deal as [improvements.md](improvements.md):
logged immediately, triaged later; an entry is a raw observation from real
use, not a verdict. Code-internal findings still go to improvements.md.

Format: `- [ ] short description — what happened, where/file refs (YYYY-MM-DD)`

## Open

- [ ] **Review process is not super intuitive.** The inbox
      (`web/src/components/ReviewInbox.tsx`, surfaced on the series page via
      `EditionManager.tsx` and on `QueuePage.tsx`) mixes four item kinds,
      each wanting a different action: files to confirm (edition + books,
      unverified asset contents, presence issues), failed or needs-review
      downloads, pending chapters from sources, and MangaBaka edition-link
      suggestions — items built in `internal/store/review.go` (`reviewRows`
      + `LinkSuggestions`), unioned into one list. Triage: do a think-aloud
      pass and find where it loses the user — discovering the inbox,
      understanding what an item wants, or seeing the result of acting on
      it. (2026-10-29)
- [ ] **Nausicaä case (live): the review loop doesn't close.** Force-grabbed
      the Deluxe Edition torrent — the funnel itself had flagged it
      (`pack_unverified`: "Nothing in the name says which books this pack
      holds"; `numberSource: unknown`). The import then parked everything:
      `partial import: verified 0 of 2 expected books; unmatched files
      need review` (queue item 1). After manually confirming the files in
      the file-review view, both split units (books 1487/1488) now have
      Volume_01/Volume_02 linked and on disk — but (a) queue item 1 *still*
      sits in needs_review with the stale "verified 0 of 2" message, which
      now contradicts the actual state; there is no "you're done"
      transition after manual resolution; and (b) the torrent's third file,
      `0 [hash].cbz`, is quarantined in the series `review/` folder as
      unrequested, with the prompt "Confirm the edition and books in this
      file" — no hint of what the file is, why it's there, or what
      confirming would do (unit '0' is unmonitored). The process is *safe*
      — it refuses to guess and the quarantine is right — but it strands
      the user: stale items, unexplained mystery items, and the decision
      trail (funnel flagged → forced → partial import → manual confirm)
      scattered across the queue snapshot, the queue error, and the file
      prompt, with no single narrative. Sharpens the entry above.
      (2026-10-29)
- [ ] **Dev checkout + remote qBittorrent = raw, unactionable error.** The
      same grab on the *dev* checkout sticks in needs_review with
      `import needs review: open /data/Downloads/qbittorrent/complete/sento/…/
      Volume_01.cbz: no such file or directory` — the download client's
      path, which doesn't exist on the Mac host. Two gaps, one hard limit:
      the error is raw (no plain-language class for "the files live on the
      download client, not here"); the path-mapping feature exists but only
      as env/yaml (`SENTO_PATH_MAPPINGS`, `internal/config/config.go:28` —
      documented in `deploy/docker-compose.yml`, absent from the
      Connections UI where you set up the client); and fundamentally a Mac
      dev checkout can't import from the remote client at all unless its
      files are mounted locally. Dev stacks must stub or skip the
      download/import leg (cf. `cmd/e2estub`). (2026-10-29)
- [ ] **Editions are still a bit confusing.** The editions area —
      `EditionManager.tsx` with the editor set from `EditionDialogs.tsx`
      (EditionEditor, VersionEditor, BookEditor, CatalogLink,
      MappingEditor), the `EditionBooks.tsx` slot grid, `EditionActions.tsx`
      — is hard to find one's way around. The model underneath is deep
      (editions → collected versions → books → slots → files,
      `docs/domain/editions.md`) and the UI exposes all of it. Triage:
      think-aloud session focused on the editions flow; find whether the
      confusion is the hierarchy, the naming, or not knowing what the
      actions do. (2026-10-29)
- [ ] **Nausicaä case (live): boxset + "split this edition" felt like the
      wrong tool.** MangaBaka models the "Deluxe" box set as a single unit
      numbered 0 — the "boxset has 0 volumes" quirk — while the torrent
      carries three files: Volume_01, Volume_02, and a `0` CBZ. To make the
      grab map, I split the unit into 2 (units 1 and 2) — which left the
      `0` file homeless (unit '0' unmonitored, the file quarantined in
      `review/`). Not convinced split was the right model: the torrent's
      real layout is 0/1/2, or the `0` file is an extra that isn't a book
      at all. A custom edition (like the auto-created "Chapters" edition)
      might model it honestly, but nothing in the editions UX surfaces
      that choice — split was just what was there. Sharpens the editions
      entry above. (2026-10-29)
- [ ] **"Keep an additional version" doesn't do what it sounds like.** In a
      collected version's ⋯ menu, I expected "Keep an additional version"
      (`EditionManager.tsx:298`) to add another *edition* (a Standard next
      to the Deluxe); it actually opens the VersionEditor
      (`EditionDialogs.tsx:74`) to add another *collected version of the
      same edition* — "Keep another release of this edition, such as a
      second release group". The action I wanted exists elsewhere: a
      separate "Add edition" button (`EditionManager.tsx:164,435`). The
      edition / collected-version / book levels (`docs/domain/editions.md`)
      aren't visible in that menu — "Edit edition", "Keep an additional
      version", and "Add volume" sit side by side with no cue of what
      level each acts on, and the label never says "of this edition". Fix
      shape: rename ("Keep another copy of this edition"), group the menu
      by level, or label the levels in the menu. Sharpens the editions
      entries above. (2026-10-29)
- [ ] **No way to see the source when adding a series.** When adding — and
      after, on the series page — nothing links back to the metadata
      source, so I can't check "what it is" on MangaBaka before trusting
      the entry (or after, when something looks off). The data is nearly
      there: lookup results and stored series both carry `mangabakaId`
      (`SeriesRef`, `web/src/types/metadata.ts`; `Series.mangabakaId`,
      `types/store.ts`), and the add card (`AddSeriesForm.tsx:74-87`)
      already renders `coverUrl` — but neither the add flow nor the series
      page exposes a source URL (the MangaBaka client talks to
      `api.mangabaka.org/v1`; the site link must be stored from the API or
      constructed from the ID). Fix shape: a "view on MangaBaka" external
      anchor on the search-result card and the series header. (2026-10-29)
- [ ] **Chapter list shows the oldest first; no way to sort.** In a chapter
      edition's book list, chapter 1 is on top and the latest chapter ends
      up on the last page — but the interesting end is the latest one
      (what just got grabbed, what's missing next). Latest-first would be
      more convenient. Trail:
      `web/src/components/EditionBooks.tsx` (no sort; list view pages the
      API order starting at page 1) ← `api.versionBooksOf`
      (`GET /series/:id/versions/:version/books`) ←
      `Library.VersionBooks` (`internal/service/edition_view.go`) ← slots
      built from books ordered `ORDER BY sequence_num,count_type,id`
      (`internal/store/presence.go:73`). The API returns the whole list
      unpaged, so a client-side sort in the component looks sufficient.
      (2026-10-29)

## Applied

(none yet — entries move here from Open once a fix lands, with how it was
resolved and what verified it)
