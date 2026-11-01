# Exploratory missions

Follow [the brief](../exploratory-testing.md). These are user goals and checks,
not a rigid click script. Investigate deviations and vary the action order.
Record individual subcases; a mission is not passed if a required subcase is
untested. A full offline pass covers M1–M6. Real-source checks are supplemental.

## M1 — Find the right release and source matches

Use Directory Kingdom (105), Scenario Source, and `nested-listings`.
Search its volume edition from the UI. Inspect the root and both child matches,
including `!(Digital) [1r0n]` and `!ED-Japanese Names (Digital) [1r0n, Bingbong]`.
The adapter deliberately repeats a child: Sento must retain distinct folders
without duplicate bindings or releases. Inspect profile scoring and acceptance;
a root scan must not hide the Digital candidates. “Japanese Names” describes
retained source style, not a Japanese-language claim.

Disable one match, search again and reload: the other matches still work and
the disabled match stays disabled. Search twice; bindings must not accumulate.
Try a near-title collision (Stub Blade versus Stub Blade: Afterstory), and an
explicitly conflicting language/edition using a local profile/edition.
Record fit, ambiguity and rejection explanations. No automatic grab may rely on
an ambiguous edition match. This simulator checks the adapter protocol; raw
Madokami HTML traversal is a separate JVM/real-source check.

## M2 — Understand and recover from a source failure

Use Recovery Chapters (106). Collect chapters from 1, search Scenario Source,
and choose one chapter. Use a new chapter/ref or a fresh reset for each fault:

- `page-503`: page 5 fails twice then works. Expect one imported CBZ, exactly
  three attempts for page 5, and one request per successful page. Verify its
  five image entries and ComicInfo; no duplicate file or coverage after reload.
- `rate-limit`: first page-5 attempt returns 429 with Retry-After=2. Compare
  journal timestamps; no later attempt may start before that deadline. Grab
  another chapter to observe that the source shares its cooldown.
- `long-cooldown`: Retry-After=60. Observe “Waiting for source,” not “Stalled.”
  Restart **sandboxapi only**, preserve fixture state, and inspect recovery.
  Report whether timing and state survive; never assume they do.
- `truncated-page`: page 5 ends before Content-Length twice. An incomplete
  image must not be silently accepted as a finished book.
- `permanent-401`: one failed page request, then Review with the underlying
  cause; no repeated authentication attempts and no blocklist entry.
- `source-outage`: exhaust four page attempts, observe persisted queue backoff,
  retry count, retry-at and reservations. Restart sandboxapi during retry_wait;
  it must not immediately request pages. Switch to `healthy` without resetting
  counters to model recovery. Also exercise the five-queue-retry bound through
  real elapsed backoff, or explicitly mark that long subcase untested and cite
  the automated queue-bound regression separately. Do not fast-forward SQLite.

Try cancelling a waiting job and retrying a Review job through offered actions.
Check pending/owned/Wanted and the history after each. Job age alone must not
blocklist a source release. Do not claim successful pages persist across an
entire failed chapter attempt or process restart: currently they survive
individual page retries; an exhausted chapter attempt cleans them up.

## M3 — Send a torrent to the intended client

Add two clients: regular (`/qbit`) and VPN (`/qbit-vpn`), `stub` credentials,
`sento-test` category. Add a fake Nyaa Torznab indexer at `/torznab/api`, English,
using VPN. Search a volume and submit a release through the UI.

Inspect `/_control`: its hash must exist only in VPN, and a `torrent-add` event
must name VPN and the right category. Change Nyaa's selection to regular. The
existing job must remain tracked in VPN; a different new release goes to
regular. Reload and restart sandboxapi, then verify both jobs again.

Remove the selected client and try another grab: Sento must refuse with an
actionable reason, not fall back. Use `/_control/torrent` to pause/stall/complete
a fake torrent. Look for a replacement, then cancel the search: the old job and
reservation remain. Select a replacement and verify retirement happens once.

Cancel a queue job; its fake torrent must remain. Exercise the series removal
options separately on this run's fixtures: keep data versus remove data, keep
versus remove torrents, correct client and exact hashes. Inspect deletion
journal flags. Fake torrents carry no payload; mark actual file-deletion
verification under M5, not as proved by a successful fake-client response.

## M4 — Keep editions and profile-based copies coherent

Use Standard and a local Deluxe edition with different numbering; collect the
same edition with two profiles. Require a group/format in one, refuse it in the
other, then change the policy after importing a file or queuing a job. Verify
search eligibility, rejection owner/reason, existing-file fulfillment and
Wanted agree with the current profiles. A force action must not bypass hard
profile gates. Where profiles overlap, do not assume separate cards promise
exclusive file ownership; inspect what the dialog explains.

Change inherited versus explicit profiles, chapter cutoff (including zero),
unknown-date handling, individual monitoring and extras. Distinguish owned,
pending, missing, upcoming and unknown. Remove one card while keeping another;
then test the last-card behavior. Reload and restart to verify saved intent.
Try One Piece-like long lists, descending/ascending order, filters and mobile.

## M5 — Import, organize, rescan and remove files without losing them

Use successful source imports to obtain real local CBZs. Keep Standard and
Deluxe and chapter/volume paths separate. Preview naming and organize, inspect
actual paths and ComicInfo, then rescan twice. Coverage and file identities
must remain stable; no duplicate files or invented ownership. Copy a tagged
fixture into a separate worktree-local import folder and import it: Sento
should identify its series/edition from metadata, including into an existing
series. Explicitly record whether a test used Sento tags or only filenames.

Exercise a boxset modeled as unit 0 containing 1/2 plus an extra, an overlapping
pack, a loose-image folder, and an unreadable or missing input. Keep ambiguous
files inspectable. Hash originals before/after repeat import and organization;
verify bytes remain valid (metadata updates may legitimately change a library
copy). Inspect pending reservations during overlap; one wanted book must not
be assigned to two simultaneous jobs in the same collected version.

Test remove-series keep-files versus delete-files using only local fixtures
this run created. Verify both the database and actual filesystem. A reader's
ability to ingest the naming is judged from the output layout/metadata, not by
modifying a real Codex/Kavita instance. If a needed payload is unavailable,
record the subcase untested instead of faking its result.

## M6 — Finish Review and explain the result

Create a partial/mismatched import (a completed fake torrent has no payload,
which exercises the failure path; use local tagged files for mapping). Follow
file suggestions, correct a wrong mapping, keep an extra without claiming a
book, and resolve the last outstanding file. Verify the held job closes when
its work is complete and stale “0 of N” errors stop contradicting ownership.

Distinguish file Review, download Review, chapter proposals and catalog-link
suggestions. Every item must explain its goal and offer a usable next action.
Try accept-one, accept-all where offered, dismiss, retry and cancel. Verify the
remaining count and reload. A problem that requires API/SQL repair is a finding,
not a successful UI recovery. Recheck related issue reports with evidence.

## Open exploration

After the missions, try at least two unplanned combinations and follow any
unexpected result to its durable state. Include keyboard/mobile interaction,
a pending operation, and an undo/revisit/reload where possible. Preserve
surprises that worked as well as failures. Append these journeys to the report.
