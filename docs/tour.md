# Guided code tour

An interactive, exhaustive walkthrough of the backend, one session per
sitting. Goal: understand every package well enough to change it
confidently, documenting as we go and logging improvement ideas without
derailing (see [improvements.md](improvements.md)).

## How a session works

1. Walk through one package group: its job, key types, the interesting
   functions, with `file:line` references to read alongside.
2. Questions and challenges welcome at any point — they shape the docs.
3. Smells and ideas go to [improvements.md](improvements.md) immediately;
   triage happens later, never mid-tour.
4. At session end, distill what we covered into a domain doc
   ([domain/](domain/)) and/or the flow docs it unlocked
   ([flows/](flows/), index in [architecture.md](architecture.md)).

Sessions go bottom-up: leaf packages first, so every later layer is read
with its vocabulary already in place.

## Sessions

- [x] **1 — The parser & title matching**: `internal/parse`, `internal/match`
      → domain doc [domain/parsing.md](domain/parsing.md) — **complete**.
      3 bugs fixed, Findings A–D, live-harvest corpus expansion (75→87) +
      2 harvest-driven parser fixes, match validation. Tools: `cmd/harvest`,
      `cmd/matchcheck` (build-tagged).
- [x] **2 — Profiles & the ledger**: `internal/profile`, `internal/ledger`
      → domain doc [domain/profiles-ledger.md](domain/profiles-ledger.md)
      — **complete**. Both pure & clean; 1 efficiency lead (matchRegexp).
- [ ] **3 — Persistence**: `internal/db`, `internal/store`
      → domain doc `domain/persistence.md`
- [ ] **4 — Providers & clients**: `internal/provider`, `internal/torznab`,
      `internal/qbittorrent`, `internal/sourcehost`, `internal/prowlarr`
      → flow `07-source-download.md`
- [ ] **5 — Metadata & the library side**: `internal/metadata`(+`mangabaka`),
      `internal/mediafile`, service: library, scan, bulkimport, mediasettings
      → flows `02-add-series.md`, `03-refresh.md`, `08-library-scan.md`
- [ ] **6 — Acquisition**: service: search, funnel, grab, importer
      → flows `04-search.md`, `05-grab.md`, `06-import.md`
- [ ] **7 — Wiring & operations**: `internal/config`, service: connections,
      indexers, sites, extensions, profiles; `internal/scheduler`,
      `internal/server`, `cmd/sento`
      → flows `01-startup.md`, `09-settings.md`, `10-scheduler.md`
