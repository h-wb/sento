# Exploratory testing

This is the agent's mission brief for testing Sento's correctness and usability.
Use the app as a long-time Sonarr/Radarr user discovering Sento for the first
time. Follow [the missions](testing/exploratory-missions.md), investigate
unexpected combinations, and deliver an evidence-backed report. Human
observations live in [dogfooding.md](dogfooding.md); current product rules are
in [domain/editions.md](domain/editions.md) and
[domain/acquisition.md](domain/acquisition.md).

## Kickoff and target

A pass gets a disposable worktree. The default is **offline**: the real Sento
backend and UI, with controllable MangaBaka, extension, Torznab and qBittorrent
upstreams. No credentials or real downloads are needed.

```sh
paseo run --provider mistral-vibe --new-workspace worktree \
  --title "Sento exploratory pass" \
  "Run the exploratory-test pass using docs/exploratory-testing.md. Start in offline mode, execute the missions, explore unexpected combinations, and leave a report with evidence."
```

Boot only through the supervised services. Each service gets this worktree's
ports and proxy URLs; no hand-backgrounded processes:

```sh
paseo script start fixtures
paseo script start sandboxapi
paseo script start sandboxweb
paseo script ls
```

Post the **sandboxweb**, **sandboxapi**, and **fixtures** proxy URLs in your
first progress update after startup. Browse sandboxweb; its `/api` points to
sandboxapi. Fixtures is a test control surface, not the application under test.
Also record its internal URL `http://127.0.0.1:<fixtures-port>` from the service
listing. Use that internal URL for Connections/indexers: backend requests must
not depend on a browser-authenticated proxy.
These services use `mise run explore:fixtures` and `mise run explore:api`.
The separate database, library and staging files live in `data/exploratory/`.
A backend restart preserves that database and the separately running upstreams.
Use a fresh worktree for a fresh pass. Never wipe evidence to make a failure pass.

Preflight before exercising downloads:

- Confirm the UI's Connections point to this run's internal fixture URL prefixes:
  `/host`, `/prowlarr`, `/qbit`, and (when added) `/qbit-vpn`.
- Confirm `GET <fixtures>/_control` identifies itself as `sento-e2estub`.
- Record commit, browser/tool, viewport, URLs and mode in the report. Check
  browser availability rather than assuming a particular tool or OS package
  exists. Use the connected browser tool (Paseo browser or Chromium/CDP).
  If no usable browser exists, mark UI missions blocked; API checks do not
  establish usability.
- Read dogfooding entries as historical observations. Reproduce them against
  the current app and classify fixed, still reproducible, or not retested.
  An old checkbox is not evidence that a bug remains.

The Playwright suite (`mise run e2e`) is the repeatable regression layer. It
can run where browser dependencies are installed; report an environment blocker
if they are missing. It does not replace the exploratory session.

## Controlling the upstreams

The simulator's controls exist only in the `e2e` build and with `-control`.
They change upstream responses, never Sento's database or API responses.
[Scenario reference](testing/exploratory-scenarios.md) describes the controls.
For example, substitute the published fixture URL:

```sh
curl -fsS -X POST <fixtures>/_control/scenario \
  -H 'Content-Type: application/json' -d '{"name":"page-503"}'
curl -fsS <fixtures>/_control
```

Use the UI to configure **Scenario Source** (add any fixture repo URL first;
`https://repo.example/index.pb` is handled offline). Choose a non-healthy
scenario to make the extension appear in the picker. The source offers
**Directory Kingdom** (catalog id 105) and **Recovery Chapters** (106).
The normal Stub Blade and Deterministic Diaries fixtures remain available.
Do not add the real Keiyoushi repo to the offline stack.

Use the internal fixture Torznab URL `<fixtures-internal>/torznab/api`, language English,
for a fake Nyaa indexer. Two independent clients live at `<fixtures-internal>/qbit`
and `<fixtures-internal>/qbit-vpn`; both use `stub` credentials and category `sento-test`.
They keep separate torrents and journal adds/deletes. They transfer no data.
Source fixtures provide real tiny images so Sento can assemble and import CBZs.

Selecting another scenario preserves attempts and events: this models an
upstream recovering while the app restarts. Resetting is different. Before
`POST /_control/reset`, stop sandboxapi and save the report/evidence. Reset
clears fault counters, events and fake torrents; it does not erase Sento's DB
or installed extension configuration. Restart sandboxapi afterwards. Do not
reset during an observed retry, or claim the reset repaired an app defect.

## How to explore

Start with matching, recovery and routing (M1–M3), then complete M4–M6.
For each mission: state the user's goal and expected outcome, take the actions
through the UI, inspect the result, reload, and verify the relevant persisted
state. Read-only API, request-journal and file checks strengthen the evidence.
API setup is allowed for repeatable fixtures, but disclose it: an API-seeded
workflow does not prove the equivalent UI setup works.

