import { expect, test } from '@playwright/test'
import { activityFixture } from './activity-fixtures'

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
    await page.route('**/api/v1/activity?seriesId=*', (route) => route.fulfill({ json: activityFixture(queueItems) }))
    await page.goto('/queue?activity=progress')
    await page.getByRole('button', { name: 'Show downloads for One Piece' }).click()
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
    await page.getByRole('tab', { name: /Needs attention/ }).click()
    await page.getByText('Details', { exact: true }).click()
    const error = page.locator('[data-testid="download-row"] pre')
    await expect(error).toHaveText(longError)
    expect(await error.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath('error-details.png'), fullPage: true, animations: 'disabled' })
    await page.getByRole('tab', { name: 'History', exact: true }).click()
    await expect(page.getByText('Finished release', { exact: true })).toBeVisible()
    await expect(page.getByText('Downloading release', { exact: true })).toHaveCount(0)
  })
}

test('activity shows a failed request and lets the user retry', async ({ page }) => {
  let fail = true
  await page.route('**/api/v1/activity?seriesId=*', (route) =>
    fail
      ? route.fulfill({ status: 503, json: { error: 'Queue unavailable' } })
      : route.fulfill({ json: activityFixture() }),
  )
  await page.goto('/queue')
  await expect(page.getByText('Queue unavailable', { exact: true })).toBeVisible()
  await expect(page.getByText('No downloads in progress', { exact: true })).toHaveCount(0)
  fail = false
  await page.getByRole('button', { name: 'Try again' }).click()
  await expect(page.getByText('No downloads in progress', { exact: true })).toBeVisible()
})

