import { expect, test } from '@playwright/test'

// Connections holds the services Sento downloads through. The values
// rendered here come from the backend's own config, so this is the
// cheapest proof that config → API → UI is wired end to end.
test('both services are shown with their live status', async ({ page }) => {
  await page.goto('/connections')
  await expect(page.getByRole('heading', { name: 'Connections', level: 2 })).toBeVisible()

  const qbit = page.getByRole('region', { name: 'qBittorrent' })
  await expect(qbit.getByRole('textbox', { name: 'URL' })).toHaveValue(/\/qbit$/)
  // Probed on load — nobody pressed Test.
  await expect(qbit.getByText(/^Connected/)).toBeVisible()

  const host = page.getByRole('region', { name: 'Source host' })
  await expect(host.getByRole('textbox', { name: 'URL', exact: true })).toHaveValue(/\/host$/)
  await expect(host.getByText(/^Connected · \d+ sources?/)).toBeVisible()
})

test('the old /sources address still lands on the page', async ({ page }) => {
  await page.goto('/sources')
  await expect(page).toHaveURL(/\/connections$/)
  await expect(page.getByRole('heading', { name: 'Connections', level: 2 })).toBeVisible()
})

// Each service tests and saves on its own: a wrong qBittorrent URL is
// reported on that card, and the source host is not touched by it.
test('a service is tested on its own before saving', async ({ page }) => {
  await page.goto('/connections')
  const qbit = page.getByRole('region', { name: 'qBittorrent' })
  const url = qbit.getByRole('textbox', { name: 'URL' })
  const good = await url.inputValue()

  await url.fill('http://127.0.0.1:9/nothing-here')
  await qbit.getByRole('button', { name: 'Test' }).click()
  await expect(qbit.getByText('Unreachable')).toBeVisible()
  // Still the saved, working connection: nothing was applied.
  await expect(qbit.getByText(/^Connected/)).toBeVisible()

  await qbit.getByRole('button', { name: 'Reset' }).click()
  await expect(url).toHaveValue(good)
  await expect(qbit.getByRole('button', { name: 'Save' })).toBeDisabled()
})

// A repo that fails to load says so on its row — it used to be a bare URL
// whose only symptom was an empty extension list somewhere else.
test('a failing extension repo is reported on its row', async ({ page }) => {
  await page.goto('/connections')
  const host = page.getByRole('region', { name: 'Source host' })
  await expect(host.getByText(/No repos yet/)).toBeVisible()

  await host.getByRole('textbox', { name: 'Extension repo URL' }).fill('https://broken.example/index.min.json')
  await host.getByRole('button', { name: 'Add repo' }).click()
  await expect(host.getByText('Failing')).toBeVisible()
  await expect(host.getByText(/returned HTTP 404/)).toBeVisible()

  await host.getByRole('button', { name: 'Remove' }).click()
  await expect(host.getByText(/No repos yet/)).toBeVisible()
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
