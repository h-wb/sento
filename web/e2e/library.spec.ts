import { expect, test } from '@playwright/test'
import type { Series } from '../src/types/store'

const series: Series[] = ['Witch Hat Atelier', 'Blue Period', 'Vinland Saga', 'Dorohedoro', 'Dungeon Meshi', 'Yotsuba'].map((title, i) => ({
  id: i + 1, mangabakaId: i + 1, title, sortTitle: title, type: 'manga',
  status: i % 2 ? 'completed' : 'ongoing', monitored: i !== 3,
  finalVolume: i + 8, totalChapters: 70, year: 2015 + i, addedAt: '2026-01-01T00:00:00Z',
}))

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/series/', (route) => route.fulfill({ json: series }))
})

test('library filters combine, reset, and carry into table view', async ({ page }) => {
  await page.goto('/')
  const posters = page.locator('main a[href^="/series/"]')
  await expect(posters).toHaveCount(6)
  await page.getByRole('button', { name: /^Unmonitored/ }).click()
  await expect(posters).toHaveCount(1)
  await expect(posters.first()).toContainText('Dorohedoro')
  await page.getByRole('textbox', { name: 'Filter by title' }).fill('Blue')
  await expect(page.getByText('No series match')).toBeVisible()
  await page.getByRole('button', { name: 'Clear filters' }).click()
  await expect(posters).toHaveCount(6)
  await page.getByRole('button', { name: /^Monitored/ }).click()
  await page.getByRole('button', { name: 'Table', exact: true }).click()
  await expect(page.getByRole('checkbox', { name: /^Select / })).toHaveCount(6)
  await page.reload()
  await expect(page.getByRole('checkbox', { name: 'Select all' })).toBeVisible()
})

test('library fits desktop and mobile in both color schemes', async ({ page }, testInfo) => {
  await page.goto('/')
  await expect(page.locator('main a[href^="/series/"]')).toHaveCount(6)
  for (const mode of ['dark', 'light'] as const) {
    await page.getByRole('button', { name: 'Appearance' }).click()
    await page.getByRole('menuitem', { name: mode === 'dark' ? 'Dark' : 'Light', exact: true }).click()
    await page.keyboard.press('Escape')
    await expect(page.locator('html')).toHaveCSS('color-scheme', mode)
    await expect(page.locator('header')).toHaveCSS('background-color', mode === 'dark' ? 'rgb(29, 29, 27)' : 'rgb(255, 255, 255)')
    await page.screenshot({ path: testInfo.outputPath(`library-${mode}.png`), fullPage: true })
  }
  await page.setViewportSize({ width: 390, height: 844 })
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await expect(page.getByRole('textbox', { name: 'Filter by title' })).toBeVisible()
  await page.getByRole('button', { name: 'Toggle navigation' }).click()
  await page.getByRole('link', { name: 'Calendar', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Calendar', level: 2 })).toBeVisible()
  await page.goto('/')
  await page.screenshot({ path: testInfo.outputPath('library-mobile.png'), fullPage: true })
})
