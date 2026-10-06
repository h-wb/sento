import { expect, test, type Page } from '@playwright/test'

// Every page reachable from the navbar, with the heading its PageHeader
// renders. A lazy route that fails to load shows the Suspense fallback
// forever, so an h2 assertion catches broken code-splitting too.
const PAGES = [
  { nav: 'Library', heading: 'Library', url: '/' },
  { nav: 'Calendar', heading: 'Calendar', url: '/calendar' },
  { nav: 'Activity', heading: 'Activity', url: '/queue' },
  { nav: 'Indexers', heading: 'Indexers', url: '/indexers' },
  { nav: 'Connections', heading: 'Connections', url: '/connections' },
  { nav: 'Profiles', heading: 'Profiles', url: '/profiles' },
  { nav: 'Media', heading: 'Media', url: '/media' },
  { nav: 'System', heading: 'System', url: '/system' },
]

// Console errors are treated as failures: a React render error or a failed
// fetch surfaces here long before it changes any visible text.
function collectConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text())
  })
  page.on('pageerror', (err) => errors.push(err.message))
  return errors
}

test('the shell renders and reports a healthy backend', async ({ page }) => {
  const errors = collectConsoleErrors(page)
  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'sento', level: 3 })).toBeVisible()
  // The navbar prints "· backend unreachable" next to the version when
  // /system/status fails, and the version once it succeeds.
  await expect(page.getByText(/^v/)).toBeVisible()
  await expect(page.getByText('backend unreachable')).toHaveCount(0)
  expect(errors).toEqual([])
})

test('every navbar destination loads', async ({ page }) => {
  const errors = collectConsoleErrors(page)
  await page.goto('/')

  for (const { nav, heading } of PAGES) {
    await page.getByRole('link', { name: nav, exact: true }).click()
    await expect(page.getByRole('heading', { name: heading, level: 2 })).toBeVisible()
  }
  expect(errors).toEqual([])
})

test('deep links render without going through the shell nav', async ({ page }) => {
  // The SPA fallback in mountStatic has to serve index.html for these.
  for (const { url, heading } of PAGES) {
    await page.goto(url)
    await expect(page.getByRole('heading', { name: heading, level: 2 })).toBeVisible()
  }
})

test('the command palette opens and navigates', async ({ page }) => {
  await page.goto('/')
  await page.keyboard.press('ControlOrMeta+k')

  const palette = page.getByRole('dialog')
  await expect(palette).toBeVisible()
  await palette.getByRole('textbox').fill('Activity')
  await palette.getByRole('button', { name: /Activity/ }).first().click()

  await expect(page.getByRole('heading', { name: 'Activity', level: 2 })).toBeVisible()
})

test('the g-key sequence jumps to a page', async ({ page }) => {
  await page.goto('/')
  await page.keyboard.press('g')
  await page.keyboard.press('o')
  await expect(page.getByRole('heading', { name: 'Connections', level: 2 })).toBeVisible()
})

test('the theme menu switches presets', async ({ page }) => {
  await page.goto('/')
  const html = page.locator('html')
  const before = await html.getAttribute('data-mantine-color-scheme')

  await page.getByRole('button', { name: /theme|appearance/i }).first().click()
  await page.getByRole('menuitem', { name: before === 'dark' ? 'Light' : 'Dark' }).click()

  await expect(html).not.toHaveAttribute('data-mantine-color-scheme', before ?? '')
})
