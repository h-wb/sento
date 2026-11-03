import { expect, test } from '@playwright/test'
import { fixtureURL } from '../playwright.config'
import type { ReleaseCandidate } from '../src/types/service'
import type { BlocklistEntry, QueueItem } from '../src/types/store'

test('blocked releases can be reconsidered in search, selected, or cleared together', async ({ page, request }, testInfo) => {
  const indexer = await request.post('/api/v1/indexers', { data: {
    name: 'Blocklist recovery fixture', protocol: 'torznab', enabled: true, useVolumes: true, useChapters: false,
    priority: 0, settings: { url: `${fixtureURL}/torznab/api`, apiKey: 'fixture-key' },
  } })
  expect(indexer.ok()).toBeTruthy()
  const { id: indexerID } = await indexer.json()
  const added = await request.post('/api/v1/series/', { data: { mangabakaId: 103, monitor: 'all' } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  const blocks = async (): Promise<BlocklistEntry[]> => (await request.get('/api/v1/blocklist')).json()
  try {
    const search = await request.post(`/api/v1/series/${id}/search`, { data: { ledger: 'volume', dryRun: true } })
    expect(search.ok()).toBeTruthy()
    const { candidates }: { candidates: ReleaseCandidate[] } = await search.json()
    const ready = candidates.filter((c) => c.siteId === String(indexerID) && c.decision.canDownload)
    expect(ready).toHaveLength(3)
    const jobs: number[] = []
    for (const c of ready) {
      const grab = await request.post(`/api/v1/series/${id}/search`, { data: {
        ledger: 'volume', grab: c.guid, grabProvider: c.provider, grabSiteId: c.siteId, grabVersionId: c.versionId,
      } })
      expect(grab.ok(), await grab.text()).toBeTruthy()
      const { queued } = await grab.json()
      jobs.push(queued[0])
      expect((await request.post(`/api/v1/queue/${queued[0]}/blocklist`)).ok()).toBeTruthy()
    }
    expect(await blocks()).toHaveLength(3)
    const attempts = async (): Promise<QueueItem[]> => ((await (await request.get('/api/v1/queue?all=1')).json()) as QueueItem[]).filter((q) => q.seriesId === id)
    const before = await attempts()

    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/series/${id}`)
    const edition = page.locator('.mantine-Card-root').filter({ hasText: 'Standard Edition' }).first()
    await edition.getByRole('button', { name: /More actions/ }).click()
    await page.getByRole('menuitem', { name: 'Search releases', exact: true }).click()
    await page.getByText('Ruled out 3', { exact: true }).click()
    const rejected = page.getByTestId('release-row').filter({ hasText: ready[0].title })
    await expect(rejected).toHaveAttribute('data-status', 'blocked')
    await rejected.getByRole('button', { name: 'Unblock', exact: true }).click()
    // Refreshing the search produces an eligible plan; unblocking itself
    // does not submit a download or revive the failed queue record.
    await expect(page.getByRole('checkbox', { name: `Include ${ready[0].title}`, exact: true })).toBeVisible()
    await expect.poll(async () => (await blocks()).length).toBe(2)
    await page.keyboard.press('Escape')

    await page.goto('/queue')
    await page.getByRole('tab', { name: 'History', exact: true }).click()
    await page.getByText('Blocked releases', { exact: true }).click()
    await expect(page.getByText('2 blocked releases', { exact: true })).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath('blocked-release-rows-mobile.png'), fullPage: true, animations: 'disabled' })
    const remaining = await blocks()
    const filter = page.getByRole('textbox', { name: 'Filter blocked releases' })
    await filter.fill(remaining[0].title!)
    await page.getByRole('checkbox', { name: `Select ${remaining[0].title}`, exact: true }).check()
    await page.getByRole('button', { name: 'Unblock selected (1)', exact: true }).click()
    await expect.poll(async () => (await blocks()).length).toBe(1)
    // Clear all includes releases hidden by a filter, and requires a
    // confirmation that can be cancelled without changing any blocks.
    await filter.fill('no matching release')
    await page.getByRole('button', { name: 'Unblock all', exact: true }).click()
    const confirmation = page.getByRole('dialog', { name: 'Unblock all 1 release?' })
    await expect(confirmation.getByText(/including manual blocks/)).toBeVisible()
    await confirmation.getByRole('button', { name: 'Cancel', exact: true }).click()
    expect(await blocks()).toHaveLength(1)
    await expect(page.getByText('1 blocked release', { exact: true })).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath('blocked-releases-mobile.png'), fullPage: true, animations: 'disabled' })
    await page.getByRole('button', { name: 'Unblock all', exact: true }).click()
    await confirmation.getByRole('button', { name: 'Unblock all', exact: true }).click()
    await expect(page.getByText('No blocked releases.', { exact: true })).toBeVisible()
    expect(await blocks()).toHaveLength(0)
    const after = await attempts()
    expect(after.map((q) => q.id).sort()).toEqual(jobs.sort())
    expect(after.map((q) => [q.id, q.status, q.books, q.error])).toEqual(before.map((q) => [q.id, q.status, q.books, q.error]))
  } finally {
    await request.delete('/api/v1/blocklist')
    await request.delete(`/api/v1/indexers/${indexerID}`)
    await request.delete(`/api/v1/series/${id}/`)
  }
})
