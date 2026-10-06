import { expect, test } from '@playwright/test'

const LAZY = 'eu.kanade.tachiyomi.extension.all.lazysource'

type Ext = { pkgName: string; installedVersion?: string; sources?: { id: string }[] }

// With no repo configured there is nothing to pick. The dialog says why
// and takes the repo URL right there, instead of sending the user to the
// Connections page and back. (Later tests rely on the repo added here.)
test('an empty catalog offers to add a repo in place', async ({ page }) => {
  await page.goto('/indexers')
  await page.getByRole('button', { name: 'Add indexer' }).click()
  const modal = page.getByRole('dialog')
  await modal.getByText('Extension source').click()

  await expect(modal.getByText('No extensions to choose from yet')).toBeVisible()
  await modal.getByRole('textbox', { name: 'Extension repo URL' }).fill('https://repo.example/index.min.json')
  await modal.getByRole('button', { name: 'Add repo' }).click()

  // The catalog loads and the picker replaces the prompt.
  await expect(modal.getByPlaceholder('Search extensions…')).toBeVisible()
  await expect(modal.getByText('No extensions to choose from yet')).toBeHidden()
  await expect(modal.getByText(/2 extensions from your repos/)).toBeVisible()
})

// Adding an extension source: the catalog already says which sources an
// extension provides, so one can be picked before anything is installed on
// the host. The install happens on Add, and the source's own settings —
// readable only from an installed extension — are offered right after.
test('a source is picked before its extension is installed', async ({ page, request }) => {
  const installed = async () => {
    const exts: Ext[] = await (await request.get('/api/v1/extensions')).json()
    return !!exts.find((e) => e.pkgName === LAZY)?.installedVersion
  }
  expect(await installed()).toBe(false)

  await page.goto('/indexers')
  await page.getByRole('button', { name: 'Add indexer' }).click()
  const modal = page.getByRole('dialog')
  await modal.getByText('Extension source').click()

  await modal.getByPlaceholder('Search extensions…').fill('Lazy')
  await page.getByRole('option', { name: /Lazy Source \(all\)/ }).click()

  // Both of its sources are on offer, and the host is still untouched.
  await modal.getByLabel('Source', { exact: true }).click()
  await expect(page.getByRole('option', { name: 'Lazy Source (en)' })).toBeVisible()
  await page.getByRole('option', { name: 'Lazy Source (fr)' }).click()
  await expect(modal.getByLabel('Name', { exact: true })).toHaveValue('Lazy Source (fr)')
  await expect(modal.getByText(/Adding installs Lazy Source/)).toBeVisible()
  expect(await installed()).toBe(false)

  await modal.getByRole('button', { name: 'Add', exact: true }).click()

  // Installed now, and the modal stays open on the source's settings.
  await expect(modal.getByText(/Lazy Source \(fr\) is added/)).toBeVisible()
  await expect(modal.getByLabel('Mirror', { exact: true })).toBeVisible()
  expect(await installed()).toBe(true)

  await modal.getByRole('button', { name: 'Close' }).click()
  await expect(modal).toBeHidden()
  const row = page.getByRole('row').filter({ hasText: 'Lazy Source (fr)' })
  await expect(row).toBeVisible()

  // Leave the shared instance as it was: deleting the last entry of an
  // extension uninstalls it.
  const indexers: { id: number; name: string }[] = await (await request.get('/api/v1/indexers')).json()
  const created = indexers.find((i) => i.name === 'Lazy Source (fr)')
  expect(created).toBeTruthy()
  expect((await request.delete(`/api/v1/indexers/${created!.id}`)).status()).toBe(204)
  expect(await installed()).toBe(false)
})

test('cancelling the add dialog installs nothing', async ({ page, request }) => {
  await page.goto('/indexers')
  await page.getByRole('button', { name: 'Add indexer' }).click()
  const modal = page.getByRole('dialog')
  await modal.getByText('Extension source').click()
  await modal.getByPlaceholder('Search extensions…').fill('Lazy')
  await page.getByRole('option', { name: /Lazy Source \(all\)/ }).click()
  await modal.getByRole('button', { name: 'Cancel' }).click()
  await expect(modal).toBeHidden()

  const exts: Ext[] = await (await request.get('/api/v1/extensions')).json()
  expect(exts.find((e) => e.pkgName === LAZY)?.installedVersion).toBeFalsy()
})
