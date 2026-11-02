---
name: exploratory-test
description: Run Sento's exploratory correctness and usability pass in an isolated worktree, exercising controlled upstream failures and real UI workflows, recording evidence and filing findings. Not for ordinary development or broad product fixes.
---

# Sento exploratory pass

Read `docs/exploratory-testing.md` as the mission brief, then
`docs/testing/exploratory-missions.md` and
`docs/testing/exploratory-scenarios.md`.

- Default to the offline stack: `paseo script start fixtures`, then
  `sandboxapi`, then `sandboxweb`. Use `paseo script ls` and post all three
  proxy URLs in your first progress update after startup. Browse sandboxweb.
  Never hand-background servers or assume a particular browser tool exists.
- Verify isolation, select controlled faults through the fixture service,
  and execute M1–M6. Start with matching, recovery and routing. Use the UI
  for the behavior under review; disclose API setup, and use read-only API,
  request-journal and filesystem checks to verify durable outcomes.
- Try at least two unexpected combinations beyond the missions. Follow
  failures through recovery/reload/restart instead of stopping at a click.
  Do not repair SQLite, intercept Sento's API, or reset upstream counters
  during a measured recovery to manufacture a pass.
- Only run real-upstream smoke checks when explicitly requested. Use your
  own isolated dev instance, test credentials and small downloads as described
  in the brief. Never use production services or the real library.
- Leave `data/exploratory/report.md` using the report template. Mark each
  mission passed, failed, blocked or untested; identify skipped subcases.
  Link redacted evidence and record resources left behind. Do not call a
  blocked pass clean or treat old reports as current defects.
- Report rather than rework. A small fix may ride along in a worktree PR;
  never push main during an exploratory pass. Deduplicate findings and file
  GitHub issues only when requested; do not assume access to a maintainer token.
