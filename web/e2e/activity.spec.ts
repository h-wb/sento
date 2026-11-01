import { expect, test } from '@playwright/test'
import { activityFixture } from './activity-fixtures'

test('global activity reviews files in place and confirms an uncertain download without submitting twice', async ({
  page,
}) => {
  const ref = { versionId: 10, bookId: 20, editionId: 30, ledger: 'volume', number: 100 }
  const fileQuestion = {
    id: 'file-301',
    kind: 'file',
    seriesId: 901,
    seriesTitle: 'One Piece',
    itemId: 301,
    title: 'Volume 1.cbz',
    status: 'present',
    detail: 'Confirm the edition and books',
    proposal: {
      books: [ref],
      edition: 'Standard Edition',
      label: 'volume 1',
      evidence: 'from the file name',
    },
  }
  let mapped = false
  let attached = false
  let attempts = 0
  let saved: unknown
  let clientID = ''
  const submissions: string[] = []
  page.on('request', (request) => {
    if (request.method() === 'POST' && /\/series\/\d+\/(search|grab)/.test(request.url()))
      submissions.push(request.url())
  })
  await page.route('**/api/v1/activity?seriesId=*', (route) =>
    route.fulfill({
      json: activityFixture(
        [
          { id: 50, title: 'Imported pack', status: mapped ? 'imported' : 'needs_review' },
          {
            id: 60,
            title: 'Uncertain submission',
            status: attached ? 'queued' : 'dispatch_unknown',
          },
          {
            id: 70,
            title: 'Access denied source',
            protocol: 'source',
            status: 'needs_review',
            canTryAnother: true,
          },
        ],
        {
          50: { files: mapped ? [] : [fileQuestion], attention: !mapped, history: mapped },
          70: { issue: 'access', explanation: 'The source did not allow this download.' },
        },
      ),
    }),
  )
  await page.route('**/api/v1/series/901/', (route) =>
    route.fulfill({
      json: {
        series: { id: 901, title: 'One Piece' },
        editions: [
          {
            id: 30,
            seriesId: 901,
            editionName: 'Standard Edition',
            language: 'en',
            type: 'volume',
          },
        ],
      },
    }),
  )
  await page.route('**/api/v1/series/901/versions', (route) =>
    route.fulfill({
      json: [
        { id: 10, editionId: 30, name: 'Default', isDefault: true, monitored: true, profileId: 0 },
      ],
    }),
  )
  let factsPatch: unknown = null
  await page.route('**/api/v1/series/901/files/301/facts', (route) => {
    factsPatch = route.request().postDataJSON()
    return route.fulfill({ status: 204 })
  })
  await page.route('**/api/v1/series/901/files', (route) =>
    route.fulfill({
      json: [
        {
          id: 301,
          seriesId: 901,
          path: '/library/review/Volume 1.cbz',
          needsReview: true,
          // The file's own name says nothing about language; the release
          // it came from advertised en, so review prefills that.
          facts: { title: 'Volume 1.cbz' },
          claims: { language: 'en', group: '1r0n' },
        },
      ],
    }),
  )
  await page.route('**/api/v1/series/901/coverage', (route) => route.fulfill({ json: [] }))
  await page.route('**/api/v1/series/901/books', (route) =>
    route.fulfill({
      json: {
        '10': [
          {
            versionId: 10,
            book: { id: 20, editionId: 30, sequenceNum: 100, sequenceStr: '1', countType: 'main' },
            fileIds: [],
            onDiskFileIds: [],
          },
        ],
      },
    }),
  )
  await page.route('**/api/v1/series/901/files/301/mapping', (route) => {
    saved = route.request().postDataJSON()
    attempts++
    if (attempts === 1)
      return route.fulfill({ status: 503, json: { error: 'Storage temporarily unavailable' } })
    mapped = true
    return route.fulfill({ status: 204 })
  })
  await page.route('**/api/v1/queue/60/retry', (route) => {
    clientID = route.request().postDataJSON().remoteId
    attached = true
    return route.fulfill({ status: 204 })
  })
  await page.goto('/queue')
  const source = page.getByTestId('download-row').filter({ hasText: 'Access denied source' })
  await expect(source.getByRole('button', { name: 'Check source settings' })).toBeVisible()
  await page.getByRole('button', { name: 'Review 1 file', exact: true }).click()
  await page.getByRole('button', { name: 'Review file', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Match imported file' })
  await expect(dialog.getByRole('combobox', { name: 'Edition', exact: true })).toHaveValue(/Standard Edition/)
  // Language and group are prefilled from what the release advertised.
  await expect(dialog.getByRole('textbox', { name: 'Confirmed language' })).toHaveValue('en')
  await expect(dialog.getByRole('textbox', { name: 'Confirmed release group' })).toHaveValue('1r0n')
  await dialog.getByRole('button', { name: 'Save coverage' }).click()
  await expect(dialog.getByText(/Storage temporarily unavailable/)).toBeVisible()
  await dialog.getByRole('button', { name: 'Save coverage' }).click()
  await expect(dialog).toBeHidden()
  expect(saved).toEqual({ books: [ref] })
  // Saving recorded the prefilled claims as the file's confirmed facts.
  expect(factsPatch).toEqual({ language: 'en', group: '1r0n' })
  await expect(page.getByRole('button', { name: 'Review 1 file', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Confirm download', exact: true }).click()
  const confirmation = page.getByRole('dialog', { name: 'Confirm existing download' })
  await confirmation
    .getByRole('textbox', { name: 'Client download ID' })
    .fill('existing-torrent-hash')
  await confirmation.getByRole('button', { name: 'Attach download' }).click()
  await expect(confirmation).toBeHidden()
  expect(clientID).toBe('existing-torrent-hash')
  expect(submissions).toEqual([])
})

test('a large waiting queue keeps the current download visible', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const jobs = [
    { id: 1, title: 'Current volume', status: 'downloading', progress: 0.42 },
    ...Array.from({ length: 80 }, (_, i) => ({ id: i + 2, title: `Waiting volume ${i + 1}`, status: 'queued' })),
  ]
  await page.route('**/api/v1/activity?seriesId=*', (route) => route.fulfill({ json: activityFixture(jobs) }))
  await page.goto('/queue')
  await page.getByRole('button', { name: 'Show downloads for One Piece' }).click()
  await expect(page.getByText('Current volume', { exact: true })).toBeVisible()
  await expect(page.getByText('42%', { exact: true })).toBeVisible()
  await expect(page.getByText('Waiting volume 1', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Show 80 waiting downloads' }).click()
  await expect(page.getByText('Waiting volume 80', { exact: true })).toBeVisible()
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})
