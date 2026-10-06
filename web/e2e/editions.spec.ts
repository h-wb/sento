import { expect, test } from '@playwright/test'

test('editions retain independent versions, editable units, and mobile layout', async ({
  page,
  request,
}, testInfo) => {
  const added = await request.post('/api/v1/series/', {
    data: { mangabakaId: 103, monitor: 'all' },
  })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  try {
    await page.goto(`/series/${id}`)
    await expect(page.getByRole('tab', { name: 'Editions', exact: true })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    const edition = page
      .locator('.mantine-Card-root')
      .filter({ hasText: 'Standard Edition' })
      .first()
    await edition.getByRole('button', { name: /More actions/ }).click()
    await page.getByRole('menuitem', { name: 'Keep an additional copy', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Keep an additional copy', exact: true })
    await dialog.getByRole('textbox', { name: 'Version name', exact: true }).fill('Preferred group')
    await dialog.getByRole('textbox', { name: 'Required release group', exact: true }).fill('LuCaZ')
    await dialog.getByRole('button', { name: 'Save version', exact: true }).click()
    await expect(dialog).toBeHidden()
    const targets = await (await request.get(`/api/v1/series/${id}/targets`)).json()
    expect(targets.filter((t: { name: string }) => t.name === 'Preferred group')).toHaveLength(1)
    await edition.getByRole('button', { name: /More actions/ }).click()
    await page.getByRole('menuitem', { name: 'Add volume', exact: true }).click()
    const unit = page.getByRole('dialog', { name: 'Add book', exact: true })
    await unit.getByRole('textbox', { name: 'Display label', exact: true }).fill('0')
    await unit.getByRole('button', { name: 'Save book', exact: true }).click()
    await expect(unit).toBeHidden()
    await expect(edition.getByRole('button', { name: /Hide volumes/ })).toBeVisible()
    await expect(edition.getByRole('button', { name: '0', exact: true })).toBeVisible()
    await edition.getByRole('button', { name: '0', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Volume 0', exact: true })).toBeVisible()
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Close dialog', exact: true })
      .click()
    await edition.getByRole('button', { name: /More actions/ }).click()
    await page.getByRole('menuitem', { name: 'Edit edition', exact: true }).click()
    const edit = page.getByRole('dialog', { name: 'Edit edition', exact: true })
    await edit
      .getByRole('textbox', { name: 'Edition name', exact: true })
      .fill('Publisher corrected edition')
    await edit.getByRole('button', { name: 'Save edition', exact: true }).click()
    await expect(edit).toBeHidden()
    await page.reload()
    await expect(page.getByText(/Publisher corrected edition/).first()).toBeVisible()
    await expect(page.getByRole('button', { name: 'Collections', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Add edition', exact: true }).click()
    const add = page.getByRole('dialog', { name: 'Add edition', exact: true })
    await add.getByRole('textbox', { name: 'Edition name', exact: true }).fill('Spanish edition')
    await add.getByRole('button', { name: 'Close dialog', exact: true }).click()
    const discard = page.getByRole('dialog', { name: 'Discard unsaved changes?', exact: true })
    await expect(discard).toBeVisible()
    await discard.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(add.getByRole('textbox', { name: 'Edition name', exact: true })).toHaveValue(
      'Spanish edition',
    )
    await add.getByRole('textbox', { name: 'Language', exact: true }).fill('es')
    await add.getByRole('button', { name: 'Add edition', exact: true }).click()
    await expect(add).toBeHidden()
    await expect(page.getByText('Spanish edition', { exact: true })).toBeVisible()
    const added = await request.get(`/api/v1/series/${id}/`)
    const detail = await added.json()
    const spanish = detail.collections.find(
      (c: { editionName: string }) => c.editionName === 'Spanish edition',
    )
    expect(spanish.custom).toBe(true)
    const freshSlots = await (await request.get(`/api/v1/series/${id}/target-slots`)).json()
    expect(
      Object.values(freshSlots)
        .flat()
        .filter(
          (s: unknown) =>
            (s as { work: { collectionId: number } }).work.collectionId === spanish.id,
        ),
    ).toHaveLength(0)
    const chapters = detail.collections.find((c: { type: string }) => c.type === 'chapter')
    for (const number of [7050, 7100]) {
      const response = await request.post(`/api/v1/series/${id}/observations`, {
        data: {
          collectionId: chapters.id,
          source: 'Verified reader',
          externalKey: String(number),
          label: String(number / 100),
          number,
        },
      })
      expect(response.ok()).toBeTruthy()
    }
    await page.getByRole('tab', { name: 'Review', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Add chapter', exact: true })).toHaveCount(2)
    await page.getByRole('button', { name: 'Accept new chapters (2)', exact: true }).click()
    await expect(page.getByText('Nothing needs review.', { exact: true })).toBeVisible()
    const reviewed = await (await request.get(`/api/v1/series/${id}/target-slots`)).json()
    expect(
      Object.values(reviewed)
        .flat()
        .some(
          (book: unknown) => (book as { work: { sequenceNum: number } }).work.sequenceNum === 7050,
        ),
    ).toBe(true)
    await page.getByRole('tab', { name: 'Editions', exact: true }).click()
    await page.setViewportSize({ width: 390, height: 844 })
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
      .toBe(true)
    await page.screenshot({
      path: testInfo.outputPath('editions-mobile.png'),
      fullPage: true,
      animations: 'disabled',
    })
  } finally {
    await request.delete(`/api/v1/series/${id}`).catch(() => {})
  }
})

test('a long chapter edition is browsed as a grid with filters, ranges and jump', async ({
  page,
  request,
}, testInfo) => {
  const added = await request.post('/api/v1/series/', {
    data: { mangabakaId: 103, monitor: 'all' },
  })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  try {
    await page.goto(`/series/${id}`)
    await expect(page.getByRole('button', { name: /Hide chapters/ })).toBeVisible()
    // One tab row only: no nested section switcher above the editions.
    await expect(page.getByText('Your editions', { exact: true })).toHaveCount(0)
    await expect(page.getByRole('tab')).toHaveText(['Editions', /^Files/, 'Review', 'Indexers', 'History'])
    const chapters = page.locator('.mantine-Card-root').filter({ hasText: 'Local edition' })
    const layout = chapters.getByRole('radiogroup', { name: 'Book layout' })
    await expect(layout.getByRole('radio', { name: 'Grid' })).toBeChecked()
    const cells = chapters.locator('[data-book]')
    await expect(cells).toHaveCount(63)
    await expect(chapters.getByText('Paused 63', { exact: true })).toBeVisible()

    // A number range becomes a selection, and one action applies to all of it.
    await chapters.getByRole('button', { name: 'Select range', exact: true }).click()
    await page.getByRole('textbox', { name: 'From', exact: true }).fill('10')
    await page.getByRole('textbox', { name: 'To', exact: true }).fill('20')
    await page.getByRole('textbox', { name: 'To', exact: true }).press('Enter')
    await expect(chapters.getByText('11 selected', { exact: true })).toBeVisible()
    await chapters.getByRole('button', { name: 'Collect', exact: true }).click()
    await expect(chapters.getByText('Paused 52', { exact: true })).toBeVisible()
    await expect(chapters.getByText('11 selected', { exact: true })).toHaveCount(0)

    // Shift-click extends a selection without opening the book.
    await cells.nth(0).click({ modifiers: ['ControlOrMeta'] })
    await cells.nth(4).click({ modifiers: ['Shift'] })
    await expect(chapters.getByText('5 selected', { exact: true })).toBeVisible()
    await chapters.getByRole('button', { name: 'Clear', exact: true }).click()

    // A status count filters the grid.
    await chapters.getByText('Paused 52', { exact: true }).click()
    await expect(cells).toHaveCount(52)
    await chapters.getByText('All 63', { exact: true }).click()

    await chapters.getByRole('textbox', { name: 'Go to chapter number' }).fill('40')
    await chapters.getByRole('textbox', { name: 'Go to chapter number' }).press('Enter')
    await expect(chapters.locator('[data-flash="true"]')).toHaveText('40')
    await page.screenshot({ path: testInfo.outputPath('chapters-grid.png'), fullPage: true })

    // The list shows the same books one per line and still opens a book.
    await layout.getByText('List', { exact: true }).click()
    await expect(chapters.getByRole('checkbox', { name: 'Collect chapter 12', exact: true })).toBeChecked()
    await page.screenshot({ path: testInfo.outputPath('chapters-list.png'), fullPage: true })
    await chapters.getByRole('button', { name: '12', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Chapter 12', exact: true })).toBeVisible()
  } finally {
    await request.delete(`/api/v1/series/${id}/`)
  }
})

test('organize files previews renames and metadata updates before applying', async ({
  page,
  request,
}, testInfo) => {
  const added = await request.post('/api/v1/series/', {
    data: { mangabakaId: 103, monitor: 'all' },
  })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  try {
    const file = {
      id: 1,
      seriesId: id,
      path: '/library/Series/collections/1/versions/1/Series v01 [abc].cbz',
      size: 1024,
      provenance: 'torrent',
      presence: 'present',
      facts: {},
      claims: {},
    }
    const posted: unknown[] = []
    await page.route(`**/api/v1/series/${id}/files`, (route) => route.fulfill({ json: [file] }))
    await page.route(`**/api/v1/series/${id}/files/organize`, (route) => {
      if (route.request().method() === 'POST') {
        posted.push(route.request().postDataJSON())
        return route.fulfill({ json: { renamed: 1, retagged: 0, errors: [] } })
      }
      return route.fulfill({
        json: [
          {
            fileId: 1,
            from: 'collections/1/versions/1/Series v01 [abc].cbz',
            to: 'Standard Edition [en]/Series v01.cbz',
            rename: true,
            retag: true,
          },
          { fileId: 2, from: 'review/unknown.cbz', to: 'review/unknown.cbz', rename: false, retag: false, skip: 'Not matched to a book yet.' },
        ],
      })
    })
    await page.goto(`/series/${id}`)
    await page.getByRole('tab', { name: /^Files/ }).click()
    await page.getByRole('button', { name: 'Organize files', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Organize files', exact: true })
    await expect(dialog.getByText('→ Standard Edition [en]/Series v01.cbz')).toBeVisible()
    await expect(dialog.getByText(/1 file is left alone/)).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath('organize.png') })

    // Unticking metadata applies the move only.
    await dialog.getByRole('checkbox', { name: 'Update embedded metadata (1)' }).uncheck()
    await dialog.getByRole('button', { name: 'Apply to 1 file', exact: true }).click()
    await expect(dialog).toHaveCount(0)
    expect(posted).toEqual([{ rename: true, retag: false }])
  } finally {
    await request.delete(`/api/v1/series/${id}/`)
  }
})
