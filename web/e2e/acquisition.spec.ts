import { expect, test } from '@playwright/test'

for (const width of [390, 1280]) {
  test(`download plan edits coverage and reports partial outcomes at ${width}px`, async ({ page, request }, testInfo) => {
    const added = await request.post('/api/v1/series/', { data: { mangabakaId: 103, monitor: 'all' } })
    expect(added.ok()).toBeTruthy()
    const { id } = await added.json()
    try {
      await page.setViewportSize({ width, height: 844 })
      let submitted: { picks: { key: string; books: { bookId: number }[] }[] } | undefined
      await page.route(`**/api/v1/series/${id}/search`, async (route) => {
        const body = route.request().postDataJSON()
        if (body.picks) {
          submitted = body
          return route.fulfill({ json: { candidates: [], searched: [], ledger: 'volume', queued: [900], outcomes: [
            { key: 'pack-a', title: 'Pack A', status: 'queued', message: 'Queued for download', queueId: 900 },
            { key: 'pack-b', title: 'Pack B', status: 'failed', message: 'Release no longer meets the profile' },
          ] } })
        }
        const ref = (n: number) => ({ bookId: n, versionId: body.scope.versionId, editionId: 1, ledger: 'volume', number: n * 100 })
        const release = (key: string, title: string, ns: number[]) => ({ key, contentKey: key, guid: key, title, provider: 'torrent', protocol: 'torrent', site: 'Example indexer', siteId: '1', versionId: body.scope.versionId, score: 2000, representedBooks: 2, size: 104857600, seeders: 10, coverage: ns.map(ref), parse: { quality: {}, edition: {}, volumes: [[ns[0], ns[1]]], language: 'en' }, decision: { status: 'ready', canDownload: true, reasons: [], warnings: [] } })
        const candidates = [release('pack-a', `Pack A ${'long-title-'.repeat(20)}`, [1, 2]), release('pack-b', 'Pack B', [2, 3])]
        const plan = { searchedAt: new Date().toISOString(), complete: false, picks: [
          { key: 'pack-a', books: [ref(1), ref(2)], additionalBooks: 0 },
          { key: 'pack-b', books: [ref(3)], additionalBooks: 1 },
        ], books: [1, 2, 3, 4].map((n) => ({ book: ref(n), label: String(n), ownership: 'missing', availability: n === 4 ? 'unknown' : 'available' })), size: 209715200, unknownSizes: 0 }
        return route.fulfill({ contentType: 'application/x-ndjson', body: [
          { type: 'sites', searched: ['torrent/Example indexer'] },
          { type: 'batch', candidates, queries: ['deterministic diaries', 'journal deterministe'], sites: ['torrent/Example indexer'] },
          { type: 'done', plan },
        ].map((event) => JSON.stringify(event)).join('\n') + '\n' })
      })
      await page.goto(`/series/${id}`)
      const edition = page.locator('.mantine-Card-root').filter({ hasText: 'Standard Edition' }).first()
      if (width < 768) {
        await edition.getByRole('button', { name: /More actions/ }).click()
        await page.getByRole('menuitem', { name: 'Search releases', exact: true }).click()
      } else await edition.getByRole('button', { name: 'Search releases', exact: true }).click()
      const plan = page.getByTestId('acquisition-plan')
      await expect(plan.getByText('3 of 4 wanted volumes selected')).toBeVisible()
      await expect(plan.getByText('Will download · 3')).toBeVisible()
      await expect(plan.getByText('Not checked · 1')).toBeVisible()
      await expect(plan.getByText(/Pack also holds 1 book/)).toBeVisible()
      // An unticked release stays where it was, so the choice can be undone.
      const packB = plan.getByRole('checkbox', { name: 'Include Pack B', exact: true })
      await packB.uncheck()
      await expect(plan.getByRole('button', { name: 'Download 2 volumes' })).toBeVisible()
      await expect(plan.getByText('Available, not selected · 1')).toBeVisible()
      await plan.getByRole('button', { name: 'Browse all 2 releases' }).click()
      await page.getByTestId('release-row').filter({ hasText: 'Pack B' }).getByRole('button', { name: 'Add to plan' }).click()
      await expect(packB).toBeChecked()
      await expect(plan.getByRole('button', { name: 'Download 3 volumes' })).toBeVisible()
      expect(await plan.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
      await page.screenshot({ path: testInfo.outputPath('plan.png'), fullPage: true, animations: 'disabled' })
      await plan.getByRole('button', { name: 'Download 3 volumes' }).click()
      await expect(plan.getByText(/Queued for download/)).toBeVisible()
      await expect(plan.getByText(/Release no longer meets the profile/)).toBeVisible()
      expect(submitted?.picks.map((p) => p.books.map((b) => b.bookId))).toEqual([[1, 2], [3]])
    } finally { await request.delete(`/api/v1/series/${id}/`) }
  })
}

test('stalled download recovery is explicit and excludes the old release', async ({ page }) => {
  let cancelled = false
  let scope: { excludeRelease: string } | undefined
  await page.route('**/api/v1/queue?all=1', (route) => route.fulfill({ json: cancelled ? [] : [{
    id: 910, seriesId: 901, providerId: 'torrent', protocol: 'torrent', releaseKey: 'old-hash',
    title: 'Stalled release', status: 'downloading', stalled: true, progress: 0.2,
    books: [{ versionId: 10, bookId: 20, editionId: 30, ledger: 'volume', number: 100 }],
    createdAt: '2026-10-07T10:00:00Z',
  }] }))
  await page.route('**/api/v1/queue/910/cancel', (route) => { cancelled = true; return route.fulfill({ status: 204 }) })
  await page.route('**/api/v1/series/901/search', (route) => {
    scope = route.request().postDataJSON().scope
    return route.fulfill({ contentType: 'application/x-ndjson', body: '{"type":"sites","searched":[]}\n{"type":"done"}\n' })
  })
  await page.goto('/queue')
  await page.getByRole('button', { name: 'Find another release' }).click()
  expect(cancelled).toBe(false)
  await expect(page.getByText(/torrent and its files stay/)).toBeVisible()
  await page.getByRole('button', { name: 'Stop tracking and search' }).click()
  await expect.poll(() => scope?.excludeRelease).toBe('torrent/old-hash')
  expect(cancelled).toBe(true)
})
