import { expect, test } from '@playwright/test'

const longError = `Download failed: https://source.example/${'unbroken-path'.repeat(150)}\nUpstream returned HTTP 403.\n${'The source did not allow this request. '.repeat(40)}`
const queueItems = [
  {
    id: 901,
    seriesId: 901,
    protocol: 'source',
    title: `Failed release ${'long-title-'.repeat(30)}`,
    status: 'failed',
    error: longError,
    createdAt: '2026-10-05T12:00:00Z',
  },
  {
    id: 902,
    seriesId: 901,
    protocol: 'torrent',
    title: 'Downloading release',
    status: 'downloading',
    progress: 0.42,
    createdAt: '2026-10-05T12:00:00Z',
  },
  {
    id: 903,
    seriesId: 901,
    protocol: 'torrent',
    title: 'Finished release',
    status: 'imported',
    createdAt: '2026-10-05T12:00:00Z',
  },
]
for (const width of [390, 1280]) {
  test(`activity handles long errors at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 })
    await page.route('**/api/v1/queue?all=1', (route) => route.fulfill({ json: queueItems }))
    await page.goto('/queue')
    await expect(page.getByText('Downloading release', { exact: true })).toBeVisible()
    await expect(page.getByText('42%', { exact: true })).toBeVisible()
    await expect(page.getByText('Finished release', { exact: true })).toHaveCount(0)
    const overflow = await page.evaluate(() =>
      [...document.querySelectorAll('*')]
        .filter((e) => e.getBoundingClientRect().right > window.innerWidth + 1)
        .map((e) => [e.tagName, e.className, e.getBoundingClientRect().right]),
    )
    expect(overflow).toEqual([])
    await page.screenshot({
      path: testInfo.outputPath('activity.png'),
      fullPage: true,
      animations: 'disabled',
    })
    await page.getByRole('button', { name: 'View error', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Download error' })
    await expect(dialog.locator('pre')).toHaveText(longError)
    await expect(dialog).toHaveCSS('opacity', '1')
    expect(await dialog.locator('pre').evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
      true,
    )
    await page.screenshot({
      path: testInfo.outputPath('error-details.png'),
      fullPage: true,
      animations: 'disabled',
    })
    await dialog.getByRole('button', { name: 'Close dialog', exact: true }).click()
    await page.getByRole('tab', { name: /Completed/ }).click()
    await expect(page.getByText('Finished release', { exact: true })).toBeVisible()
    await expect(page.getByText('Downloading release', { exact: true })).toHaveCount(0)
  })
}

test('activity shows a failed request and lets the user retry', async ({ page }) => {
  let fail = true
  await page.route('**/api/v1/queue?all=1', (route) =>
    fail
      ? route.fulfill({ status: 503, json: { error: 'Queue unavailable' } })
      : route.fulfill({ json: [] }),
  )
  await page.goto('/queue')
  await expect(page.getByText('Queue unavailable', { exact: true })).toBeVisible()
  await expect(page.getByText('No downloads in progress', { exact: true })).toHaveCount(0)
  fail = false
  await page.getByRole('button', { name: 'Try again' }).click()
  await expect(page.getByText('No downloads in progress', { exact: true })).toBeVisible()
})

test('settings navigation protects edits and supports staying or discarding', async ({ page }) => {
  await page.goto('/connections')
  const input = page
    .getByRole('region', { name: 'qBittorrent' })
    .getByRole('textbox', { name: 'URL' })
  await expect(input).toHaveValue(/qbit$/)
  await input.fill('http://changed.example:8080')
  await page.getByRole('link', { name: 'Library', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Discard unsaved changes?' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: 'Keep editing' }).click()
  await expect(input).toHaveValue('http://changed.example:8080')
  await page.getByRole('link', { name: 'Library', exact: true }).click()
  await dialog.getByRole('button', { name: 'Discard changes' }).click()
  await expect(page.getByRole('heading', { name: /^Library/, level: 2 })).toBeVisible()
})

test('importer lets users correct a match and retry a failed import', async ({ page }) => {
  let attempts = 0
  const bodies: unknown[] = []
  await page.route('**/api/v1/library/import/scan', (route) =>
    route.fulfill({
      json: [
        {
          dir: '/library/Blade',
          name: 'Blade',
          bestId: 11,
          confidence: 0.8,
          candidates: [
            { mangabakaId: 11, title: 'Blade', year: 2010, type: 'manga', status: 'completed' },
            {
              mangabakaId: 12,
              title: 'Blade Afterstory',
              year: 2015,
              type: 'manga',
              status: 'completed',
            },
          ],
        },
      ],
    }),
  )
  await page.route('**/api/v1/library/import', (route) => {
    bodies.push(route.request().postDataJSON())
    attempts++
    return attempts === 1
      ? route.fulfill({ status: 500, json: { error: 'Temporary import failure' } })
      : route.fulfill({ json: { seriesId: 912, scan: { linked: 3 } } })
  })
  await page.goto('/add?tab=import')
  await page.getByRole('button', { name: 'Scan', exact: true }).click()
  await page.getByRole('combobox', { name: 'Match to series' }).click()
  await page.getByRole('option', { name: 'Blade Afterstory (2015)' }).click()
  await page.getByRole('button', { name: 'Import', exact: true }).click()
  await expect(page.getByText(/Temporary import failure/)).toBeVisible()
  await page.getByRole('button', { name: 'Retry import', exact: true }).click()
  await expect(page.getByRole('link', { name: 'Open', exact: true })).toHaveAttribute(
    'href',
    '/series/912',
  )
  expect(bodies).toEqual([
    { dir: '/library/Blade', mangabakaId: 12 },
    { dir: '/library/Blade', mangabakaId: 12 },
  ])
})

test('filtered-out library selections cannot be bulk deleted', async ({ page }) => {
  await page.route('**/api/v1/series/', (route) =>
    route.fulfill({
      json: [
        {
          id: 901,
          title: 'Alpha',
          sortTitle: 'Alpha',
          type: 'manga',
          status: 'ongoing',
          monitored: true,
          addedAt: '',
        },
        {
          id: 902,
          title: 'Beta',
          sortTitle: 'Beta',
          type: 'manga',
          status: 'ongoing',
          monitored: true,
          addedAt: '',
        },
      ],
    }),
  )
  await page.goto('/')
  await page.getByRole('button', { name: 'Table', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Select Alpha', exact: true }).check()
  await expect(page.getByText('1 selected', { exact: true })).toBeVisible()
  await page.getByRole('textbox', { name: 'Filter by title' }).fill('Beta')
  await expect(page.getByText('1 selected', { exact: true })).toBeHidden()
  await page.getByRole('checkbox', { name: 'Select Beta', exact: true }).check()
  await page.getByRole('button', { name: 'Delete', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Remove 1 series?' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText(/files stay on disk/)).toBeVisible()
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
})

test('series interactions work with keyboard and fit a mobile viewport', async ({
  page,
}, testInfo) => {
  const series = {
    id: 901,
    mangabakaId: 901,
    title: 'Witch Hat Atelier',
    sortTitle: 'Witch Hat Atelier',
    status: 'ongoing',
    type: 'manga',
    year: 2016,
    monitored: true,
    finalVolume: 15,
    totalChapters: 85,
    description: 'A young apprentice discovers a world of magic. '.repeat(25),
  }
  const collection = {
    id: 901,
    seriesId: 901,
    editionName: 'Standard Edition',
    language: 'en',
    type: 'volume',
    monitored: true,
  }
  await page.route('**/api/v1/series/901/', (route) =>
    route.fulfill({ json: { series, collections: [collection] } }),
  )
  await page.route('**/api/v1/series/901/slots', (route) =>
    route.fulfill({
      json: {
        volumes: {
          '901': [
            {
              number: 100,
              countType: 'main',
              released: true,
              wanted: true,
              monitored: true,
              fileIds: [],
            },
          ],
        },
        chapters: {},
      },
    }),
  )
  await page.route('**/api/v1/series/901/files', (route) => route.fulfill({ json: [] }))
  await page.goto('/series/901')
  await expect(page.getByRole('button', { name: 'Search & download', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Read more' }).click()
  await expect(page.getByRole('button', { name: 'Show less' })).toHaveAttribute(
    'aria-expanded',
    'true',
  )
  await expect(page.getByRole('tab', { name: 'Editions', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await page.getByRole('button', { name: 'Show less' }).click()
  await page.screenshot({
    path: testInfo.outputPath('series.png'),
    fullPage: true,
    animations: 'disabled',
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
    .toBe(true)
  await page.screenshot({
    path: testInfo.outputPath('series-mobile.png'),
    fullPage: true,
    animations: 'disabled',
  })
})

test('media settings preview the folder layout, including a split by type', async ({ page }, testInfo) => {
  await page.goto('/media')
  // Default: one folder per series, editions inside it.
  await expect(
    page.getByText('ONE PIECE [mangabaka-377]/Standard Edition [en]/ONE PIECE v112 [Standard Edition][en].cbz', { exact: true }),
  ).toBeVisible()
  await page.getByRole('textbox', { name: 'Volumes top folder' }).fill('Volumes')
  await page.getByRole('textbox', { name: 'Chapters top folder' }).fill('Chapters')
  await expect(page.getByText(/^Volumes\/ONE PIECE \[mangabaka-377\]\/Standard Edition \[en\]\/ONE PIECE v112/)).toBeVisible()
  await expect(page.getByText(/^Chapters\/ONE PIECE \[mangabaka-377\]\/Standard Edition \[en\]\/ONE PIECE c1102\.5/)).toBeVisible()
  // The type can go inside the series instead.
  await page.getByRole('textbox', { name: 'Volumes top folder' }).fill('')
  await page.getByRole('textbox', { name: 'Chapters top folder' }).fill('')
  await page.getByLabel('Folders inside a series').first().click()
  await page.getByRole('option', { name: /By type, then edition/ }).click()
  await expect(page.getByText(/^ONE PIECE \[mangabaka-377\]\/Chapters\/Standard Edition \[en\]\/ONE PIECE c1102\.5/)).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('media-layout.png'), fullPage: true })
})
