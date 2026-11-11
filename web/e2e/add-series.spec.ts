import { expect, test } from '@playwright/test'

// The core flow, end to end through the real Go stack: MangaBaka lookup →
// add → series detail → library listing. e2estub serves fixed fixtures, so
// the titles below are stable.
test.describe.configure({ mode: 'serial' })

test('search, add, and open a series', async ({ page }) => {
  await page.goto('/add')
  await expect(page.getByRole('heading', { name: 'Add series', level: 2 })).toBeVisible()

  await page.getByPlaceholder('Search MangaBaka…').fill('Stub Blade')
  await page.getByRole('button', { name: 'Search', exact: true }).click()

  // Two fixtures match "Stub Blade"; the third must not appear.
  await expect(page.getByText('Stub Blade', { exact: true })).toBeVisible()
  await expect(page.getByText('Stub Blade: Afterstory')).toBeVisible()
  await expect(page.getByText('Deterministic Diaries')).toHaveCount(0)

  const card = page.locator('.mantine-Card-root').filter({ hasText: 'Stub Blade: Afterstory' })
  await card.getByRole('button', { name: /^Add/ }).click()

  // addSeries navigates to /series/:id on success.
  await page.waitForURL(/\/series\/\d+$/)
  await expect(page.getByRole('heading', { name: 'Stub Blade: Afterstory' })).toBeVisible()
})

test('the added series appears in the library', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Library', level: 2 })).toBeVisible()
  await expect(page.getByText('Stub Blade: Afterstory')).toBeVisible()
})

test('the added series is searchable from the command palette', async ({ page }) => {
  await page.goto('/')
  await page.keyboard.press('ControlOrMeta+k')

  const palette = page.getByRole('dialog')
  await palette.getByRole('textbox').fill('Afterstory')
  await palette.getByRole('button', { name: /Afterstory/ }).first().click()

  await page.waitForURL(/\/series\/\d+$/)
  await expect(page.getByRole('heading', { name: 'Stub Blade: Afterstory' })).toBeVisible()
})

test('re-adding is offered as opening the existing series', async ({ page }) => {
  await page.goto('/add')
  await page.getByPlaceholder('Search MangaBaka…').fill('Stub Blade')
  await page.getByRole('button', { name: 'Search', exact: true }).click()

  const card = page.locator('.mantine-Card-root').filter({ hasText: 'Stub Blade: Afterstory' })
  await expect(card.getByRole('button', { name: /^Add/ })).toHaveCount(0)
})
