---
name: exploratory-test
description: Load when asked to run Sento's exploratory usability testing pass — a session (typically in a Paseo worktree) that boots the stack, uses the app like a long-time Sonarr user, and files findings as GitHub issues. Not for normal development or code fixes beyond a small ride-along PR.
---

# Sento exploratory testing pass

Your mission brief is `docs/exploratory-testing.md` in the repo root — read it
first and follow it exactly. It carries the user persona and expectations
charter, the manga corpus, the hard limits (Nyaa only, `sento-test`
category, Weeb Central only, worktree-local library), and the issue-filing
rules.

Operational entry points:

- **The stack**: `paseo script start extensions api web` (a fresh worktree's
  first api start includes a Go build — give it a few minutes), then
  `paseo script ls` for the ports and proxy URLs. Open the **web** URL, not
  the backend's; `GET /` on the backend is a 404 by design.
- **Read `docs/dogfooding.md` first**: sharpen its entries with repro and
  evidence instead of filing duplicates; a finding that matches no entry
  gets a fresh issue.
- **Report, don't rework**: findings are the deliverable. A small, obvious
  fix may ride along as a pull request from a branch — never a direct push
  to main.
- **Every GitHub write goes through the scoped token**:
  `mise x --secrets GH_TOKEN -- gh issue create --label usability …`
  (the repo-scoped PAT from Proton Pass — not the pod's broader token).
