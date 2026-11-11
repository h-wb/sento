import { expect, test } from '@playwright/test'

// The Wanted page is the library-wide view of what the monitored editions
// are still missing, and its search buttons drive the same automatic
// grab as the series page. The library-scan task behind it must be
// registered and runnable.
test('wanted page lists missing books, searches them, and rescans on demand', async ({ page, request }) => {
  // Other specs may have added the same stub series already; either way
  // it is in the library now.
  await request.post('/api/v1/series/', { data: { mangabakaId: 103 } })
  const series = await (await request.get('/api/v1/series/')).json()
  const id = series.find((s: { mangabakaId: number }) => s.mangabakaId === 103).id

  // Check the queued-release notification first.
  await page.route(`**/api/v1/series/${id}/search`, (route) =>
    route.fulfill({
      json: { candidates: [], searched: [], sites: [], ledger: 'volume', queued: [900], outcomes: [] },
    }),
  )

  await page.goto('/wanted')

  // One section per series: its link, then one line per card naming the
  // missing books as ranges with a count.
  const section = page.locator('.mantine-Card-root').filter({ hasText: 'Deterministic Diaries' }).first()
  await expect(section).toBeVisible()
  await expect(section.getByText(/^Vol\. \d/).first()).toBeVisible()
  await expect(section.getByText(/^\d+ missing$/).first()).toBeVisible()

  // The filter shows the counts.
  await expect(page.getByText(/^Missing ·/)).toBeVisible()

  // The automatic search queues its release and says so.
  await section.getByRole('button', { name: /Search volumes for Deterministic Diaries/ }).click()
  await expect(page.getByText(/Queued 1 release for Deterministic Diaries/)).toBeVisible()

  // With the real backend, no configured volume indexer means nothing can
  // be queued. This used to return null and crash the success handler.
  await page.unroute(`**/api/v1/series/${id}/search`)
  const response = page.waitForResponse((res) => res.url().endsWith(`/api/v1/series/${id}/search`))
  await section.getByRole('button', { name: /Search volumes for Deterministic Diaries/ }).click()
  expect(await (await response).json()).toMatchObject({ queued: [] })
  await expect(page.getByText('No usable release found for Deterministic Diaries', { exact: true })).toBeVisible()
  await expect(page.getByText('Something went wrong', { exact: true })).toHaveCount(0)

  // The library-scan task is registered, and a manual run succeeds —
  // that is the loop that keeps the wanted list honest.
  const tasks = await (await request.get('/api/v1/system/tasks')).json()
  expect(tasks.map((t: { slug: string }) => t.slug)).toContain('library-scan')
  const run = await request.post('/api/v1/system/tasks/library-scan/run')
  expect(run.ok()).toBeTruthy()
})
