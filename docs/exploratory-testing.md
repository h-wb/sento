# Exploratory testing

Use the app with a concrete reader's goal: add a series, choose an edition,
find a release, recover from a failed download, or resolve a review item.
[The missions](testing/exploratory-missions.md) give repeatable starting points;
try unexpected combinations as well. Current rules live in
[editions](domain/editions.md) and [acquisition](domain/acquisition.md).

## Start an offline session

The default is the real backend and UI against controllable, local MangaBaka,
extension, Torznab and qBittorrent upstreams. No credentials or real downloads
are needed. In a fresh worktree after normal developer setup:

```sh
mise run explore
mise daemons urls
```

Open **sandboxweb**. Its `/api` requests go to **sandboxapi**. The **fixtures**
service is a test control surface, not the app. Stop the stack with
`mise run explore:stop`; Ctrl-C only leaves the log view. Paseo users can start
`fixtures`, `sandboxapi` and `sandboxweb` through its service controls instead.

The database, library and staging directories are under `data/exploratory/`,
separate from the normal dev stack. Restarting the backend preserves them.
Use a fresh worktree for a new pass; preserve state that demonstrates a bug.

Record the commit, browser, viewport and all three service URLs. Use the
**internal** fixture URL (`http://127.0.0.1:<port>`) in Connections and indexers,
so backend requests do not depend on a browser-authenticated proxy. Confirm
`GET <fixtures>/_control` identifies itself as `sento-e2estub`. If no browser
is available, mark UI missions blocked; API tests do not establish usability.

## Control the upstreams

Faults belong at the upstream boundary, not in mocked Sento responses.
The simulator exposes controls only in its `e2e` build with `-control`.
[The scenario reference](testing/exploratory-scenarios.md) lists the controls:

```sh
curl -fsS -X POST <fixtures>/_control/scenario \
  -H 'Content-Type: application/json' -d '{"name":"page-503"}'
curl -fsS <fixtures>/_control
```

In the UI, add a fixture repo such as `https://repo.example/index.pb` (handled
offline). A non-healthy scenario exposes **Scenario Source** in the picker.
It offers **Directory Kingdom** (105) and **Recovery Chapters** (106); the
normal Stub Blade and Deterministic Diaries fixtures remain available.
Do not add real extension repositories to this offline stack.

Use `<fixtures-internal>/torznab/api` with English language for a fake torrent
indexer. Independent clients at `/qbit` and `/qbit-vpn` use `stub` credentials
and category `sento-test`; they journal adds/deletes but transfer no data.
Source fixtures serve tiny images so CBZ assembly and import are exercised.

Changing a scenario preserves attempts and events, modeling a recovering
upstream. `POST /_control/reset` clears them and fake torrents, but leaves
Sento's database and installed extension configuration intact. Save evidence
and stop sandboxapi before resetting. Do not reset during an observed retry
or describe a reset as an application repair.

## Exercise the workflow

Start with matching, recovery and routing (M1–M3), then complete M4–M6.
For each mission, state the goal and expected result, act through the UI,
reload, and check persisted state. Read-only API, request-journal and file
checks support the evidence. Disclose API-seeded setup; it does not prove
that equivalent UI setup works.

Try at least two combinations beyond the missions: change a profile with a
job pending, search in two tabs, cancel while a request completes, toggle a
source and reload, or switch between desktop and a 390px viewport mid-dialog.
Include keyboard use, dirty forms and long errors. Inspect generated files;
connecting a reader is outside this pass.

Do not edit the database or clear reservations to force success. Retry,
cancel, rescan and run-now are normal user actions. A failure that recovers
still deserves a usability check: could the user understand what happened?

## Optional real-upstream smoke checks

Only use real upstreams when the run explicitly requests them, after the
offline checks. Use your own separate dev instance and test credentials,
with a worktree-local database, library and staging area. Verify the selected
client, test category and readable torrent mounts before grabbing anything.
Never copy production settings or point the offline stack at real providers.

Keep downloads small; do not force rate limits or repeatedly hammer a source.
Operate only on torrents and files created by this run. Record hashes and
paths before cleanup. Canceling queue tracking and deleting a torrent or
its data are separate actions; preserve originals unless a test explicitly
requires deleting its own files. Missing credentials or mounts are environment
blockers, not importer defects.

## Record findings

Use [the report template](testing/exploratory-report.md) at
`data/exploratory/report.md`. Mark each mission **passed**, **failed**,
**blocked** or **untested**. A pass needs outcome and persistence checks.
Preserve screenshots, redacted API responses, request-journal excerpts and
file metadata. Do not include credentials.

Prioritize data loss or incorrect ownership, wrong matches or routing,
recovery dead ends, misleading labels, then friction. Distinguish upstream
outages, environment limits and product defects. The final report should
link the evidence and list coverage gaps and resources left running.

Search existing issues before filing a confirmed finding with
[the finding form](../.github/ISSUE_TEMPLATE/exploratory-finding.yml). Reproduce
older reports against the current version before treating them as open bugs.
File issues when requested; do not automatically open issues or change live
services during a test pass. Add regression tests during the subsequent fix.
