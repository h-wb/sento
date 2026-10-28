# Agent notes

Code map and data flows: [`docs/architecture.md`](docs/architecture.md). Tasks
live in [`mise.toml`](mise.toml) — use `mise run <task>`, never bare `go`/`npm`
with a system toolchain (versions are pinned there). New worktrees already ran
`mise install && mise run web:install` (see `paseo.json`).

## Running the app (dev)

Under Paseo, run the three services declared in `paseo.json` — Paseo
supervises them and gives each worktree its own ports:

```sh
paseo script start extensions   # JVM sidecar; builds first (minutes the first time, seconds after)
paseo script start api          # Go backend, pointed at this worktree's sidecar
paseo script start web          # Vite, proxying /api to this worktree's backend
paseo script ls                 # ports + proxy URLs; open the web one
```

Don't background processes by hand (`nohup`, `&`): they outlive the session
and nothing stops them. Logs are in each service's terminal.

Outside Paseo, the same pieces on their default ports, each in its own terminal:

```sh
mise run extensions:build && mise run extensions:run   # :8901 (optional — only chapter sources need it)
mise run run       # Go backend on :8484 (SQLite at data/sento.db, library at data/library)
mise run web:dev   # Vite on :5173, proxies /api to :8484
```

- Open the **web** URL, not the backend's. The backend only serves the UI
  from `web/dist` (`mise run web:build`); without it, `GET /` there is a 404
  — that is expected, not a failed start.
- Smoke check: `curl <web>/api/v1/series` → `200 []` on a fresh DB.
- `extension reconcile failed … connection refused` at backend startup means
  the sidecar isn't up (yet). Everything except chapter sources works without it.
- Ports are env-driven: `SENTO_PORT`, `SENTO_EXTENSIONS_PORT`,
  `SENTO_EXTENSIONS_URL`, and `SENTO_API_URL` (Vite's proxy target).
- Machine-specific settings (qBittorrent, Prowlarr keys, …) go in
  `mise.local.toml` (gitignored), not `mise.toml`.

## Checks

`mise run ci` runs everything CI does. Narrower: `mise run test`, `mise run lint`,
`mise run lint:web`, `mise run e2e` (Playwright against stubbed upstreams —
no network or sidecar needed). After changing Go API structs, run
`mise run gen` and commit `web/src/types`.
