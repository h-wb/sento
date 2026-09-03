# Improvement log

Captured during the [code tour](tour.md) — smells, ideas, open questions.
Logged immediately, triaged later; an entry here is a lead, not a verdict.

Format: `- [ ] short description — context/file refs (session N)`

## Applied

- [x] **Bug fixed**: token `full` → `complete` fired inside "full color".
      Split `full` into its own case in `classifySegment` — sets complete
      only when not part of "full color"/"full colour"; `(Full)` standalone
      still means complete (torrent_packs.jsonl:8 relies on it). Locked
      with `complete:false` on torrent_volumes.jsonl:5 (session 1). The
      bigger `{phrase, apply}` consuming-scanner refactor is still open as
      a future cleanup — see Leads.
- [x] `mergeRanges` rewritten: sorts first, single coalescing pass, fixes
      the non-transitive-bridge bug; comment now matches. Unit-tested in
      `num_test.go` (session 1).
- [x] `isPureRangeSegment` regexp hoisted to package var `rePureRange`
      (was compiling per call) (session 1).
- [x] JP-volume one-element loop unwrapped in `extractNumbers` (session 1).
- [x] `docs/spec/parser.md` §2 schema drift — added a correction note
      pointing at `result.go` as truth (session 1).
- [x] **Finding A — edition/quality vocab unified** (was the root cause of
      the full-color bug class). Extracted the token/phrase scan from
      `classifySegment` into shared `scanTags`, now used by both the
      bracketed-segment path and `parseInto`'s free-text pass. Switched
      `phrase` matching from substring `strings.Contains` to word-boundary
      `indexWord`, which also kills the `raw`-in-`drawback` /
      `color`-in-`discolored` false-positive class for both paths. Free
      text now recognizes `2-in-1`, `Coloured`, `RAW`, `Digital`,
      publishers, etc. (verified live). Locked by `tags_test.go`
      (path-consistency + word-boundary). Residual: BLIND parse of a
      series whose title IS an edition word (`Raw Hero`) false-positives
      that tag — confined to blind/RSS (anchored parse removes the title
      first, verified correct); same risk class the old code already took
      for `deluxe`/`omnibus`. Finding B (title cutting) reduces it further.
      (session 1)
- [x] **Finding B — `extra`/`omake`/`special` title pollution fixed.** The
      extra-keyword loop in `parseInto` now drags `titleEnd` like the
      `complete` loop, but only when title text precedes the marker
      (`i > 0`) — a leading marker is likely the title itself (`Special A`,
      `Extra Life` keep their titles). `One Piece Special` → `"one piece"`
      (was `"one piece special"`, which fell below the 0.75 match
      threshold). Locked by two corpus cases in titles_hazards.jsonl.
      Residual: the `Extra` *flag* (not the title) still sets on blind
      `Extra Life`/`Special A` — same blind-only title-collision class as
      Raw Hero, anchored parse is correct. (session 1)
- [x] **Finding C — bracketed year/chapter-number collision documented.**
      `[1960]` reads as a year not a chapter (`reYear`). Not fixed (the
      default is right ~always: no series has reached chapter 1800+, marked
      chapters never hit this path); recorded as a known limit in
      `reYear`'s comment and parser.md §9 (session 1).
- [x] **Finding D — coverage-inert bare numbers documented.** Confirmed
      `.Bare` is read only inside parse.go (implicit-pack count +
      confidence), never downstream. Added a clarifying comment in
      `resolveBare` and a parser.md §9 note: an axis-ambiguous number
      conservatively covers no slot, by design (session 1).

## Applied (live-harvest session)

Drove these from real chapter names pulled off the running sento-host
sidecar (`cmd/harvest`, build-tagged): ~110 installed sources across ~14
languages, 497 records, 19 sources answering. See `docs/domain/parsing.md`.

