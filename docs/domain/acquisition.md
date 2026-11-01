# Acquisition planning

Sento collects and manages files. **Reader integration is explicitly out of scope**: no embedded reader, reader connection, reader scan, reading progress, or reader handoff. This is a standing product decision confirmed on 2026-10-07. It supersedes the reader proposal in the Lire comparison.

Acquisition plans are derived from a search and the selected edition/version's books. They are not another collection model. Book identity remains `(versionID, bookID)`; chapter numbers and volume numbers are never interchangeable.

The plan distinguishes the release's contents from the subset of wanted books assigned to a job. Overlapping packs can therefore have disjoint reservations. Import still verifies file contents and preserves the original payload. Existing profile gates, edition checks, blocklists, and uncertain-dispatch handling remain authoritative.

The pack rule applies to a job's share as well as to the release: the default plan, which automatic acquisition also uses, never picks a pack whose remaining contribution is under half of the books it holds. Those books stay listed as available, and a person can still select the pack; the plan then says why it was not chosen.

Search availability is a timestamped observation, separate from ownership: available, outside policy, no seeders reported, not found in this search, or unknown after source failures. A proposed download does not establish ownership.

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
Permanent source errors go directly to Review. Neither outcome blocklists
the release. A chapter's 30-minute timeout starts when a download slot opens.
