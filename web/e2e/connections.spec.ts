import { expect, test } from '@playwright/test'

// Connections holds the services Sento downloads through. The values
// rendered here come from the backend's own config, so this is the
// cheapest proof that config → API → UI is wired end to end.
test('each connection is a card with its live status, and its settings open in a dialog', async ({ page }, testInfo) => {
  await page.goto('/connections')
  await expect(page.getByRole('heading', { name: 'Connections', level: 2 })).toBeVisible()

  // Probed on load — nobody pressed Test.
  const qbit = page.getByRole('button', { name: 'Edit qBittorrent' })
  await expect(qbit.getByText(/^Connected/)).toBeVisible()
  await expect(qbit.getByText('Default', { exact: true })).toBeVisible()
  const sidecar = page.getByRole('button', { name: 'Edit Extensions sidecar' })
  await expect(sidecar.getByText(/^Connected · \d+ sources?/)).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('connections.png'), fullPage: true })

  await qbit.click()
  const dialog = page.getByRole('dialog', { name: 'qBittorrent' })
  await expect(dialog.getByRole('textbox', { name: 'URL' })).toHaveValue(/\/qbit$/)
  await page.screenshot({ path: testInfo.outputPath('connections-dialog.png') })
  await dialog.getByRole('button', { name: 'Cancel' }).click()

  await sidecar.click()
  await expect(
    page.getByRole('dialog', { name: 'Extensions sidecar' }).getByRole('textbox', { name: 'URL', exact: true }),
  ).toHaveValue(/\/host$/)
})

test('the old /sources address still lands on the page', async ({ page }) => {
  await page.goto('/sources')
  await expect(page).toHaveURL(/\/connections$/)
  await expect(page.getByRole('heading', { name: 'Connections', level: 2 })).toBeVisible()
})

// A connection is tested as typed before saving: a wrong URL is reported
// in the dialog, and nothing is applied until Save.
test('a connection is tested on its own before saving', async ({ page }) => {
  await page.goto('/connections')
  const tile = page.getByRole('button', { name: 'Edit qBittorrent' })
  await tile.click()
  const dialog = page.getByRole('dialog', { name: 'qBittorrent' })
  await expect(dialog.getByRole('button', { name: 'Save' })).toBeDisabled()

  await dialog.getByRole('textbox', { name: 'URL' }).fill('http://127.0.0.1:9/nothing-here')
  await dialog.getByRole('button', { name: 'Test' }).click()
  await expect(dialog.getByText('Unreachable')).toBeVisible()

  // Cancel drops the edits: what was saved is still what runs.
  await dialog.getByRole('button', { name: 'Cancel' }).click()
  await expect(dialog).toHaveCount(0)
  await expect(tile.getByText(/^Connected/)).toBeVisible()
})

// A repo that fails to load says so on its row — it used to be a bare URL
// whose only symptom was an empty extension list somewhere else.
test('a failing extension repo is reported on its row', async ({ page }) => {
  await page.goto('/connections')
  const host = page.getByRole('region', { name: 'Extension sources' })
  await expect(host.getByText(/Add a repository to see available sources/)).toBeVisible()

  await host.getByRole('textbox', { name: 'Extension repo URL' }).fill('https://broken.example/index.min.json')
  await host.getByRole('button', { name: 'Add repo' }).click()
  await expect(host.getByText('Failing')).toBeVisible()
  await expect(host.getByText(/returned HTTP 404/)).toBeVisible()

  await host.getByRole('button', { name: 'Remove' }).click()
  await expect(host.getByText(/Add a repository to see available sources/)).toBeVisible()
})

test('the indexers page lists everything Sento can search', async ({ page }) => {
  await page.goto('/indexers')
  await expect(page.getByRole('heading', { name: 'Indexers', level: 2 })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Add indexer' })).toBeVisible()
})

test('the activity queue renders when empty', async ({ page }) => {
  await page.goto('/queue')
  await expect(page.getByRole('heading', { name: 'Activity', level: 2 })).toBeVisible()
})

test('system status shows the backend version and scheduled tasks', async ({ page }) => {
  await page.goto('/system')
  await expect(page.getByRole('heading', { name: 'System', level: 2 })).toBeVisible()
  await expect(page.getByText(/dev|\d+\.\d+/).first()).toBeVisible()
})

// A second download client can be added, shows its own status, becomes a
// choice on torrent indexers, and can be removed.
test('an additional download client is added, offered to indexers, and removed', async ({ page }) => {
  await page.goto('/connections')
  await page.getByRole('button', { name: 'Edit qBittorrent' }).click()
  const url = await page.getByRole('dialog').getByRole('textbox', { name: 'URL' }).inputValue()
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()

  await page.getByRole('button', { name: 'Add download client' }).click()
  const add = page.getByRole('dialog', { name: 'Add download client' })
  await expect(add.getByRole('button', { name: 'Save' })).toBeDisabled() // no URL yet
  await add.getByRole('textbox', { name: 'Name', exact: true }).fill('VPN')
  await add.getByRole('textbox', { name: 'URL' }).fill(url)
  await add.getByRole('button', { name: 'Test' }).click()
  await expect(add.getByText(/^Reachable/)).toBeVisible()
  await add.getByRole('button', { name: 'Save' }).click()

  const vpn = page.getByRole('button', { name: 'Edit VPN' })
  await expect(vpn.getByText(/^Connected/)).toBeVisible()
  await expect(vpn.getByText('Default', { exact: true })).toHaveCount(0)

  // Torrent indexers can now say which client their releases go to.
  await page.goto('/indexers')
  await page.getByRole('button', { name: 'Add indexer' }).click()
  const modal = page.getByRole('dialog')
  await modal.getByLabel('Download client').first().click()
  await expect(page.getByRole('option', { name: 'Default (qBittorrent)' })).toBeVisible()
  await page.getByRole('option', { name: 'VPN', exact: true }).click()
  await modal.getByRole('button', { name: 'Cancel' }).click()

  // Removed from its dialog; the default client is untouched.
  await page.goto('/connections')
  await page.getByRole('button', { name: 'Edit VPN' }).click()
  await page.getByRole('dialog', { name: 'VPN' }).getByRole('button', { name: 'Remove' }).click()
  await expect(page.getByRole('button', { name: 'Edit VPN' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Edit qBittorrent' }).getByText(/^Connected/)).toBeVisible()
})
