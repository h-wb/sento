import { expect, test } from '@playwright/test'
import { activityFixture } from './activity-fixtures'
import { reviewBook, reviewFile } from './review-choice-fixtures'
import type { ReviewItem } from '../src/types/store'

// A pack's file questions arrive grouped by release, with the pack-level
// actions: one decline for the whole pack, one edition choice mapped by
// each file's own numbers. Unresolved files are never guessed.
const packFiles: ReviewItem[] = [
  {
    id: 'file-301',
    kind: 'file',
    seriesId: 901,
    seriesTitle: 'One Piece',
    itemId: 301,
    title: 'Volume 1.cbz',
    status: 'present',
    detail: 'Confirm the edition and books',
    releaseKey: 'pack',
    release: 'One Piece v01-03 (Digital)',
    hint: { ledger: 'volume', numbers: ['1'] },
  },
  {
    id: 'file-302',
    kind: 'file',
    seriesId: 901,
    seriesTitle: 'One Piece',
    itemId: 302,
    title: 'Volume 2.cbz',
    status: 'present',
    detail: 'Confirm the edition and books',
    releaseKey: 'pack',
    release: 'One Piece v01-03 (Digital)',
    hint: { ledger: 'volume', numbers: ['2'] },
  },
  {
    id: 'file-303',
    kind: 'file',
    seriesId: 901,
    seriesTitle: 'One Piece',
    itemId: 303,
    title: 'Extras.cbz',
    status: 'present',
    detail: 'Confirm the edition and books',
    releaseKey: 'pack',
    release: 'One Piece v01-03 (Digital)',
  },
]

test('a release pack can be declined at once after seeing what will be marked', async ({ page }) => {
  let resolved: unknown
  await page.route('**/api/v1/activity?seriesId=*', (route) =>
    route.fulfill({ json: activityFixture([], {}, packFiles) }),
  )
  await page.route('**/api/v1/series/901/review/resolve', (route) => {
    resolved = route.request().postDataJSON()
    return route.fulfill({ json: { resolved: [301, 302, 303], skipped: [] } })
  })
  await page.goto('/queue')
  await expect(page.getByText('Release · 3 files waiting')).toBeVisible()
  // The per-file questions stay visible inside the group.
  await expect(page.getByText('Volume 2.cbz')).toBeVisible()
  await page.getByRole('button', { name: 'Decline all (3)' }).click()
  const confirm = page.getByRole('dialog', { name: 'Decline all 3 files?' })
  await expect(confirm.getByText('Volume 1.cbz')).toBeVisible()
  await expect(confirm.getByText('Volume 2.cbz')).toBeVisible()
  await confirm.getByRole('button', { name: 'Mark 3 reviewed and unmapped' }).click()
  await expect(confirm).toBeHidden()
  await expect(page.getByText(/3 file\(s\) reviewed and unmapped/)).toBeVisible()
  expect(resolved).toEqual({ fileIds: [301, 302, 303] })
})