test('settings navigation protects edits and supports staying or discarding', async ({ page }) => {
  // Media keeps its form on the page (Connections edits in dialogs).
  await page.goto('/media')
  const input = page.getByRole('textbox', { name: 'Volumes top folder' })
  await expect(input).toHaveValue('')
  await input.fill('Volumes')
  await page.getByRole('link', { name: 'Library', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Discard unsaved changes?' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: 'Keep editing' }).click()
  await expect(input).toHaveValue('Volumes')
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
  const dialog = page.getByRole('dialog', { name: 'Remove this series?' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('checkbox', { name: 'Delete its files from disk' })).toBeChecked()
  await expect(dialog.getByRole('checkbox', { name: 'Remove its torrents from qBittorrent' })).not.toBeChecked()
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
  const edition = {
    id: 901,
    seriesId: 901,
    editionName: 'Standard Edition',
    language: 'en',
    type: 'volume',
    monitored: true,
  }
  await page.route('**/api/v1/series/901/', (route) =>
    route.fulfill({ json: { series, editions: [edition] } }),
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
  const tree = page.getByTestId('path-tree')
  const level = (name: string, setting: string) => tree.getByRole('button', { name: new RegExp(`^${name.replace(/[[\]().]/g, '\\$&')}: ${setting}`) })
  // Default: one folder per series, editions inside it.
  await expect(level('ONE PIECE [mangabaka-377]', 'Series folder')).toHaveCount(1)
  await expect(level('Standard Edition [en] (Colored)', 'Book folder')).toBeVisible()
  await expect(level('ONE PIECE v112 [Standard Edition][en].cbz', 'Volume file')).toHaveCount(2)
  await expect(page.getByText(/^Kavita shows ONE PIECE and ONE PIECE Colored, but from one series folder/)).toBeVisible()
  await page.getByRole('textbox', { name: 'Volumes top folder' }).fill('Volumes')
  await page.getByRole('textbox', { name: 'Chapters top folder' }).fill('Chapters')
  await expect(level('Volumes', 'Volumes top folder')).toBeVisible()
  await expect(level('Chapters', 'Chapters top folder')).toBeVisible()
  // Picking a level goes to the setting that makes it.
  await level('Chapters', 'Chapters top folder').click()
  await expect(page.getByRole('textbox', { name: 'Chapters top folder' })).toBeFocused()
  // The type can go inside the series instead.
  await page.getByRole('textbox', { name: 'Volumes top folder' }).fill('')
  await page.getByRole('textbox', { name: 'Chapters top folder' }).fill('')
  await page.getByLabel('Folders inside a series').first().click()
  await page.getByRole('option', { name: /By type, then edition/ }).click()
  await expect(level('Chapters', 'Book folder')).toBeVisible()
  // The reader setup gives every name in readers its own folder.
  await page.getByRole('button', { name: /^Kavita or Komga/ }).click()
  await expect(level('ONE PIECE Colored [mangabaka-377]', 'Series folder')).toBeVisible()
  await expect(page.getByText(/^Kavita shows ONE PIECE and ONE PIECE Colored as two series/)).toBeVisible()
  await expect(page.getByText(/Turn off Append volume to series title/)).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('media-layout.png'), fullPage: true })
})

for (const width of [390, 1280]) {
  test(`activity groups problems and shares recovery with a series at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 })
    const jobs = Array.from({ length: 8 }, (_, i) => ({
      id: 1000 + i, seriesId: 901, seriesTitle: 'One Piece', protocol: 'source',
      title: `Broken volume ${i + 1}`, status: i < 6 ? 'needs_review' : 'retry_wait',
      canTryAnother: true, error: i < 6 ? 'HTTP error 404' : 'HTTP error 503', retryAt: new Date(Date.now() + 120000).toISOString(),
    }))
    const pack = { id: 2000, seriesId: 902, seriesTitle: 'Death Note', protocol: 'torrent', title: 'Full Color pack', status: 'needs_review', canTryAnother: false }
    const files = Array.from({ length: 8 }, (_, i) => ({ id: `file-${i}`, kind: 'file', seriesId: 902, seriesTitle: 'Death Note', itemId: 3000 + i, title: `Volume ${i + 1}.cbz`, detail: 'Confirm the edition and books', status: 'present' }))
    const data = activityFixture([...jobs, pack], { 2000: { files, issue: 'files' } })
    let recovered: number[] = []
    let retried = 0
    await page.route('**/api/v1/activity?seriesId=*', (route) => {
      const scope = Number(new URL(route.request().url()).searchParams.get('seriesId'))
      return route.fulfill({ json: { ...data, downloads: data.downloads.filter((d) => !scope || d.job.seriesId === scope) } })
    })
    await page.route('**/api/v1/queue/another', (route) => { recovered = route.request().postDataJSON().ids; return route.fulfill({ status: 204 }) })
    await page.route('**/api/v1/queue/1006/retry', (route) => { retried++; return route.fulfill({ status: 204 }) })
    await page.goto('/queue')
    // Six identical failures and one pack are two questions, not 15 questions.
    await expect(page.getByRole('tab', { name: 'Needs attention 2' })).toBeVisible()
    await expect(page.getByTestId('activity-problem')).toHaveCount(2)
    await expect(page.getByText('6 affected downloads')).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath('attention-overview.png'), fullPage: true, animations: 'disabled' })
    await expect(page.getByText('Broken volume 1', { exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Try other releases', exact: true }).click()
    await expect.poll(() => recovered).toEqual([1000, 1001, 1002, 1003, 1004, 1005])
    await page.getByRole('button', { name: 'Show affected downloads' }).click()
    await page.getByRole('button', { name: 'Try another release', exact: true }).first().click()
    await expect.poll(() => recovered).toEqual([1000])
    await expect(page.getByRole('button', { name: 'Review 8 files' })).toBeVisible()
    await expect(page.getByText('Volume 1.cbz', { exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Review 8 files' }).click()
    await expect(page.getByRole('button', { name: 'Review file', exact: true })).toHaveCount(8)
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath('attention.png'), fullPage: true, animations: 'disabled' })
    await page.getByRole('tab', { name: /In progress/ }).click()
    await page.getByRole('button', { name: 'Show downloads for One Piece' }).click()
    await expect(page.getByText(/Retrying in 2 min/)).toHaveCount(2)
    await page.getByRole('button', { name: 'Retry now', exact: true }).first().click()
    await expect.poll(() => retried).toBe(1)
    await page.screenshot({ path: testInfo.outputPath('progress.png'), fullPage: true, animations: 'disabled' })
    // The same row and action operate inside a series without a second inbox.
    await page.route('**/api/v1/series/901/', (route) => route.fulfill({ json: { series: { id: 901, title: 'One Piece', monitored: true }, editions: [] } }))
    await page.goto('/series/901?view=activity&activity=attention')
    await expect(page.getByRole('tab', { name: 'Review', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Show affected downloads' }).click()
    await expect(page.getByRole('button', { name: 'Try another release', exact: true })).toHaveCount(6)
    await page.getByRole('button', { name: 'Try another release', exact: true }).last().click()
    await expect.poll(() => recovered).toEqual([1005])
  })
}
