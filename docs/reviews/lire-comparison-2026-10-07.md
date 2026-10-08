# Lire comparison — 2026-10-07

## Recommendation

Borrow Lire's acquisition experience: show the books a download plan would add, explain what remains unavailable, and make the common action obvious. Keep Sento's domain model, parser pipeline, durable queue, and Mantine interface. The most useful backend addition is an explicit acquisition planner; the most immediately useful search change is bounded alias queries for torrents.

Lire is oriented around finding a series and getting it into Kavita. Sento manages particular editions and versions, including chapters, custom numbering, different groups, and multiple copies. Their simpler model is not a replacement for ours.

This is an investigation and proposed direction, not an accepted implementation design. No application code was changed.

> **Follow-up, 2026-10-07:** sections 1–4, 6 and 7 were implemented afterwards; see [acquisition planning](../domain/acquisition.md). Section 5 (reader handoff) was declined and is out of scope.

## Scope and verification

- Lire revision: [`1271458755f58b6dd352b09f53e54308cf41af60`](https://github.com/Joopinhontas/lire/tree/1271458755f58b6dd352b09f53e54308cf41af60).
- Sento revision: `2048de0`. This includes the fixes in `c1564e1` and `ba5f731` and the unified download-client settings. Earlier review findings are not treated as outstanding here.
- Read the implementation of release parsing, planning, metadata/search, ownership, downloading, monitoring, conversion, reader integration, and the relevant frontend; examined the supplied series, details, and library screenshots and design brief.
- Ran Lire's complete Python suite in an isolated virtual environment: **96 passed**, one dependency deprecation warning.
- Wrote and ran **five independent behavior probes**, all confirming the limitations described below. Probe code: `/tmp/test_lire_review_probes.py`; checkout: `/tmp/sento-lire-review-20261007`; interpreter: `/tmp/sento-lire-review-venv/bin/python`.
- Did not connect live Prowlarr, qBittorrent, or Kavita instances. UI conclusions come from source and supplied screenshots, not a full browser or accessibility audit. Passing tests establish the tested behavior, not production reliability.

## What is worth adopting

### 1. A reviewable download plan — highest product value

Lire's [planner](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/plan.py#L56-L115) reevaluates how much new coverage each release contributes after each selection, penalizes overlap, and removes redundant selections. The UI presents a preselected plan and lets the user inspect alternatives.

Sento currently ranks candidates and walks them in that order in `internal/service/grab.go:GrabBest`. `web/src/components/CandidateList.tsx` already has good filtering, rejection explanations, and expandable detail, but it remains a list of individual acquisition choices. A plan would connect those choices to the user's actual objective.

Suggested interaction, scoped to the selected edition and version:

> 12 volumes missing · 10 found in 2 releases · about 1.4 GB
>
> Adds volumes 3–12. Volumes 13–14 were not found in this search.
>
> Download 10 volumes · Review 2 releases

Show useful coverage first on each chosen release: “Adds 3–8; also contains 1–2 already owned.” Keep title, group, format, size, and the existing explanation available. Selecting another release should update the proposed coverage and duplicate count immediately.

Implementation boundaries:

- Plan over `(versionID, bookID)`, not integer ranges. Custom editions, extras, revisions, fractional chapters, and simultaneous versions must remain separate.
- Apply existing edition matching, verification, monitoring, profile, cutoff, and hard rejection rules before planning. A large pack must not win by violating them.
- Distinguish **the release's full contents**, **books that need acquisition or an upgrade**, and **the subset assigned to each job**. For example, packs 1–5 and 4–8 can add all eight books, while reservations for 4–5 must not conflict. Today `Grab` reserves candidate coverage as a whole; this needs an explicit design, not just a different sort order.
- Preserve full content evidence for import even when only a subset is requested. Extra bytes in a pack do not automatically satisfy another version.
- Prefer candidates that meet the desired quality; within the allowed policy, balance additional useful coverage, duplicate content, size, and availability. Preserve determinism and explain the choice.
- Revalidate ownership, profiles, blocklist, client configuration, and reservations at submission. A preview is not a reservation or a promise that all jobs will start.
- Report partial outcomes accurately: queued, already pending, invalidated since preview, or failed submission. Keep uncertain submissions on Sento's existing recovery path.

Lire's algorithm is a weighted greedy heuristic. It does **not** guarantee the smallest number of torrents or minimum bytes; size is a late tie-breaker. We should not promise an “optimal” plan either.

### 2. Search localized aliases — highest immediate backend value

Lire's [search helpers](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/services.py#L232-L280) query several names and merge results. Its metadata lookup also checks an explicit AniList link before accepting a MangaDex match, a useful principle for enrichment.

In Sento, `internal/service/search.go` passes all titles to providers, but `internal/provider/torrent.go:Search` sends only `req.Query` to each Torznab indexer. Source binding already tries alternatives; torrent retrieval is the specific gap. Matching aliases after retrieval cannot recover releases that were never returned.

Start with aliases already stored in MangaBaka and user-supplied titles. Query a small, deduplicated set, ordered by the selected edition's language, with per-indexer concurrency/rate budgets and caching. A fallback pass after insufficient coverage can control request volume. Add another metadata service only when a measured gap remains.

Keep offers distinct by indexer/GUID while grouping identical content by a reliable hash where available. The same torrent on two indexers can have different access, client routing, and failure behavior; merging must not discard the alternate offer or its provenance.

Expose the queries used and partial failures in search detail. Lire's aggregator discards exceptions, even when every query fails; that behavior should not be adopted.

### 3. Separate availability from ownership in the UI

Lire's [series view and shelf](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/main.py#L733-L780) make the relationship between owned, downloading, proposed, and missing volumes visible. The useful idea is the distinction between states, not the literal bookshelf styling.

Sento already has a richer book grid/list, pagination for long editions, and statuses such as upcoming, storage unavailable, and needs review. Keep those. Add a search/plan overlay rather than another permanent ownership status:

| Ownership state | Search observation | Useful wording |
| --- | --- | --- |
| Missing | Selected by this plan | Will download |
| Missing | Eligible release exists | Available |
| Missing | Only offers reporting zero seeders | No seeders reported |
| Missing | Offers fail profile constraints | Found, outside your profile |
| Missing | Successful search, no matching offer | Not found in this search |
| Missing | Sources failed or have not been searched | Availability unknown |
| Owned | Upgrade selected | Will upgrade |

Availability needs the search time and scope. An empty search is not evidence that a book does not exist, and a reported zero seeder count is not proof a torrent can never complete. Planned coverage must be calculated from distinct newly satisfied book/version pairs, not all numbers inside the selected packs.

This can be implemented with Mantine and the existing components. No UI library change is warranted. Keep advanced search one step away and keep Sento's stable row order during streaming.

### 4. Make inefficient packs a conscious choice

Lire's [follow planning](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/main.py#L962-L977) sends packs with mostly old volumes for human review.

The use case is directly relevant: the user owns 1–43 and only needs 44, but the available release contains 1–44. Sento should say “Adds volume 44; includes 43 volumes already owned,” with total bytes and an alternative when one exists.

Initially this is a planner explanation and optional automation guard, not a new matrix of settings. A fixed ratio of new books is only a proxy: one small pack may be acceptable, and two equally sized books can have very different byte costs. Unknown sizes should stay unknown. Do not hide the only viable release.

Keep monitoring each wanted book. Do not replace that with a moving highest-number baseline, which can forget gaps.

### 5. Optional reader handoff

Lire implements [per-user resume information and Kavita links](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/main.py#L1155-L1224). This closes a real gap between collecting a file and reading it.

For Sento, start smaller: an optional reader connection, a debounced scan after successful import, and “Open in Kavita” or another configured reader. Implement and verify one adapter first; Komga is a possible later adapter, not an existing Lire capability at this revision.

Reader synchronization must run after Sento commits a verified import. Store its failure separately and retry it; a reader outage should not undo ownership or trigger another download. Reader mappings should use external IDs and paths where possible, with reviewable fallbacks rather than title equality alone.

Reading progress is optional, belongs to an account, and is independent of Sento's file ownership. Full account provisioning, password sharing, a reader proxy, and a reading dashboard are larger product decisions. They are not prerequisites for a useful handoff.

### 6. Stalled-download recovery, introduced conservatively

Lire notices sustained lack of progress and searches for replacements. The user value is clear: a queue should not silently wait forever.

Start in Sento with a persisted last-progress time, a clear stalled state, and “Find another release.” Distinguish intentional pauses, queued jobs, client outages, and unavailable storage from a torrent making no progress. Keep policy and client-specific observations separate.

If automatic replacement is later enabled, require verified coverage of all still-needed books, the same edition/version policy, and successful reconciliation of the replacement's submission. Transfer reservations deliberately and avoid automatic file deletion. Rate-limit retries. The existing failed-download blocklist is a foundation, but a transient stall is not necessarily a permanently bad release.

### 7. Broader parser scenarios and loose-image imports

Lire has a useful collection of real release-shape examples: French lists/ranges, author prefixes, mixed volume-and-chapter names, spin-offs sharing a title, edition terms, and international volume markers. Sento already has a corpus and a more expressive parser; expand our independently written regression scenarios rather than replace the parser or import its expectations wholesale. An upstream rejection of chapters or extras may be a valid supported case here.

Lire's [image packing](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/pack.py) also addresses a real torrent payload: folders of page images. It leaves source pages in place for seeding and creates reader-friendly CBZs.

Sento already builds CBZs for extension downloads. The additional capability would be recognizing image-only torrent folders and feeding a reusable archive builder. Only do this when a folder maps confidently to one book; stage and validate the archive, retain the originals, and use Sento's non-overwriting import rules. Do not assume every numbered subfolder is a volume—chapter folders and scans split into parts need different handling.

## What should stay different

Sento's current model covers the use cases raised during its design:

| Concern | Lire at the reviewed revision | Keep in Sento |
| --- | --- | --- |
| Publication identity | Series plus language and a single variant label | Explicit editions and stable books |
| Multiple collected copies | One follow per series/language; variant filters releases | Independent collected versions and profiles |
| Numbering | Predominantly positive integer volumes | Chapters, fractions, extras, revisions, unknown numbers |
| Ownership | Reader/torrent observations unioned as numbers | Verified local assets and version-specific fulfillment |
| Queue lifecycle | Grab history plus downloader state | Reservations, frozen jobs, selected client, import review |
| Metadata outside catalog | Free-text comics search | Custom editions with explicit membership and overrides |

The worthwhile new abstraction is an **acquisition plan**, not another collection/edition layer. Plans are derived from current wanted books, offers, and policy; accepted jobs then become durable records. Search availability is similarly an observation with a timestamp, not an ownership fact.

Do not switch from MangaBaka to AniList to imitate Lire. Supplemental aliases/descriptions can have provenance without changing catalog identity. Do not collapse multiple edition traits into one string or treat standard and omnibus number 1 as interchangeable.

Accounts/SSO are worth a separate scope discussion if Sento becomes a shared request service. Discovery feeds, an installable PWA, and full reading-progress dashboards are secondary to reliable acquisition. Lire's PWA caches its shell, not an offline manga library. Its optional synopsis translation does not address Sento's main gaps.

## Reproduced limitations that inform these recommendations

These are reasons to adapt the ideas carefully, not a comprehensive security or correctness audit of Lire.

| Probe | Observed result | Consequence for Sento |
| --- | --- | --- |
| A completed standard volume 1 torrent; a deluxe volume 1 offer | Ownership becomes `{1}` and the deluxe plan is empty | Ownership must remain edition/version-specific |
| Stalled pack 1–3; only volume 1 has a replacement | `replace_dead` adds volume 1 and calls deletion for the old pack | Replacement must prove remaining coverage before retiring a job |
| Follow baseline 43; volume 45 arrives before 44 | Baseline advances to 45; a later volume 44 offer is ignored | Track individual wanted books, not only a high-water mark |
| Every alias query raises a connection error | Search aggregation returns an empty list | Preserve failed/partial search status |
| Own 1–2; proposed pack 1–3 | Plan coverage contains all three numbers, including owned ones | Separate physical contents from new coverage in totals |

Relevant upstream code: [ownership](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/main.py#L679-L699), [replacement](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/main.py#L878-L899), [follow advancement](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/main.py#L979-L1002), [query aggregation](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/services.py#L268-L280), and [plan coverage](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/plan.py#L108-L115). The deletion helper explicitly sends [`deleteFiles=true`](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/app/services.py#L346-L347); the probe mocked that call and did not delete real data.

Other source-level limitations: a single variant field cannot express independent edition traits; positive integer horizons synthesize a continuous sequence; missing language evidence falls back to a configured language (French by default). None should displace Sento's existing explicit identities and uncertainty handling.

To rerun the independent probes while the temporary files exist:

```sh
PYTHONPATH=/tmp/sento-lire-review-20261007/app:/tmp/sento-lire-review-20261007/tests \
  /tmp/sento-lire-review-venv/bin/python -m pytest -q /tmp/test_lire_review_probes.py
```

## Reuse and implementation order

The repository uses [PolyForm Noncommercial 1.0.0](https://github.com/Joopinhontas/lire/blob/1271458755f58b6dd352b09f53e54308cf41af60/LICENSE), which is not a permissive MIT/Apache-style license. Its grants are restricted to permitted purposes and include distribution/notice conditions. Directly importing its code, assets, or test corpus would need a licensing decision. Prefer independent implementations of the product ideas; no upstream code or assets were copied into Sento in this investigation.

Recommended sequence:

1. **Bounded torrent alias search**, retaining partial-error reporting and offer identity. Expand our parser scenarios alongside it.
2. **Acquisition planner and preview**, including disjoint job claims, profile-aware choices, overlap explanations, and truthful per-book availability. Reuse the same planning service for interactive and automatic acquisition.
3. **Optional reader handoff**, with a durable/retryable post-import scan and a verified reader link.
4. **Stall detection and guided recovery**; consider automation only after the lifecycle is proven.
5. **Loose-image torrent imports**, if actual library/download examples justify the added import path.

The largest visible improvement is step 2. The smallest useful first change is step 1. Both fit the current Go/Mantine architecture and preserve the domain decisions already made.
