import { expect, test } from '@playwright/test'
import { activityFixture } from './activity-fixtures'
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
  await page.route('**/api/v1/series/901/books', (route) =>
    route.fulfill({
      json: {
        '10': [1, 2].map((n) => ({
          versionId: 10,
          book: { id: 19 + n, editionId: 30, sequenceNum: n * 100, sequenceStr: String(n), countType: 'main' },
          fileIds: [],
          onDiskFileIds: [],
        })),
      },
    }),
  )
  await page.route('**/api/v1/series/901/review/match', (route) => {
    matched = route.request().postDataJSON()
    return route.fulfill({
      json: [
        { fileId: 301, books: [{ versionId: 10, bookId: 20, editionId: 30, ledger: 'volume', number: 100 }], label: 'volume 1' },
        { fileId: 302, books: [{ versionId: 10, bookId: 21, editionId: 30, ledger: 'volume', number: 200 }], label: 'volume 2' },
        { fileId: 303, books: [], note: "The file's own name does not say which books it holds" },
      ],
    })
  })
  await page.goto('/queue')
  await page.getByRole('button', { name: 'Match whole release' }).click()
  const dialog = page.getByRole('dialog', { name: 'Match whole release' })
  await expect(dialog.getByText('3 files waiting for review')).toBeVisible()
  await dialog.getByRole('combobox', { name: 'Card', exact: true }).click()
  await page.getByRole('option', { name: 'en · Standard Edition' }).click()
  // Each file previews by its own numbers; the unreadable one stays orange.
  await expect(dialog.getByText('volume 1', { exact: true })).toBeVisible()
  await expect(dialog.getByText('volume 2', { exact: true })).toBeVisible()
  await expect(dialog.getByText('Name does not say which books it holds')).toBeVisible()
  await dialog.getByRole('button', { name: 'Match 2 of 3 files' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByText(/Matched 2 of 3 files/)).toBeVisible()
  await expect(page.getByText(/1 file\(s\) left for review/)).toBeVisible()
  expect(matched).toEqual({ versionId: 10, fileIds: [301, 302, 303] })
})
