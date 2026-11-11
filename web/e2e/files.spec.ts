import { expect, test } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import type { CollectedVersion, File as SeriesFile } from '../src/types/store'
import { reviewArchives } from './review-choice-fixtures'

// Files whose names say "Colored" land on a card whose profile refuses
// color. The card says so and offers the ways out; saying the files were
// read wrong gives them to the card at once, and the Files table shows what
// each file is used for and acts on a selection.
test('a card explains refused files, and they can be corrected and moved together', async ({ page, request }) => {
  const added = await request.post('/api/v1/series/', { data: { mangabakaId: 103, monitor: 'all' } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  let profileId = 0
  try {
    const detail = await (await request.get(`/api/v1/series/${id}/`)).json()
    const formats: { id: number; name: string }[] = await (await request.get('/api/v1/formats')).json()
    const colored = formats.find((f) => f.name === 'Colored')!
    const created = await request.post('/api/v1/profiles/', {
      data: { name: 'No color', minFormatScore: -10000, refused: [colored.id] },
    })
    expect(created.ok()).toBeTruthy()
    profileId = (await created.json()).id
    const versions: CollectedVersion[] = await (await request.get(`/api/v1/series/${id}/versions`)).json()
    const version = versions.find((v) => v.isDefault && v.editionId === detail.editions.find((e: { type: string }) => e.type === 'volume').id)!
    expect((await request.put(`/api/v1/series/${id}/versions/${version.id}`, { data: { ...version, profileId } })).ok()).toBeTruthy()

    const dir = path.resolve('.playwright', 'files-import')
    await mkdir(dir, { recursive: true })
    for (const n of [1, 2]) {
      await writeFile(path.join(dir, `${detail.series.title} v0${n} (Scan) (Colored).cbz`), Buffer.from(reviewArchives[n], 'base64'))
    }
    expect((await request.post(`/api/v1/series/${id}/import`, { data: { path: dir } })).ok()).toBeTruthy()
    const files: SeriesFile[] = await (await request.get(`/api/v1/series/${id}/files`)).json()
    expect(files).toHaveLength(2)
    expect(files.every((f) => f.facts.flags?.colored)).toBe(true)

    await page.goto(`/series/${id}`)
    const card = page.locator('.mantine-Card-root').filter({ hasText: 'No color' }).first()
    await expect(card.getByText('Vol. 1–2 are on disk, but this edition doesn’t use their files', { exact: true })).toBeVisible()
    await expect(card.getByRole('button', { name: 'Collect them with another profile…' })).toBeVisible()
    await card.getByRole('button', { name: 'Correct what they are…' }).click()
    const facts = page.getByRole('dialog', { name: 'Correct what these files are', exact: true })
    await expect(facts.getByRole('button', { name: 'Correct 2 files', exact: true })).toBeDisabled()
    await facts.getByRole('combobox', { name: 'These files are not', exact: true }).click()
    await page.getByRole('option', { name: 'Colored', exact: true }).click()
    // The list stays open for more flags; clicking away closes it.
    await facts.getByText(/^2 files\./).click()
    await facts.getByRole('button', { name: 'Correct 2 files', exact: true }).click()
    await expect(facts).toBeHidden()
    await expect(card.getByText(/on disk, but this edition doesn’t use/)).toHaveCount(0)
    await expect(card.getByText(/2 owned/)).toBeVisible()
    const after: SeriesFile[] = await (await request.get(`/api/v1/series/${id}/files`)).json()
    expect(after.every((f) => f.facts.flags?.colored === false && f.facts.sources?.['flag:colored'] === 'manual')).toBe(true)

    // The Files table says what each file is for and acts on a selection.
    await page.getByRole('tab', { name: /^Files/ }).click()
    await expect(page.getByRole('cell', { name: /^Vol\. 1 · / })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Use for an edition…' })).toHaveCount(0)
    await page.getByRole('checkbox', { name: 'Select all files' }).check()
    await expect(page.getByText('2 selected', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Use for an edition…' }).click()
    const move = page.getByRole('dialog', { name: 'Use files for an edition', exact: true })
    await move.getByRole('combobox', { name: 'Edition', exact: true }).click()
    await page.getByRole('option', { name: /Standard Edition/ }).first().click()
    await expect(move.getByRole('status')).toHaveText(/2 of 2 can be used by/)
    await move.getByRole('button', { name: 'Use 2 files', exact: true }).click()
    await expect(move).toBeHidden()

    // One file: its books, what it is, and the cards that use it.
    await page.getByRole('button', { name: 'Edit file', exact: true }).first().click()
    const editor = page.getByRole('dialog', { name: 'File', exact: true })
    await editor.getByRole('button', { name: 'Correct…', exact: true }).click()
    await expect(editor.getByText('Set by you: Colored off. A rescan keeps these.')).toBeVisible()
    await expect(editor.getByRole('checkbox', { name: /Keep this copy for volume/ })).toBeChecked()
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await editor.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
    await editor.getByRole('button', { name: 'Close dialog', exact: true }).click()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  } finally {
    await request.delete(`/api/v1/series/${id}/`)
    if (profileId) await request.delete(`/api/v1/profiles/${profileId}`)
  }
})
