# Acquisition planning

Sento collects and manages files. **Reader integration is explicitly out of scope**: no embedded reader, reader connection, reader scan, reading progress, or reader handoff. This is a standing product decision confirmed on 2026-10-07. It supersedes the reader proposal in the Lire comparison.

Acquisition plans are derived from a search and the selected edition/version's books. They are not another collection model. Book identity remains `(versionID, bookID)`; chapter numbers and volume numbers are never interchangeable.

The plan distinguishes the release's contents from the subset of wanted books assigned to a job. Overlapping packs can therefore have disjoint reservations. Import still verifies file contents and preserves the original payload. Existing profile gates, edition checks, blocklists, and uncertain-dispatch handling remain authoritative.

The pack rule applies to a job's share as well as to the release: the default plan, which automatic acquisition also uses, never picks a pack whose remaining contribution is under half of the books it holds. Those books stay listed as available, and a person can still select the pack; the plan then says why it was not chosen.

Search availability is a timestamped observation, separate from ownership: available, outside policy, no seeders reported, not found in this search, or unknown after source failures. A proposed download does not establish ownership.

An indexer being reachable does not prove its search feed contains books.
**Test search** on the Indexers page runs the same Books query used by
acquisition. If that returns nothing, it compares an unrestricted query and
shows sample titles and categories. These diagnostic results never enter
acquisition plans or start downloads; they help distinguish an empty query
from a feed whose category settings need attention.

Saving a source preference rebuilds its loaded instance before the save
finishes. The source stays discoverable alongside other installed sources,
including when saved preferences are restored after a sidecar restart.

The common interaction is a coverage summary and editable selection of releases, with raw release details and rejected candidates available below. A provisional plan accompanies each batch of results so the view fills in during the search; it becomes editable when the search ends. Submission searches and evaluates again and reports each release's outcome; it never silently substitutes a different release.

Stall detection reports sustained lack of progress, excluding deliberate pauses and queued/checking states. Recovery is guided: a replacement search treats the stalled job's books as wanted and rules out the job's own release, while the job keeps running and keeps its reservations. Sento stops tracking the old job only when a chosen replacement claims one of its books; a search that finds nothing changes nothing. The torrent and its files are never removed from the client, and no replacement is made automatically.

Loose-image imports must map a folder confidently to one book, order and validate its pages, build a staged CBZ, and pass through the normal non-overwriting import path. The source pages remain untouched for seeding. Ambiguous folders require review.

Source downloads retry temporary HTTP failures (408, 425, 429 and 5xx),
network failures and truncated page bodies for up to four attempts per page.
Each attempt uses the source's shared page rate limiter; increasing delays
with jitter and upstream `Retry-After` pause page requests across that source.
Successful pages stay staged during another page's retries. An exhausted
chapter attempt cleans up its temporary files and enters the persisted queue
backoff (30 seconds, doubling up to 32 minutes, or longer for `Retry-After`).
After five queue retries it moves to Review with its book reservations intact.
Other permanent source errors go directly to Review. Neither outcome
blocklists the release. A source that cannot give the release is different:
one that no longer has the listing or a page of it (404, 410), or serves
an archive with an explicit ZIP format error. The job fails and the release
is blocklisted, like a torrent that never completes, and the series is
searched again within the minute for another (the same happens when you ask
to try another release).

An IO-wrapped range/header failure, including "Not a local file header",
remains retryable: a transient response can fail that check even for a valid
archive. Upgrades remove automatic blocks created by the old classification,
preserving manual blocks and evidence that a release is gone. Historical jobs
stay failed; ordinary acquisition decides whether to try the release again.
Confirmed source 404/410 failures move to Activity History while their books
remain wanted for replacement searches. They do not remain tasks merely
because no replacement exists. Downloaded files with unresolved review
questions still require attention. Other failures retain the normal
coverage-based resolution rule.
A chapter's 30-minute timeout starts when a download slot opens.

Blocked releases can be reconsidered from Activity → History → Blocked
releases, individually, as a selection, or all at once. The list groups a
release's known listing, download and feed identifiers into one entry and
shows its title, block reason and date. Clearing all blocks asks for
confirmation and includes manual blocks and releases outside the current
page or filter. Search results rejected by the blocklist also offer
**Unblock**, then search again to evaluate current policy and ownership.
Unblocking removes all known blocked identifiers of the chosen release;
it leaves previous attempts, files and reservations unchanged. Releases
become eligible for normal searches and automatic acquisition again.
Blocks retain their title and protocol-scoped aliases independently of queue
history, so housekeeping cannot split a release into separate blocks.
Upgrades recover existing aliases from retained queue history; blocks with
no remaining identity evidence stay separate. New blocks save their full
alias group atomically, including relationships learned by earlier attempts.

An archive-range read failure is not evidence of permanent damage; Activity
labels it as an archive read error and retains the queue's actual retry or
failure phase. An empty page list says that no file was downloaded, rather
than presenting it as an import decision. A 404/410 means the download or a
required page is unavailable; uncovered books still need a replacement.

Partial imports show accepted books as owned and identify the requested
books that remain unmatched, while file questions and their reservations
remain intact. **Find releases for unmatched books** uses the normal guided
replacement search: it excludes the original release, leaves its files and
job intact until another release is selected, and does not reacquire books
already accepted from the partial import. Retry times interpret stored UTC
timestamps consistently with the other Activity dates.

Reviewing an overlapping pack preserves the existing selected archive unless
replacement was explicitly requested in the acquisition snapshot or chosen in
review. Grab snapshots name requested replacements only for the selected books
that already have verified, present owners; surplus books in a pack do not gain
replacement intent. Automated imports continue to apply the acquisition plan and
configured upgrade policy. Review separately confirms archive contents, chooses
each collected book's supplying copy, and retains or trashes unused incoming
archives. See [review and file ownership](editions.md) for preview validation,
recoverable trash, and durable skip behavior.
