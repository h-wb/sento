import { expect, test } from '@playwright/test'
import { activityFixture } from './activity-fixtures'

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
      await edition.getByRole('button', { name: /More actions/ }).click()
      await page.getByRole('menuitem', { name: 'Search releases', exact: true }).click()
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
      // The shelf answers "why is this book not in the plan?" in one click.
      await plan.getByRole('button', { name: 'volume 3: Will download' }).click()
      await expect(page.getByText('Releases containing volume 3')).toBeVisible()
      await expect(page.getByTestId('release-row')).toHaveCount(1)
      await page.getByRole('button', { name: 'Show all releases' }).click()
      await plan.getByRole('button', { name: 'Download 3 volumes' }).click()
      await expect(plan.getByText('Queued', { exact: true })).toBeVisible()
      await expect(plan.getByText('Failed', { exact: true })).toBeVisible()
      await expect(plan.getByText(/Release no longer meets the profile/)).toBeVisible()
      await expect(plan.getByRole('link', { name: 'Follow in Activity' })).toBeVisible()
      expect(submitted?.picks.map((p) => p.books.map((b) => b.bookId))).toEqual([[1, 2], [3]])
    } finally { await request.delete(`/api/v1/series/${id}/`) }
  })
}

test('looking for a replacement leaves the stalled download alone', async ({ page }) => {
  let cancelled = false
  let scope: { replaceQueueId: number } | undefined
  await page.route('**/api/v1/activity?seriesId=*', (route) => route.fulfill({ json: activityFixture([{
    id: 910, seriesId: 901, providerId: 'torrent', protocol: 'torrent', releaseKey: 'old-hash',
    title: 'Stalled release', status: 'downloading', stalled: true, progress: 0.2,
    books: [{ versionId: 10, bookId: 20, editionId: 30, ledger: 'volume', number: 100 }],
    createdAt: '2026-10-07T10:00:00Z',
  }]) }))
  await page.route('**/api/v1/queue/910/cancel', (route) => { cancelled = true; return route.fulfill({ status: 204 }) })
  await page.route('**/api/v1/series/901/search', (route) => {
    scope = route.request().postDataJSON().scope
    return route.fulfill({ contentType: 'application/x-ndjson', body: '{"type":"sites","searched":[]}\n{"type":"done"}\n' })
  })
  await page.goto('/queue')
  await page.getByRole('button', { name: 'Choose a replacement' }).click()
  await expect(page.getByText(/keeps downloading while you look/)).toBeVisible()
  await expect(page.getByText(/torrent and files stay/)).toBeVisible()
  await expect.poll(() => scope?.replaceQueueId).toBe(910)
  // Only the server retires the old job, and only when a replacement is picked.
  expect(cancelled).toBe(false)
})

test('a local edition is offered the catalog edition that matches it', async ({ page, request }) => {
  const added = await request.post('/api/v1/series/', { data: { mangabakaId: 103, monitor: 'all' } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  try {
    const detail = await (await request.get(`/api/v1/series/${id}/`)).json()
    const catalog = detail.editions.find((e: { editionName: string; type: string }) => e.editionName === 'Standard Edition' && e.type === 'volume')
    const created = await request.post(`/api/v1/series/${id}/editions`, {
      data: { editionName: 'My scans', language: catalog.language, publisher: '', medium: '', type: 'volume', basedOn: catalog.id, volumes: 0, cutoffNum: 0, monitored: true },
    })
    expect(created.ok()).toBeTruthy()

    await page.goto(`/series/${id}?view=review`)
    const item = page.getByTestId('activity-problem').filter({ hasText: 'Catalog edition available' })
    await expect(item.getByText(/MangaBaka lists “Standard Edition/)).toBeVisible()
    await item.getByRole('button', { name: 'Review and link' }).click()
    const dialog = page.getByRole('dialog', { name: 'Link edition to MangaBaka' })
    await expect(dialog.getByText(/of your books match the catalog/)).toBeVisible()
    await dialog.getByRole('button', { name: 'Link edition' }).click()
    await expect(dialog).toBeHidden()
    await expect(page.getByText('Catalog edition available')).toHaveCount(0)

    // One edition remains, and it is a catalog edition now.
    const after = await (await request.get(`/api/v1/series/${id}/`)).json()
    const standard = after.editions.filter((e: { editionName: string; type: string }) => e.editionName === 'Standard Edition' && e.type === 'volume')
    expect(standard).toHaveLength(1)
    expect(standard[0].custom).toBe(false)
  } finally { await request.delete(`/api/v1/series/${id}/`) }
})
