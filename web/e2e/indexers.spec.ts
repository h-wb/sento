import { expect, test } from '@playwright/test'

const LAZY = 'eu.kanade.tachiyomi.extension.all.lazysource'

type Ext = { pkgName: string; installedVersion?: string; sources?: { id: string }[] }

test('test search distinguishes a working Books feed from unrelated results without queuing downloads', async ({ page, request }) => {
  const connections = await (await request.get('/api/v1/settings/connections')).json()
  const upstream = new URL('/torznab/api', connections.extensions.url).href
  const added = await request.post('/api/v1/indexers', { data: {
    name: 'Search diagnostic fixture', protocol: 'torznab', enabled: true, useVolumes: true, useChapters: false,
    priority: 0, settings: { url: upstream, apiKey: 'fixture-key' },
  } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  const queueIds = async () => ((await (await request.get('/api/v1/queue?all=1')).json()) as { id: number }[]).map((job) => job.id)
  const before = await queueIds()
  try {
    await page.goto('/indexers')
    await page.getByRole('button', { name: 'Test search on Search diagnostic fixture', exact: true }).click()
    const modal = page.getByRole('dialog', { name: 'Test search · Search diagnostic fixture' })
    await modal.getByRole('textbox', { name: 'Title', exact: true }).fill('Example')
    await modal.getByRole('button', { name: 'Test search', exact: true }).click()
    await expect(modal.getByText('Books results for “Example”: 3')).toBeVisible()
    await expect(modal.getByText('Example v01 (Digital) (Fixture)', { exact: true })).toBeVisible()
    await expect(modal.getByText('Comics (7030)').first()).toBeVisible()
    await modal.getByRole('textbox', { name: 'Title', exact: true }).fill('Category diagnostic')
    await modal.getByRole('button', { name: 'Test search', exact: true }).click()
    await expect(modal.getByText('Books results for “Category diagnostic”: 0')).toBeVisible()
    await expect(modal.getByText('Unfiltered results: 1')).toBeVisible()
    await expect(modal.getByText(/Check its category setting in Prowlarr or Jackett/)).toBeVisible()
    await expect(modal.getByText('Category diagnostic S01E01', { exact: true })).toBeVisible()
    await expect(modal.getByText('Anime (5070)', { exact: true })).toBeVisible()
    expect(await queueIds()).toEqual(before)
  } finally {
    await request.delete(`/api/v1/indexers/${id}`)
  }
})

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

  await modal.getByRole('button', { name: 'Close', exact: true }).click()
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

// The Manage extensions dialog shows installed vs available versions and
// runs explicit updates; the indexer row carries its extension's version.
test('manage extensions updates an installed extension', async ({ page, request }) => {
  // Add the Stub Source indexer through the UI so its extension is
  // installed (and desired — reconcile must not uninstall it).
  await page.goto('/indexers')
  await page.getByRole('button', { name: 'Add indexer' }).click()
  const modal = page.getByRole('dialog')
  await modal.getByText('Extension source').click()
  await modal.getByPlaceholder('Search extensions…').fill('Stub')
  await page.getByRole('option', { name: /Stub Source \(en\)/ }).click()
  await modal.getByRole('button', { name: 'Add', exact: true }).click()
  await modal.getByRole('button', { name: 'Close', exact: true }).click()
  await expect(modal).toBeHidden()

  // The row shows the extension's installed version and the update.
  const row = page.getByRole('row').filter({ hasText: 'Stub Source (en)' })
  await expect(row.getByText(/v1\.4\.0 — v1\.5\.0 available/)).toBeVisible()

  // Manage extensions: the update is listed, run it.
  await page.getByRole('button', { name: 'Manage extensions' }).click()
  const dlg = page.getByRole('dialog')
  await expect(dlg.getByText('v1.4.0 → v1.5.0', { exact: true })).toBeVisible()
  await dlg.getByRole('button', { name: 'Update all (1)' }).click()
  // Updated in place: no more update offered, the new version shows.
  await expect(dlg.getByRole('button', { name: 'Update all (1)' })).toBeHidden()
  await expect(dlg.getByText('v1.5.0', { exact: true })).toBeVisible()
  await dlg.getByRole('button', { name: 'Close', exact: true }).click()
  await expect(row.getByText(/v1\.5\.0/)).toBeVisible()
  await expect(row.getByText(/available/)).toBeHidden()

  // Leave the shared instance as it was: deleting the last entry of an
  // extension uninstalls it.
  const indexers: { id: number; name: string }[] = await (await request.get('/api/v1/indexers')).json()
  const created = indexers.find((i) => i.name === 'Stub Source (en)')
  expect(created).toBeTruthy()
  expect((await request.delete(`/api/v1/indexers/${created!.id}`)).status()).toBe(204)
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