test('a release pack maps by one card choice and leaves unreadable files for review', async ({ page }) => {
  let matched: unknown
  await page.route('**/api/v1/activity?seriesId=*', (route) =>
    route.fulfill({ json: activityFixture([], {}, packFiles) }),
  )
  await page.route('**/api/v1/series/901/', (route) =>
    route.fulfill({
      json: {
        series: { id: 901, title: 'One Piece' },
        editions: [{ id: 30, seriesId: 901, editionName: 'Standard Edition', language: 'en', type: 'volume' }],
      },
    }),
  )
  await page.route('**/api/v1/series/901/versions', (route) =>
    route.fulfill({
      json: [{ id: 10, editionId: 30, name: 'Default', isDefault: true, monitored: true, profileId: 0 }],
    }),
  )
  await page.route('**/api/v1/series/901/review/choices/preview', (route) => route.fulfill({ json: {
    token: 'pack-token', files: [
      reviewFile(301, [reviewBook({ versionId: 10, bookId: 20, editionId: 30, ledger: 'volume', number: 100 }, 'volume 1')]),
      reviewFile(302, [reviewBook({ versionId: 10, bookId: 21, editionId: 30, ledger: 'volume', number: 200 }, 'volume 2')]),
      reviewFile(303, [], 'Its number is shared by two books of this edition'),
    ],
  } }))
  await page.route('**/api/v1/series/901/review/choices', (route) => {
    matched = route.request().postDataJSON()
    return route.fulfill({ json: [{ fileId: 301, status: 'imported', note: '' }, { fileId: 302, status: 'imported', note: '' }, { fileId: 303, status: 'later', note: '' }] })
  })
  await page.goto('/queue')
  await page.getByRole('button', { name: 'Match whole release' }).click()
  const dialog = page.getByRole('dialog', { name: 'Match whole release' })
  await expect(dialog.getByText('3 files waiting for review')).toBeVisible()
  await dialog.getByRole('combobox', { name: 'Card', exact: true }).click()
  await page.getByRole('option', { name: 'en · Standard Edition' }).click()
  // Each file previews by its own numbers; the unreadable one stays orange.
  await expect(dialog.getByRole('checkbox', { name: 'Import volume 1', exact: true })).toBeVisible()
  await expect(dialog.getByRole('checkbox', { name: 'Import volume 2', exact: true })).toBeVisible()
  await expect(dialog.getByText('Its number is shared by two books of this edition')).toBeVisible()
  expect(matched).toBeUndefined()
  await dialog.getByRole('button', { name: 'Apply choices' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByText('Imported 2; kept existing 0; replaced 0; moved 0 to trash', { exact: true })).toBeVisible()
  expect(matched).toMatchObject({ request: { versionId: 10 }, choices: [
    { fileId: 301, disposition: 'keep' }, { fileId: 302, disposition: 'keep' }, { fileId: 303, disposition: 'later' },
  ] })
})

test('large packs use bounded requests for preview, match and decline', async ({ page }) => {
  const files = Array.from({ length: 501 }, (_, n) => ({
    ...packFiles[0], id: `file-${n + 1000}`, itemId: n + 1000, title: `Volume ${n + 1}.cbz`,
  }))
  const batches = { preview: [] as number[][], match: [] as number[][], decline: [] as number[][] }
  await page.route('**/api/v1/activity?seriesId=*', (route) =>
    route.fulfill({ json: activityFixture([], {}, files) }),
  )
  await page.route('**/api/v1/series/901/', (route) => route.fulfill({ json: {
    series: { id: 901, title: 'One Piece' },
    editions: [{ id: 30, seriesId: 901, editionName: 'Standard Edition', language: 'en', type: 'volume' }],
  } }))
  await page.route('**/api/v1/series/901/versions', (route) => route.fulfill({ json: [
    { id: 10, editionId: 30, name: 'Default', isDefault: true, monitored: true, profileId: 0 },
  ] }))
  await page.route('**/api/v1/series/901/review/choices/preview', (route) => {
    const ids: number[] = route.request().postDataJSON().files.map((f: { fileId: number }) => f.fileId)
    expect(ids.length).toBeLessThanOrEqual(500)
    batches.preview.push(ids)
    return route.fulfill({ json: { token: 'large-pack', files: ids.map((fileId) => reviewFile(fileId, [reviewBook({ versionId: 10, bookId: fileId, editionId: 30, ledger: 'volume', number: fileId * 100 }, `volume ${fileId}`)])) } })
  })
  await page.route('**/api/v1/series/901/review/choices', (route) => {
    const ids: number[] = route.request().postDataJSON().choices.map((f: { fileId: number }) => f.fileId)
    expect(ids).toHaveLength(500)
    batches.match.push(ids)
    return route.fulfill({ json: ids.map((fileId) => ({ fileId, status: 'imported', note: '' })) })
  })
  await page.route('**/api/v1/series/901/review/resolve', (route) => {
    const ids: number[] = route.request().postDataJSON().fileIds
    expect(ids.length).toBeLessThanOrEqual(500)
    batches.decline.push(ids)
    return route.fulfill({ json: { resolved: ids, skipped: [] } })
  })
  await page.goto('/queue')
  await page.getByRole('button', { name: 'Match whole release' }).click()
  const dialog = page.getByRole('dialog', { name: 'Match whole release' })
  await dialog.getByRole('combobox', { name: 'Card', exact: true }).click()
  await page.getByRole('option', { name: 'en · Standard Edition' }).click()
  await dialog.getByRole('button', { name: 'Apply choices' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByText('Imported 500; kept existing 0; replaced 0; moved 0 to trash', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Decline all (501)' }).click()
  const confirmation = page.getByRole('dialog', { name: 'Decline all 501 files?' })
  await confirmation.getByRole('button', { name: 'Mark 501 reviewed and unmapped' }).click()
  await expect(confirmation).toBeHidden()
  for (const action of ['preview', 'match'] as const) {
    expect(batches[action].every((ids) => ids.length === 500)).toBe(true)
    expect([...new Set(batches[action].flat())]).toEqual(files.slice(0, 500).map((f) => f.itemId))
  }
  expect(batches.decline.map((ids) => ids.length)).toEqual([500, 1])
  expect(batches.decline.flat()).toEqual(files.map((f) => f.itemId))
})
