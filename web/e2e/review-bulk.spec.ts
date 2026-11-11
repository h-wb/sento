import { expect, test } from '@playwright/test'
import { activityFixture } from './activity-fixtures'
import { reviewBook, reviewFile, reviewScore } from './review-choice-fixtures'
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
  await dialog.getByRole('combobox', { name: 'Edition', exact: true }).click()
  await page.getByRole('option', { name: 'en · Standard Edition' }).click()
  // Each file previews by its own numbers; the unreadable one stays orange.
  await expect(dialog.getByRole('checkbox', { name: 'Import volume 1', exact: true })).toBeVisible()
  await expect(dialog.getByRole('checkbox', { name: 'Import volume 2', exact: true })).toBeVisible()
  await expect(dialog.getByText('Its number is shared by two books of this edition')).toBeVisible()
  expect(matched).toBeUndefined()
  await dialog.getByRole('button', { name: 'Apply choices' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByText('Saved: import 2 · left for later 1', { exact: true })).toBeVisible()
  expect(matched).toMatchObject({ request: { versionId: 10 }, choices: [
    { fileId: 301, disposition: 'keep' }, { fileId: 302, disposition: 'keep' }, { fileId: 303, disposition: 'later' },
  ] })
})

for (const [preset, existing] of [[false, false], [false, true], [true, false], [true, true]]) {
  test(`duplicate incoming books ${existing ? 'with an existing copy' : 'without an existing copy'} remain usable with ${preset ? 'the missing-books preset' : 'an explicit copy switch'}`, async ({ page }) => {
    const book = (n: number) => ({ versionId: 10, bookId: 20 + n, editionId: 30, ledger: 'volume', number: n * 100 })
    let submitted: { choices: { fileId: number; use: typeof firstBook[] }[] } | undefined
    const firstBook = book(1)
    const duplicate = () => ({
      ...reviewBook(firstBook, 'volume 1'), useIncoming: !existing,
      owners: existing ? [{ fileId: 900, path: '/library/Existing volume 1.cbz', group: 'Old group', source: 'source', pinned: false, present: true, score: reviewScore }] : [],
    })
    await page.route('**/api/v1/activity?seriesId=*', (route) => route.fulfill({ json: activityFixture([], {}, packFiles) }))
    await page.route('**/api/v1/series/901/', (route) => route.fulfill({ json: {
      series: { id: 901, title: 'One Piece' },
      editions: [{ id: 30, seriesId: 901, editionName: 'Standard Edition', language: 'en', type: 'volume' }],
    } }))
    await page.route('**/api/v1/series/901/versions', (route) => route.fulfill({ json: [
      { id: 10, editionId: 30, name: 'Default', isDefault: true, monitored: true, profileId: 0 },
    ] }))
    // Even a cached preview that proposes both copies must remain usable.
    await page.route('**/api/v1/series/901/review/choices/preview', (route) => route.fulfill({ json: {
      token: 'duplicate-pack', files: [
        reviewFile(301, [duplicate()]),
        reviewFile(302, [duplicate()]),
        reviewFile(303, [reviewBook(book(2), 'volume 2')]),
      ],
    } }))
    await page.route('**/api/v1/series/901/review/choices', (route) => {
      submitted = route.request().postDataJSON()
      return route.fulfill({ json: [301, 302, 303].map((fileId) => ({ fileId, status: 'kept', note: '' })) })
    })
    await page.goto('/queue')
    await page.getByRole('button', { name: 'Match whole release', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Match whole release', exact: true })
    const copies = dialog.getByRole('checkbox', { name: existing ? 'Use incoming instead for volume 1' : 'Import volume 1', exact: true })
    if (existing) {
      await expect(copies.nth(0)).not.toBeChecked()
      await expect(dialog.getByText('Import 1 · Keep existing 1', { exact: true })).toBeVisible()
    } else await expect(copies.nth(0)).toBeChecked()
    await expect(copies.nth(1)).not.toBeChecked()
    await expect(dialog.getByRole('checkbox', { name: 'Import volume 2', exact: true })).toBeChecked()
    await copies.nth(1).check()
    await expect(copies.nth(0)).not.toBeChecked()
    await expect(copies.nth(1)).toBeChecked()
    if (preset) {
      await dialog.getByRole('button', { name: 'Import missing; keep existing', exact: true }).click()
      if (existing) await expect(copies.nth(0)).not.toBeChecked()
      else await expect(copies.nth(0)).toBeChecked()
      await expect(copies.nth(1)).not.toBeChecked()
    }
    const summary = existing ? (preset ? 'Import 1 · Keep existing 1' : 'Import 1 · Replace 1') : 'Import 2'
    await expect(dialog.getByText(summary, { exact: true })).toBeVisible()
    if (existing && !preset) {
      await expect(dialog.getByText('Keep existing: Existing volume 1.cbz', { exact: false })).toHaveCount(0)
      await expect(dialog.getByText('Replace: Existing volume 1.cbz', { exact: false })).toHaveCount(1)
      await expect(dialog.getByText('Replaced by another incoming archive: Existing volume 1.cbz', { exact: false })).toHaveCount(1)
    }
    await dialog.getByRole('button', { name: 'Apply choices', exact: true }).click()
    await expect(dialog).toBeHidden()
    expect(submitted?.choices.map((c) => c.use.map((b) => b.bookId))).toEqual(preset ? [existing ? [] : [21], [], [22]] : [[], [21], [22]])
  })
}

for (const refused of [false, true]) {
  test(`editing an existing ${refused ? 'profile-refused ' : ''}copy counts retention, deselection and replacement correctly`, async ({ page }) => {
    const ref = { versionId: 10, bookId: 21, editionId: 30, ledger: 'volume', number: 100 }
    await page.route('**/api/v1/activity?seriesId=*', (route) => route.fulfill({ json: activityFixture([], {}, packFiles) }))
    await page.route('**/api/v1/series/901/', (route) => route.fulfill({ json: {
      series: { id: 901, title: 'One Piece' },
      editions: [{ id: 30, seriesId: 901, editionName: 'Standard Edition', language: 'en', type: 'volume' }],
    } }))
    await page.route('**/api/v1/series/901/versions', (route) => route.fulfill({ json: [
      { id: 10, editionId: 30, name: 'Default', isDefault: true, monitored: true, profileId: 0 },
    ] }))
    await page.route('**/api/v1/series/901/review/choices/preview', (route) => route.fulfill({ json: {
      token: 'selected-copy', files: [
        { ...reviewFile(301, [{ ...reviewBook(ref, 'volume 1'), currentSelected: true, reason: refused ? 'Score is below the required minimum' : '' }]), trashReason: 'A card uses this file, so it cannot go to trash: untick it for every card first' },
        reviewFile(302, [{ ...reviewBook(ref, 'volume 1'), useIncoming: false, owners: [
          { fileId: 301, path: '/library/Volume 301.cbz', group: '1r0n', source: 'torrent', pinned: false, present: true, score: reviewScore },
        ] }]),
        reviewFile(303, [reviewBook({ ...ref, bookId: 22, number: 200 }, 'volume 2')]),
      ],
    } }))
    await page.goto('/queue')
    await page.getByRole('button', { name: 'Match whole release', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Match whole release', exact: true })
    const current = dialog.getByRole('checkbox', { name: 'Keep this copy for volume 1', exact: true })
    const incoming = dialog.getByRole('checkbox', { name: 'Use incoming instead for volume 1', exact: true })
    await expect(current).toBeChecked()
    await expect(current).toBeEnabled()
    await expect(incoming).not.toBeChecked()
    await expect(dialog.getByRole('status')).toHaveText('Import 1 · Keep existing 1')
    await dialog.getByRole('button', { name: 'Import missing; keep existing', exact: true }).click()
    await expect(current).toBeChecked()
    await current.uncheck()
    await expect(dialog.getByRole('status')).toHaveText('Import 1 · Still missing 1')
    await incoming.check()
    await expect(current).not.toBeChecked()
    await expect(dialog.getByRole('status')).toHaveText('Import 1 · Replace 1')
    await incoming.uncheck()
    await dialog.getByRole('combobox', { name: 'After review: Volume 301.cbz', exact: true }).click()
    await expect(page.getByRole('option', { name: 'Move to trash', exact: true })).toHaveAttribute('data-combobox-disabled', 'true')
    await page.getByRole('option', { name: 'Leave for later', exact: true }).click()
    await expect(dialog.getByRole('status')).toHaveText('Import 1 · Keep existing 1 · Left for later 1')
  })
}

for (const unknownFacts of [false, true]) {
  test(`missing summary deduplicates ${unknownFacts ? 'unknown-facts' : 'policy-refused'} books and distinguishes files left for later`, async ({ page }) => {
    const ref = { versionId: 10, bookId: 21, editionId: 30, ledger: 'volume', number: 100 }
    const blockedBook = {
      ...reviewBook(ref, 'volume 1'), useIncoming: false,
      incoming: { ...reviewScore, factsKnown: !unknownFacts },
      reason: unknownFacts ? '' : 'Required group does not match',
    }
    await page.route('**/api/v1/activity?seriesId=*', (route) => route.fulfill({ json: activityFixture([], {}, packFiles) }))
    await page.route('**/api/v1/series/901/', (route) => route.fulfill({ json: {
      series: { id: 901, title: 'One Piece' },
      editions: [{ id: 30, seriesId: 901, editionName: 'Standard Edition', language: 'en', type: 'volume' }],
    } }))
    await page.route('**/api/v1/series/901/versions', (route) => route.fulfill({ json: [
      { id: 10, editionId: 30, name: 'Default', isDefault: true, monitored: true, profileId: 0 },
    ] }))
    await page.route('**/api/v1/series/901/review/choices/preview', (route) => route.fulfill({ json: {
      token: 'missing-pack', files: [
        reviewFile(301, [blockedBook]), reviewFile(302, [blockedBook]),
        reviewFile(303, [reviewBook({ ...ref, bookId: 22, number: 200 }, 'volume 2')]),
      ],
    } }))
    await page.goto('/queue')
    await page.getByRole('button', { name: 'Match whole release', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Match whole release', exact: true })
    await expect(dialog.getByRole('status')).toHaveText('Import 1 · Still missing 1 · Left for later 2')
    await dialog.getByRole('combobox', { name: 'After review: Volume 301.cbz', exact: true }).click()
    await page.getByRole('option', { name: 'Keep, unused', exact: true }).click()
    await expect(dialog.getByRole('status')).toHaveText('Import 1 · Still missing 1 · Left for later 1')
    if (unknownFacts) {
      await dialog.getByRole('checkbox', { name: 'Import volume 1', exact: true }).nth(0).check()
      await expect(dialog.getByRole('status')).toHaveText('Import 2 · Left for later 1')
    }
  })
}

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
  // Past one page, archives open as one-line summaries, expanded on demand.
  await expect(dialog.getByText(/Showing 1–25 of 500 files/)).toBeVisible()
  await expect(dialog.getByRole('checkbox', { name: 'Import volume 1000', exact: true })).toHaveCount(0)
  await dialog.getByRole('button', { name: /Volume 1000\.cbz/ }).click()
  await expect(dialog.getByRole('checkbox', { name: 'Import volume 1000', exact: true })).toBeVisible()
  await dialog.getByRole('combobox', { name: 'Edition', exact: true }).click()
  await page.getByRole('option', { name: 'en · Standard Edition' }).click()
  await dialog.getByRole('button', { name: 'Apply choices' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByText('Saved: import 500', { exact: true })).toBeVisible()
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
