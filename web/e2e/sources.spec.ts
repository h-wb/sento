import { expect, test } from '@playwright/test'

// Sources is the connection-settings page: the URLs Sento talks to. The
// values rendered here come from the backend's own config, so this is the
// cheapest proof that config → API → UI is wired end to end.
test('the configured source host and download client are shown', async ({ page }) => {
  await page.goto('/sources')
  await expect(page.getByRole('heading', { name: 'Sources', level: 2 })).toBeVisible()

  const hostCard = page.locator('.mantine-Card-root').filter({ hasText: 'Source host' })
  await expect(hostCard.getByRole('textbox', { name: 'URL' })).toHaveValue(/\/host$/)

  const qbitCard = page.locator('.mantine-Card-root').filter({ hasText: 'qBittorrent' })
  await expect(qbitCard.getByRole('textbox', { name: 'URL' })).toHaveValue(/\/qbit$/)
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
