import { expect, test } from '@playwright/test'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { reviewArchives } from './review-choice-fixtures'
import type { CollectedVersion, File as SeriesFile, TrashedFile } from '../src/types/store'
import type { BookStatus } from '../src/types/service'

test('a mixed pack keeps the existing volume, trashes its duplicate, and supports an explicit replacement', async ({ page, request }, testInfo) => {
  const added = await request.post('/api/v1/series/', { data: { mangabakaId: 103, monitor: 'all' } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  let profileId = 0
  let formatId = 0
  try {
    const detail = await (await request.get(`/api/v1/series/${id}/`)).json()
    const versions: CollectedVersion[] = await (await request.get(`/api/v1/series/${id}/versions`)).json()
    const version = versions.find((v) => v.isDefault)!
    const oldDir = path.resolve('.playwright', 'review-existing')
    const incomingDir = path.resolve('.playwright', 'review-incoming')
    await mkdir(oldDir, { recursive: true })
    await mkdir(incomingDir, { recursive: true })
    const oldPath = path.join(oldDir, `${detail.series.title} v04 [Madokami].cbz`)
    await writeFile(oldPath, Buffer.from(reviewArchives[0], 'base64'))
    expect((await request.post(`/api/v1/series/${id}/import`, { data: { path: oldDir } })).ok()).toBeTruthy()
    let files: SeriesFile[] = await (await request.get(`/api/v1/series/${id}/files`)).json()
    expect(files).toHaveLength(1)
    const old = files[0]
    // An additional numbering/language makes the incoming edition ambiguous,
    // creating real review questions instead of intercepting Sento's API.
    expect((await request.post(`/api/v1/series/${id}/editions`, { data: {
      editionName: 'Alternate numbering', language: 'fr', type: 'volume', basedOn: 0, volumes: 6, monitored: true,
    } })).ok()).toBeTruthy()
    const format = await request.post('/api/v1/formats', { data: {
      name: 'Pack incoming group', conditions: [{ type: 'group', value: '^LuCaZ$' }],
    } })
    expect(format.ok()).toBeTruthy()
    formatId = (await format.json()).id
    const policy = await request.post('/api/v1/profiles/', { data: {
      name: 'Pack comparison', formatScores: { [formatId]: 250 }, minFormatScore: -10000,
      upgradesEnabled: true, minUpgradeScore: 1, upgradeUntilScore: 500,
    } })
    expect(policy.ok()).toBeTruthy()
    profileId = (await policy.json()).id
    expect((await request.put(`/api/v1/series/${id}/versions/${version.id}`, { data: { ...version, profileId } })).ok()).toBeTruthy()
    for (let n = 1; n <= 6; n++) await writeFile(
      path.join(incomingDir, `${detail.series.title} v0${n} (Digital) [LuCaZ].cbz`), Buffer.from(reviewArchives[n], 'base64'),
    )
    const scanned = await request.post(`/api/v1/series/${id}/import`, { data: { path: incomingDir } })
    expect(scanned.ok()).toBeTruthy()
    files = await (await request.get(`/api/v1/series/${id}/files`)).json()
    expect(files.filter((f) => f.needsReview)).toHaveLength(6)
    const incoming4 = files.find((f) => f.path.includes('v04 (Digital)'))!
    await page.goto('/queue')
    await page.getByRole('button', { name: 'Match whole release', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Match whole release', exact: true })
    await dialog.getByRole('combobox', { name: 'Card', exact: true }).click()
    await page.getByRole('option', { name: 'en · Standard Edition', exact: true }).click()
    await expect(dialog.getByRole('checkbox', { name: 'Use incoming instead for volume 4', exact: true })).not.toBeChecked()
    await expect(dialog.getByText(`Keep existing: ${path.basename(old.path)}`, { exact: false })).toBeVisible()
    await expect(dialog.getByText('Import 5 · Keep existing 1', { exact: true })).toBeVisible()
    await dialog.getByRole('combobox', { name: `After review: ${path.basename(incoming4.path)}`, exact: true }).click()
    await page.getByRole('option', { name: 'Move to trash', exact: true }).click()
    await expect(dialog.getByText('Import 5 · Keep existing 1 · Trash 1', { exact: true })).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath('pack-choices-desktop.png'), fullPage: true })
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
    await dialog.getByRole('button', { name: 'Apply choices', exact: true }).click()
    await expect(dialog).toBeHidden()
    const books = async (): Promise<BookStatus[]> => (await (await request.get(`/api/v1/series/${id}/versions/${version.id}/books`)).json())
    let status = await books()
    expect(status.find((b) => b.book.sequenceNum === 400)?.fileIds).toEqual([old.id])
    for (const n of [1, 2, 3, 5, 6]) expect(status.find((b) => b.book.sequenceNum === n * 100)?.fileIds).toHaveLength(1)
    expect(await readFile(old.path)).toEqual(Buffer.from(reviewArchives[0], 'base64'))
    const trash: TrashedFile[] = await (await request.get(`/api/v1/series/${id}/files/trash`)).json()
    expect(trash.map((f) => f.fileId)).toEqual([incoming4.id])
    expect((await request.post(`/api/v1/series/${id}/files/trash/${incoming4.id}/restore`)).ok()).toBeTruthy()
    status = await books()
    expect(status.find((b) => b.book.sequenceNum === 400)?.fileIds).toEqual([old.id])

    // Restoring an unused copy retains confirmed contents but does not select it.
    // A deliberate choice in the single-file editor can replace the old copy.
    await page.goto(`/series/${id}?view=files`)
    const row = page.getByRole('row').filter({ has: page.getByText(path.basename(incoming4.path), { exact: true }) }).last()
    await row.getByRole('button', { name: 'Edit file', exact: true }).click()
    const editor = page.getByRole('dialog', { name: 'File', exact: true })
    await expect(editor.getByRole('checkbox', { name: 'Use incoming instead for volume 4', exact: true })).not.toBeChecked()
    await editor.getByRole('checkbox', { name: 'Use incoming instead for volume 4', exact: true }).check()
    await expect(editor.getByText('Replace 1', { exact: true })).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath('explicit-replacement-mobile.png'), fullPage: true })
    await editor.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(editor).toBeHidden()
    status = await books()
    expect(status.find((b) => b.book.sequenceNum === 400)?.fileIds).toEqual([incoming4.id])
    const retained: SeriesFile[] = await (await request.get(`/api/v1/series/${id}/files`)).json()
    expect(await readFile(retained.find((f) => f.id === old.id)!.path)).toEqual(Buffer.from(reviewArchives[0], 'base64'))
  } finally {
    await request.delete(`/api/v1/series/${id}/`)
    if (profileId) await request.delete(`/api/v1/profiles/${profileId}`)
    if (formatId) await request.delete(`/api/v1/formats/${formatId}`)
  }
})
