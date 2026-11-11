import { expect, test } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import type { CollectedVersion, File as SeriesFile } from '../src/types/store'
import type { BookStatus } from '../src/types/service'

test('file details show current profile scores and refresh after profile or fact changes', async ({ page, request }) => {
  const added = await request.post('/api/v1/series/', { data: { mangabakaId: 103, monitor: 'all' } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  let profileId = 0
  let formatId = 0
  try {
    const detail = await (await request.get(`/api/v1/series/${id}/`)).json()
    const dir = path.resolve('.playwright', 'score-import')
    await mkdir(dir, { recursive: true })
    // A tiny valid CBZ exercises the real scanner and stored file facts.
    const archive = 'UEsDBBQAAAAIAGusY10zMwpxPwAAAEQAAAAHAAAAMDAxLnBuZ+sM8HPn5ZLiYmBg4PX0cAkC0owgzMECJLfK8DABKW5PF8eQilvJKT/4GRhZGRnVJR6nAYUZPF39XNY5JTQBAFBLAQIUAxQAAAAIAGusY10zMwpxPwAAAEQAAAAHAAAAAAAAAAAAAACAAQAAAAAwMDEucG5nUEsFBgAAAAABAAEANQAAAGQAAAAAAA=='
    await writeFile(path.join(dir, `${detail.series.title} v01 (Digital) [LuCaZ].cbz`), Buffer.from(archive, 'base64'))
    expect((await request.post(`/api/v1/series/${id}/import`, { data: { path: dir } })).ok()).toBeTruthy()
    const files: SeriesFile[] = await (await request.get(`/api/v1/series/${id}/files`)).json()
    expect(files).toHaveLength(1)
    const file = files[0]
    const versions: CollectedVersion[] = await (await request.get(`/api/v1/series/${id}/versions`)).json()
    const version = versions.find((v) => v.isDefault)!
    const format = await request.post('/api/v1/formats', {
      data: { name: 'File score test group', conditions: [{ type: 'group', value: '^LuCaZ$' }] },
    })
    expect(format.ok()).toBeTruthy()
    formatId = (await format.json()).id
    const policy = { name: 'File score policy', minFormatScore: -10000, formatScores: { [formatId]: 150 } }
    const created = await request.post('/api/v1/profiles/', { data: policy })
    expect(created.ok()).toBeTruthy()
    profileId = (await created.json()).id
    expect((await request.put(`/api/v1/series/${id}/versions/${version.id}`, {
      data: { ...version, profileId },
    })).ok()).toBeTruthy()
    const books: BookStatus[] = await (await request.get(`/api/v1/series/${id}/versions/${version.id}/books`)).json()
    const book = books.find((b) => b.book.sequenceNum === 100)!
    expect((await request.put(`/api/v1/series/${id}/files/${file.id}/mapping`, {
      data: { books: [{ versionId: version.id, editionId: version.editionId, bookId: book.book.id, ledger: 'volume', number: 100 }] },
    })).ok()).toBeTruthy()

    await page.goto(`/series/${id}?view=files`)
    await page.getByRole('button', { name: 'Edit file', exact: true }).click()
    const editor = page.getByRole('dialog', { name: 'File', exact: true })
    // Each card that can use the file shows what its profile makes of it.
    await editor.getByText('Score 150 · File score policy', { exact: true }).click()
    await expect(editor.getByText('File score test group: +150')).toBeVisible()
    await editor.getByRole('button', { name: 'Close dialog', exact: true }).click()

    // Editing a profile must change today's file score without rescanning.
    expect((await request.put(`/api/v1/profiles/${profileId}`, {
      data: { ...policy, id: profileId, formatScores: { [formatId]: -25 } },
    })).ok()).toBeTruthy()
    await page.reload()
    await page.getByRole('button', { name: 'Edit file', exact: true }).click()
    await editor.getByText('Score -25 · File score policy', { exact: true }).click()
    await expect(editor.getByText('File score test group: -25')).toBeVisible()
    // A correction is judged as it is typed, before it is saved.
    await editor.getByRole('button', { name: 'Correct…', exact: true }).click()
    await editor.getByRole('textbox', { name: 'Release group' }).fill('OtherGroup')
    await expect(editor.getByText('Score 0 · File score policy', { exact: true })).toBeVisible()
    await editor.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(editor).toBeHidden()
    await page.getByRole('button', { name: 'Edit file', exact: true }).click()
    await expect(editor.getByText('Score 0 · File score policy', { exact: true })).toBeVisible()
    await expect(editor.getByText('File score test group: -25')).toHaveCount(0)
    // What was read wrong can be said by hand, flags included.
    await editor.getByRole('button', { name: 'Correct…', exact: true }).click()
    await expect(editor.getByText(/Set by you/)).toHaveCount(0)
    await editor.getByText('Digital', { exact: true }).click()
    await expect(editor.getByText('Set by you: Digital off. A rescan keeps these.')).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await editor.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
    await editor.getByRole('button', { name: 'Close dialog', exact: true }).click()
    await page.getByRole('button', { name: 'Discard changes', exact: true }).click()

    await page.getByRole('tab', { name: 'Editions', exact: true }).click()
    const card = page.locator('.mantine-Card-root').filter({ hasText: 'File score policy' }).first()
    await card.getByRole('button', { name: '1', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Volume 1', exact: true })
    await expect(dialog.getByText('Current score: 0', { exact: true })).toBeVisible()
    await expect(dialog.getByText('Profile: File score policy', { exact: true })).toBeVisible()
  } finally {
    await request.delete(`/api/v1/series/${id}/`)
    if (profileId) await request.delete(`/api/v1/profiles/${profileId}`)
    if (formatId) await request.delete(`/api/v1/formats/${formatId}`)
  }
})
