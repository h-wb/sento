# Edition model: second design review

Reviewed the domain guide and implementation at `ed37297`. The recommendations below were accepted and implemented in the follow-up. They describe the issues found at review time; [the current domain guide](../domain/collections.md) describes the resulting behavior.

## Assessment

Keep the separation between catalog metadata, acquisition intent, release offers, and actual files. Stable local identities, explicit local editions, field overrides, release snapshots, non-overwriting imports, and explicit coverage are sound foundations. Keep Go, React, and Mantine. A replacement library would not address the product's main difficulty: users currently see too much of the internal model at once.

The next work should consolidate the implementation and simplify the common workflow. Avoid expanding the metadata ontology until a concrete workflow needs it.

## 1. Separate file contents from collection policy

Current `asset_coverage(file_id,target_id,work_id)` records both what a file contains and which acquisition target accepts it. Deleting a target cascades its coverage, losing content knowledge when that target was the only reference. Changing required-group or unknown-language policy filters mappings out of the computed owned state, even though the file is still present. Adding a second target cannot automatically reuse previously confirmed coverage.

Represent two distinct facts:

- **File contents:** asset → edition unit, with verification and evidence. These survive deletion or editing of collection preferences.
- **Target fulfillment:** target → accepted asset/unit, evaluated against current policy. Pins and deliberate user selection belong here.

Keep fulfillment derived where practical; persist explicit selections and overrides rather than another independently maintained truth. A policy change should produce “On disk, does not meet this version's rules,” not make the UI imply the file has vanished. A new target can evaluate existing verified assets before searching remotely. This must still respect language, edition, group, and quality requirements; shared catalog IDs or equal numbers alone are insufficient evidence.

This also permits clear metrics: unique volumes on disk, wanted copies, and files requiring review. Current series totals flatten target slots, so collecting the same volume twice increases “Owned” twice.

Evidence: `internal/store/targets.go:DeleteTarget`, migration 14's coverage foreign keys, `internal/service/targets.go:eligibleCoverage`, `web/src/pages/SeriesPage.tsx:allSlots`.

## 2. Make acquisition and recovery states explicit

### Confirmed mismatch: disappeared editions remain wanted

`ReconcileMetadata` marks all catalog editions stale, then reactivates those in the new snapshot. Works are marked stale only inside the loop for editions present in that snapshot. `targetSlots` checks work state, not the parent edition state. A disappeared edition can therefore retain linked, wanted units.

A temporary test using `modelFixture`, an empty catalog collection snapshot, and `TargetSlots` failed with `catalog-missing edition still wants work 1 (missing)`. The probe was removed after execution. This confirms the ledger mismatch; it was not an end-to-end remote download test.

Gate automatic acquisition on the effective state of both edition and unit. Retain files and monitoring intent, and provide a visible way to continue an abandoned catalog edition locally.

### Storage availability is not file absence

Both `prepareTargets` and `TargetSlots` treat every `os.Stat` error as “no file.” Permission errors or unavailable storage should block acquisition and show a storage issue. Even ENOENT needs root/mount health context before treating an entire library as missing. Keep filesystem probing outside frequently refreshed ledger reads; track presence through scans/imports with targeted rechecks.

### Import review is not download failure

`ProcessQueue` maps all processing errors to `failed`, while failed jobs stop reserving units. An incomplete match or transient provider error can therefore release claims before the existing payload has been resolved.

Use explicit states such as downloading, ready to import, importing, needs review, retryable error, completed, and cancelled. Preserve claims for a downloaded payload awaiting review. Retry transient communication with backoff; retry imports from the existing payload. Release unresolved claims only after a terminal decision or deliberate cancellation. Permit successfully imported units in partial jobs to remain owned.

### Make side effects recoverable

Keep reservations and snapshots, but recover the gap between a successful provider grab and saving its remote handle. Use provider reconciliation/idempotency where supported, and surface an uncertain dispatch when it cannot be established. Deletion/merge checks and reservation validation must share transactional locking or version checks; preflight checks outside the mutation transaction leave race windows. This is a code-inspection risk, not a reproduced concurrent failure.

## 3. Treat parsed metadata as evidence

`recordAsset` receives parsed file data but writes language, group, quality flags, and revision primarily from the release snapshot. Every archive in a mixed pack can inherit the same claimed facts. Save offer claims and observed file facts separately, with provenance and explicit user corrections. Prefer more specific reliable evidence, and send contradictions for review. Write facts and verified coverage atomically after installation.

Unknown metadata needs a first-class state. For example, absence of an omnibus/color tag does not establish a standard/original edition, though it may be a useful search heuristic. Keep strong contradictory evidence as a rejection and distinguish weak guesses from confirmed matches.

The current layout enum also mixes packaging with publication characteristics: a deluxe edition can be an omnibus, and a box set can group standard books. Retain catalog descriptions and a small set of independent traits where known. Do not build a universal content graph or infer omnibus ownership from numeric ranges. Existing explicit relations can remain advanced annotations until they support a concrete workflow.

