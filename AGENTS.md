# Agent notes

Code map and data flows: [`docs/architecture.md`](docs/architecture.md). Tasks
live in [`mise.toml`](mise.toml) — use `mise run <task>`, never bare `go`/`npm`
with a system toolchain (versions are pinned there). New worktrees already ran
`mise install && mise run web:install` (see `paseo.json`).

## Running the app (dev)

On a local checkout, the dev stack is three mise daemons (`[daemons]` in
`mise.toml`), supervised by pitchfork. Each builds what it needs before it
starts and gets this checkout's own ports:

```sh
mise run dev        # build + start extensions, api, web, then follow their logs
mise run dev:stop   # stop everything this checkout started
mise daemons urls   # the ports (and proxy URLs) of this checkout
```

Ctrl-C in `mise run dev` only leaves the log view; the stack keeps running.
The `api` daemon rebuilds itself on a Go change; a failed build logs
`BUILD FAILED` and keeps serving the previous binary.

Under Paseo, the same three run as the services declared in `paseo.json` —
Paseo assigns each worktree its own ports and calls the same mise tasks:

```sh
paseo script start extensions   # JVM sidecar (minutes the first time, seconds after)
paseo script start api          # Go backend, pointed at this worktree's sidecar
paseo script start web          # Vite, proxying /api to this worktree's backend
paseo script ls                 # ports + proxy URLs; open the web one
```

The Paseo `api` service rebuilds and restarts itself on a Go change too
(same port, so `web` keeps proxying); a failed build logs `BUILD FAILED`
and keeps serving the previous binary. Stopping and starting a service
gets it a new port: restart `web` after restarting `api`.

Either way, don't background processes by hand (`nohup`, `&`): they outlive
the session and nothing stops them.

- Open the **web** URL, not the backend's. The backend only serves the UI
  from `web/dist` (`mise run web:build`); without it, `GET /` there is a 404
  — that is expected, not a failed start.
- Smoke check: `curl <web>/api/v1/series` → `200 []` on a fresh DB.
- `extension reconcile failed … connection refused` at backend startup means
  the sidecar isn't up (yet). Everything except chapter sources works without it.
- Ports are env-driven: `SENTO_PORT`, `SENTO_EXTENSIONS_PORT`,
  `SENTO_EXTENSIONS_URL`, and `SENTO_API_URL` (Vite's proxy target). The
  daemons wire these from their assigned ports; Paseo injects them per
  service; a bare terminal uses the defaults (8484 / 8901 / 5173).
- Machine-specific settings (qBittorrent, Prowlarr keys, …) go in
  `mise.local.toml` (gitignored), not `mise.toml`.

## Checks

`mise run ci` runs everything CI does. Narrower: `mise run test`, `mise run lint`,
`mise run lint:web`, `mise run e2e` (Playwright against stubbed upstreams —
no network or sidecar needed). After changing Go API structs, run
`mise run gen` and commit `web/src/types`.