- [x] **Bug fixed — bare alpha sub-chapters dropped.** `One Piece 1150a`
      (real Turkish source; the source's own `number` field said 1150.1)
      parsed to *nothing*: the `a→.1` alpha convention worked only for
      marker forms (`Chapter 4a`), while a bare `1150a` was rejected by
      `cleanBoundary`'s trailing-letter guard. Added an optional `([a-i])`
      group to `reBare` + the alpha-suffix math in the bare loop;
      `cleanBoundary` still rejects j–z glue, so no new false positives.
      Locked in source_chapters.jsonl.
- [x] **Improvement — optional `第` on CJK markers.** Chinese sources name
      chapters `1167話` / volumes `5巻` *without* the `第` prefix that
      `reChJP`/`reVolJP` required. Worked incidentally on source via the
      bare path, but `12話` on a torrent channel (small number) parsed to
      nothing. Made `第` optional in both regexes. Locked in raws.jsonl.
- [x] **Corpus coverage +12** (75 → 87): real multilingual names —
      Chinese leading-number + 話-suffix titles, Arabic/Italian/German
      sources, emoji-prefixed `🔒 Chapter 24.1`, decimal `Chapter 0.1` /
      `1058.5`, German dual-dash `Ch. 1 - One Piece - Der Geerbte Wille`.

## Leads

- [ ] `profile.matchRegexp` (internal/profile/profile.go) compiles the
      regex on every call — same class as the `isPureRangeSegment` smell,
      but on the funnel's HOT path: a search scores ~100 candidates × N
      formats × M regex conditions, recompiling the same user patterns
      thousands of times. Patterns are user-defined (dynamic) so the fix is
      a memoizing cache (`sync.Map` pattern→*regexp.Regexp`), not a package
      var. Defer until the funnel (session 6) confirms it's hot; safe,
      self-contained when done (session 2)
- [ ] **`match` fuzzy false-positive tradeoff** (known limitation, not a
      bug). `cmd/matchcheck` over ~1148 real source titles: self-match
      flawless (0 failures), order tolerance perfect (`Attack on Titan` ≡
      `Titan, Attack on` = 1.0), and romanization variants correctly merge
      (`Kaiju`/`Kaijuu` 0.98, `Necrophile`/`Necrophilia` 0.98 — the
      feature). The cost is ~0.3% false collisions ≥ AutoThreshold:
      1-char-apart distinct series (`Fish Boy`/`Fist Boy` 0.94) and
      stopword-heavy overlap (`High School of the Dead`/`The God of High
      School` 0.91). Inherent to string similarity — you can't accept
      `Kaiju≈Kaijuu` and reject `Fish≈Fist`; a real fix would be semantic.
      Mitigated already: exact match wins (1.0) when the right series is
      present, anchored search scopes candidates, 0.75–0.90 manual band.
      Only risky when a release for an UNtracked series meets a tracked
      look-alike. Possible future: stopword down-weighting. (session 1,
      match validation)
- [ ] `cmd/harvest` (build-tagged manual tool) left in the tree — decide
      whether to keep it as a reusable corpus-harvester or drop it. Re-run
      with more queries/sources to keep mining (session 1, harvest)
- [ ] `archiveExts` (internal/parse/preclean.go) vs `scanExts`
      (internal/service/scan.go) — two extension lists with overlapping
      but different members; check whether the difference is deliberate
      and document it, or unify (session 1)
- [ ] **Source channel assumes chapters for bare numbers** — two layers:
      (1) `resolveBare` prior #1 (ChannelSource → chapter) but only for
      *markerless* numbers; marked volumes (`Vol. 5`/`v05`) on source
      still parse as volumes and the funnel's `coverageRanges` routes
      them to volume slots, so Madokami-style volume sources work *when
      they mark volumes* (verified live). (2) Deeper: `sourcehost.Chapter`
      models every source release as a chapter (`Number float64` →
      candidate `ChapterNumber`); no notion of a volume-native source.
      Gap: a source listing a volume as a bare integer ("5") → misfiled
      as chapter 5. The source's own `Number` corroboration is dropped
      before axis resolution (parse is pure/string-only by design).
      Likely fine in practice (volume archives mark volumes); revisit if
      a real volume-native source surfaces. Ties to D15 "sources never
      feed the ledger" (session 1)
- [ ] **i18n volume markers**: marker vocab covers EN + FR only
      (`vol/volume/tome`, `ch/chapter/chapitre`) + JP (巻/話). Verified
      live: Spanish/German *chapters* still parse on the source channel
      (bare number + channel prior rescues them), but *volume* markers
      like `Tomo 5` / `Band 5` on torrent fail — the number stays bare
      and never routes to a volume. Decide: (a) accept as FR+EN bias and
      document it in parser.md, or (b) add tomo/capítulo/band/kapitel/
      tomo-PT etc. to the marker alternations. Add corpus cases either
      way so the choice is explicit. Ties to D10 single-language design
      (session 1)
- [ ] `deploy/docker-compose.yml` still runs a pinned Suwayomi server and
      README's status paragraph says "Suwayomi-powered", but the code talks
      to the sento-host sidecar (`host/`) — deploy/README look stale
      relative to the code (pre-tour finding)
