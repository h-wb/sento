# Web UI

The React single-page app that talks to the Sento API (`/api/v1`). Source in
`src/`; the full architecture of the app (providers, routing, server state,
streaming search, dialogs, styling, generated types) is documented in
[docs/web.md](../docs/web.md).

## Commands

All commands run through mise from the repository root, so the pinned Node
toolchain is used:

| Command | What it does |
| --- | --- |
| `mise run web:install` | Install dependencies (`npm ci`) |
| `mise run web:dev` | Start the Vite dev server; proxies `/api` to `SENTO_API_URL` (default `http://localhost:8484`) |
| `mise run web:build` | Type-check (`tsc -b`) and build the production bundle |
| `mise run lint:web` | ESLint only; it does not type-check, run `web:build` for that |
| `mise run gen` | Regenerate `src/types/*.ts` from the Go structs after an API change |
| `mise run e2e` | Run the Playwright suite (see [docs/testing](../docs/testing/README.md)) |

`mise run dev` (or the Paseo `web` service) starts this app together with the
Go backend and the extension sidecar.

## Layout

- `src/main.tsx`, `src/App.tsx`, `src/routes.tsx`, `src/shell/`: entry, shell
  and routes.
- `src/api/`: the typed API client (`request.ts` plus one module per resource,
  re-exported by `index.ts`). `src/types/`: types generated from the Go
  structs by `mise run gen` (`tygo.yaml`).
- `src/queries.ts`: query keys and invalidation groups. `src/format.ts`,
  `src/ui.ts`: formatting and shared UI helpers.
- `src/pages/`: one component per route. `src/components/`: feature folders
  plus shared dialog pieces.
