import { expect, test } from '@playwright/test'

// Backend wiring checked without a browser: these assert that the Go
// stack answers the shapes web/src/types (generated from the Go structs)
// expects, so a handler regression fails here rather than as a confusing
// blank page in the UI specs.
test.describe('API', () => {
  test('health and status report a live database', async ({ request }) => {
    const health = await request.get('/api/v1/system/health')
    expect(health.ok()).toBeTruthy()
    expect(await health.json()).toMatchObject({ status: 'ok' })

    const status = await request.get('/api/v1/system/status')
    expect(status.ok()).toBeTruthy()
    const body = await status.json()
    expect(body.app).toBe('sento')
    expect(body.database.ok).toBe(true)
  })

  test('lookup proxies the metadata provider', async ({ request }) => {
    const res = await request.get('/api/v1/lookup?q=stub')
    expect(res.ok()).toBeTruthy()
    const results = await res.json()
    expect(Array.isArray(results)).toBeTruthy()
    expect(results.length).toBeGreaterThan(0)
    expect(results[0]).toMatchObject({ title: expect.any(String), mangabakaId: expect.any(Number) })
  })

  test('sources come from the configured extensions sidecar', async ({ request }) => {
    const res = await request.get('/api/v1/sources')
    expect(res.ok()).toBeTruthy()
    const sources = await res.json()
    expect(sources).toEqual(
      expect.arrayContaining([expect.objectContaining({ id: 'stub-en-1', name: 'Stub Source' })]),
    )
  })

  test('indexers come from Prowlarr', async ({ request }) => {
    const res = await request.get('/api/v1/indexers')
    expect(res.ok()).toBeTruthy()
    expect(Array.isArray(await res.json())).toBeTruthy()
  })

  test('edition endpoints answer with arrays, not null', async ({ request }) => {
    // The UI maps over these directly; a JSON null would crash the page.
    for (const path of [
      '/api/v1/series/',
      '/api/v1/queue',
      '/api/v1/history',
      '/api/v1/blocklist',
      '/api/v1/profiles',
      '/api/v1/formats',
      '/api/v1/system/tasks',
      '/api/v1/calendar',
    ]) {
      const res = await request.get(path)
      expect(res.ok(), `${path} returned ${res.status()}`).toBeTruthy()
      expect(Array.isArray(await res.json()), `${path} did not return an array`).toBeTruthy()
    }
  })

  test('unknown API routes 404 as JSON', async ({ request }) => {
    const res = await request.get('/api/v1/nope')
    expect(res.status()).toBe(404)
    expect(await res.json()).toHaveProperty('error')
  })
})
