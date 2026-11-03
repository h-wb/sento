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
    await page.getByRole('button', { name: 'Edit mappings', exact: true }).click()
    const editor = page.getByRole('dialog', { name: 'Match imported file' })
    await expect(editor.getByText('Current score: 150', { exact: true })).toBeVisible()
    await expect(editor.getByText('Profile: File score policy', { exact: true })).toBeVisible()
    await expect(editor.getByText('File score test group: +150', { exact: true })).toBeVisible()
    await editor.getByRole('button', { name: 'Close dialog', exact: true }).click()

    // Editing a profile must change today's file score without rescanning.
    expect((await request.put(`/api/v1/profiles/${profileId}`, {
      data: { ...policy, id: profileId, formatScores: { [formatId]: -25 } },
    })).ok()).toBeTruthy()
    await page.reload()
    await page.getByRole('button', { name: 'Edit mappings', exact: true }).click()
    await expect(editor.getByText('Current score: -25', { exact: true })).toBeVisible()
    await expect(editor.getByText('File score test group: -25', { exact: true })).toBeVisible()
    await editor.getByRole('textbox', { name: 'Confirmed release group' }).fill('OtherGroup')
    await expect(editor.getByText('The score uses saved facts. Save your corrections to update it.')).toBeVisible()
    await expect(editor.getByText('Current score: -25', { exact: true })).toBeVisible()
    await editor.getByRole('button', { name: 'Save coverage' }).click()
    await expect(editor).toBeHidden()
    await page.getByRole('button', { name: 'Edit mappings', exact: true }).click()
    await expect(editor.getByText('Current score: 0', { exact: true })).toBeVisible()
    await expect(editor.getByText('File score test group: -25', { exact: true })).toHaveCount(0)
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await editor.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
    await editor.getByRole('button', { name: 'Close dialog', exact: true }).click()

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