Do not intercept Sento's API to demonstrate integration correctness. Faults
belong at the upstream boundary. Do not edit SQLite, clear reservations, or
call hidden repair endpoints to make a mission succeed. Normal user actions
such as retry, cancel, rescan and run-now are part of the test.

Follow promising surprises. Try at least two combinations beyond the scripted
missions: change a profile with a job pending; search in two tabs; cancel while
a request completes; toggle a source match and reload; switch mobile/desktop
mid-dialog; undo a selection; revisit after restart. Record what you tried,
not a claim of exhaustive coverage. Reproduce failures on a fresh fixture when
possible while keeping the original state inspectable.

The *arr expectations still apply: identify the right series; explain missing
versus upcoming; explain acceptance/rejection; make the queue actionable; close
Review when the work is resolved; preserve files; keep editions separate in
paths and metadata; and retain settings after restart. Inspect at desktop and
390px mobile widths, including keyboard use, dirty dialogs and long errors.
Validate the files Sento produces; connecting to a reader is out of scope.

## Real-upstream smoke mode

Use this mode when the run requests real-upstream coverage, after the offline
missions. Start `extensions`, `api`, then `web` through Paseo and post their
URLs separately. They use the worktree's normal `data/` DB and library, not
`data/exploratory/`. Do not connect the sandbox services to real providers.
The api service seeds Prowlarr and the VPN qBittorrent endpoint, credentials
from fnox, and category `sento-test`. Saved settings win over seed env values:
verify Connections and Nyaa's selected client in the UI before grabbing.

Limits:

- **Production Sento is off-limits**: no reads, changes, cancellations or
  downloads at the live instance or its cluster API. Never use the real
  library or another worktree's files.
- **Nyaa only** for a real torrent indexer. Select the test VPN client explicitly
  and verify category `sento-test`; never rely on a client name or default.
  Operate only on this run's recorded test hashes, even if the shared test
  category contains other runs' torrents. Do not change global client settings.
- **Weeb Central** for a small real chapter download. **Madokami** for metadata
  search and nested-listing smoke checks, using credentials configured for the
  test instance. If unavailable, mark that smoke check blocked; do not read or
  copy production settings. Do not force live 429s or repeatedly hammer sources.
- Keep the library and staging directories worktree-local. Before importing a
  remote torrent, establish that its files are mounted and readable here and
  its path belongs to the test category. An unavailable mount is an environment
  blocker, not proof the importer is broken.
- Record hashes, paths and ownership before cleanup. Queue cancellation only
  stops tracking; it does not remove a torrent from its client. Distinguish
  cancellation, removing a torrent, and deleting data. Preserve originals
  unless a mission explicitly tests deletion of files this run created.

Real corpus: Kingdom (nested Digital editions); Kagurabachi (chapter identity);
Berserk and Berserk of Gluttony (edition/title collision); One Piece (long lists);
Nausicaä (boxset 0 versus volumes 1/2); Dragon Ball (complete packs);
Hunter×Hunter (hiatus); and a one-volume one-shot. Test current catalog/source
behavior rather than hardcoding mutable publication totals.

## Findings and deliverable

Classify findings in this order: data loss or incorrect ownership; wrong match,
client, coverage or policy; duplicate/abandoned work and recovery dead ends;
wrong labels; hidden narratives; friction; raw internals. Distinguish an
upstream outage, environment limitation, product defect, and usability problem.
A failure that recovers is still worth judging: did the user understand it?

Leave `data/exploratory/report.md` using
[the report template](testing/exploratory-report.md). Every mission must be
**passed**, **failed**, **blocked**, or **untested**, with evidence. A pass
requires the outcome and persistence checks, not merely clicking Save. Name
unexecuted subcases. Preserve screenshots, relevant redacted API responses,
request-journal excerpts, file paths/metadata and before/after states. Never
include credentials. End the user-facing summary with the report link,
findings, remaining coverage gaps, and the state/resources left behind.

Report, don't rework: findings are the deliverable. A small obvious fix may
ride along in a PR from the worktree; exploratory sessions never push main.
Search existing GitHub issues and dogfooding notes before filing. Add evidence
to an existing issue instead of opening a duplicate. Known-fixed observations
are regression checks, not fresh issues. Filing is authorized as part of this
explicitly invoked harness. Use the [finding form](../.github/ISSUE_TEMPLATE/exploratory-finding.yml)
and the scoped repo token for every GitHub write:

```sh
mise x --secrets GH_TOKEN -- gh issue create --label usability \
  --title '<one finding>' --body-file <filled finding.md>
mise x --secrets GH_TOKEN -- gh pr create --body-file <description.md>
```

A finding needs mission/scenario, commit, exact repro, expected/actual state,
impact, evidence, related issue and recovery attempted. Do not claim a run is
clean when missions are blocked or untested. Turning confirmed bugs into
regression tests happens during the subsequent fix.