MangaBaka remains an upstream adapter, with local IDs and raw payloads retained. Its [official API documentation](https://mangabaka.org/data/api) explicitly says the schema has not reached 1.0 stability. That supports the boundary already chosen; it does not justify treating every metadata change as a different publication.

## 4. Simplify the default edition experience

Most users want to choose a language/edition, choose a quality profile, and let the app collect missing books. They should not need to configure a “Default” version as another concept before that works.

Recommended series flow:

1. **Your editions:** compact rows for editions actively collected or containing files, with language, publisher, format, progress, and a Collect switch. Expand one edition to inspect its books.
2. **Other editions:** collapsed catalog chooser with language/format filters. Keep unselected catalog entries out of the main work area.
3. **Add local edition:** available from the chooser when the desired publication is missing. Explain that it remains independently editable until linked to catalog metadata.
4. **Additional copies:** an advanced action inside an edition. Once more than one target exists, show named versions and their rules. Use examples such as “Group A copy” and “Group B copy.” A preferred group alone usually belongs in a profile, whereas keeping both groups needs two targets.

Keep an implicit default target internally. Hide its selector when it is the only target. The target's monitoring state should determine collection intent; remove the parallel `collections.monitored`/`cutoff_num` mirrors once remaining callers use the canonical target model. Edition metadata should not itself own acquisition state.

Show a compact effective-profile summary, such as “Balanced · inherited from series.” Keep version overrides advanced. Four inheritance levels can remain internally without requiring four configuration screens during setup.

## 5. Use book-oriented controls and explain consequences

Use **Volumes** and **Chapters** in ordinary screens. Keep “unit” as an internal term. In the editor, make the category an advanced field with labels **Regular**, **Bonus / special**, and **Other**. Offer an explicit “Include extras” monitoring rule, with per-item overrides. Preserve MangaBaka's category separately from a user's decision to collect it.

The previous suggestion “Part of the main series” is not ideal: bonus material can still belong to the main series. These are counting/collection categories, not a claim about narrative canon.

Add status filters and bulk monitoring to large lists. Numbering cutoffs need plain, exact semantics: current comparisons use `number > cutoff`, while some labels say “Monitor from chapter.” Prefer “Start at chapter 25” with inclusive behavior, or “Skip through chapter 24” with exclusive behavior. Volume/chapter zero must remain selectable.

Show a reason behind status: paused series, paused version, excluded extra, future date, unknown date blocked by policy, pending download, storage unavailable, or incompatible existing file. Separate ownership, availability, monitoring intent, and acquisition progress instead of forcing all of them into one badge.

Provide one review queue for unmatched files, uncertain edition matches, import failures, and new chapter observations. Each entry needs evidence, a proposed action, and its effect. Offer bulk acceptance for compatible observations and optional trust rules for an explicitly verified source; reviewing every ordinary chapter forever would defeat automation.

Move source mappings, merge/link operations, and content relationships into edition settings or contextual review actions. They currently occupy significant space despite being occasional maintenance tasks. File mappings belong primarily in Files/review, with links from the relevant book.

## 6. Consolidate implementation before further expansion

- Reads should be reads. `ListTargets` calls `EnsureModels`, which can create targets and inferred chapters. Run initialization/inference during add, refresh, accepted observation, or explicit configuration changes.
- Use one ledger evaluator for search, calendar, and UI. `wantedSlots` still builds the old numbered wanted sets before `prepareTargets` builds the canonical version/unit sets. Remove duplicate calculations and their compatibility projections where no longer needed.
- Batch works, overrides, facts, profiles, and coverage per series. `TargetSlots` currently repeats queries per target and filesystem checks on requests. Add server-side pagination for large chapter lists instead of only slicing all received rows in the browser.
- Separate edition overview, unit list, target settings, file review, and metadata dialogs into focused components. The current large manager component mixes several independent workflows.
- Update stale comments/spec sections rather than relying on a notice at the top. They still describe descriptor-first refresh and generated chapter behavior that no longer matches the code.

## Suggested implementation order

1. Fix parent-edition state gating, storage-error handling, and nonterminal import states; add failure-oriented tests.
2. Separate verified asset contents from target fulfillment and reuse existing files before acquiring another copy.
3. Simplify the edition overview/default target and add useful status explanations and review actions.
4. Consolidate ledger evaluation, write/read boundaries, queries, and components.

Acceptance scenarios should include: deleting a target retains known file contents; changing a group rule shows existing-but-unsatisfactory files; a second target reuses a qualifying verified asset; storage loss does not trigger mass redownloads; catalog disappearance suspends automation; a mixed pack retains per-file facts; import review does not enqueue a second download; concurrent edits and grabs cannot leave dangling plans; large chapter collections remain responsive.

Retain earlier files by default, explicit catalog linking, unknown dates, fractional labels, independent local editions, and manual review for ambiguous ownership. Defer automatic chapter-to-volume replacement, a comprehensive story ontology, cross-series deduplication, and UI-library changes.

## Implementation and verification

Migration 15 preserves existing data and separates asset contents from policy. The follow-up adds independent edition traits, cached storage presence, nonterminal queue recovery, per-file facts/provenance, transactional edition locks, shared ledger evaluation, paged book responses, bulk monitoring, trusted chapter observations, and a consolidated review inbox. The UI keeps the default target implicit and moves occasional actions into menus/advanced settings.

Regression coverage exercises catalog disappearance/local continuation, target deletion and reuse, unsatisfactory on-disk copies, unavailable storage versus missing files, mixed evidence and manual corrections, review completion without refetching, transient retry reservations, fractional chapters/zero/extras, trust reset, and concurrent deletion/reservation. Browser coverage exercises edition editing, additional copies, zero-volume navigation, unsaved edits, and mobile overflow.

Pagination bounds the returned book list; the server still evaluates the full batched series ledger before filtering. Cross-series deduplication, automatic chapter-to-volume replacement, destructive file cleanup, and a universal content ontology remain deferred as proposed.
