import { defineConfig, devices } from '@playwright/test'
import { mkdirSync, rmSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webDir = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(webDir, '..')

// Sento is stateful (SQLite + library/downloads dirs). Wiping the run
// directory up front is what makes the suite repeatable: specs may add
// series, and a leftover database would make the second run diverge.
const runDir = path.join(webDir, '.playwright')
rmSync(runDir, { recursive: true, force: true })
mkdirSync(runDir, { recursive: true })

// Off the dev ports (8484 sento / 8901 host) so a running dev stack and a
// test run never fight over a socket.
const APP_PORT = 8585
const STUB_PORT = 8899
const stub = `http://127.0.0.1:${STUB_PORT}`

export const appURL = `http://127.0.0.1:${APP_PORT}`

// Prebuilt by `mise run e2e` — running the binaries rather than `go run`
// keeps process teardown clean (no orphaned child on kill).
const bin = (name: string) => path.join(repoRoot, 'bin', name)

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false, // one backend, shared mutable state
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list']],
  timeout: 30_000,
  expect: { timeout: 10_000 },

  use: {
    baseURL: appURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },

  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],

  webServer: [
    {
      command: `${bin('e2estub')} -addr 127.0.0.1:${STUB_PORT}`,
      cwd: repoRoot,
      url: `${stub}/health`,
      reuseExistingServer: !process.env.CI,
      stdout: 'pipe',
      stderr: 'pipe',
      timeout: 60_000,
    },
    {
      // A path that deliberately does not exist: config.Load tolerates a
      // missing file, so the run is configured purely by env and never
      // picks up a developer's real config.yml at the repo root.
      command: `${bin('sento')} -config ${path.join(runDir, 'absent.yml')}`,
      cwd: repoRoot,
      url: `${appURL}/api/v1/system/health`,
      reuseExistingServer: false,
      stdout: 'pipe',
      stderr: 'pipe',
      timeout: 60_000,
      env: {
        SENTO_HOST: '127.0.0.1',
        SENTO_PORT: String(APP_PORT),
        SENTO_DB_PATH: path.join(runDir, 'sento.db'),
        SENTO_LIBRARY_PATH: path.join(runDir, 'library'),
        SENTO_DOWNLOADS_DIR: path.join(runDir, 'downloads'),
        SENTO_STATIC_DIR: path.join(webDir, 'dist'),
        SENTO_MANGABAKA_BASE_URL: `${stub}/mangabaka/v1`,
        SENTO_EXTENSIONS_URL: `${stub}/host`,
        SENTO_PROWLARR_URL: `${stub}/prowlarr`,
        SENTO_PROWLARR_API_KEY: 'stub-key',
        SENTO_QBITTORRENT_URL: `${stub}/qbit`,
        SENTO_QBITTORRENT_USERNAME: 'stub',
        SENTO_QBITTORRENT_PASSWORD: 'stub',
      },
    },
  ],
})
