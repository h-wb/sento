import { expect, test } from '@playwright/test'

test('editions retain independent versions, editable books, and mobile layout', async ({
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
    await page.getByRole('menuitem', { name: 'More', exact: true }).click()
    await page.getByRole('menuitem', { name: 'Keep another copy of this edition', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Keep another copy of this edition', exact: true })
    await dialog.getByRole('textbox', { name: 'Version name', exact: true }).fill('Preferred group')
    await dialog.getByRole('textbox', { name: 'Required release group', exact: true }).fill('LuCaZ')
    await dialog.getByRole('button', { name: 'Save version', exact: true }).click()
    await expect(dialog).toBeHidden()
    const versions = await (await request.get(`/api/v1/series/${id}/versions`)).json()
    expect(versions.filter((t: { name: string }) => t.name === 'Preferred group')).toHaveLength(1)
    await edition.getByRole('button', { name: /More actions/ }).click()
    await page.getByRole('menuitem', { name: 'Add volume', exact: true }).click()
    const bookDialog = page.getByRole('dialog', { name: 'Add book', exact: true })
    await bookDialog.getByRole('textbox', { name: 'Display label', exact: true }).fill('0')
    await bookDialog.getByRole('button', { name: 'Save book', exact: true }).click()
    await expect(bookDialog).toBeHidden()
    await expect(edition.getByRole('button', { name: /Hide volumes/ })).toBeVisible()
    await expect(edition.getByRole('button', { name: '0', exact: true })).toBeVisible()
    await edition.getByRole('button', { name: '0', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Volume 0', exact: true })).toBeVisible()
    // A search from a book names that book of that collected version.
    const [searched] = await Promise.all([
      page.waitForRequest((r) => r.method() === 'POST' && r.url().endsWith(`/series/${id}/search`)),
      page.getByRole('dialog', { name: 'Volume 0', exact: true }).getByRole('button', { name: /Search releases/ }).click(),
    ])
    const scope = searched.postDataJSON().scope
    expect(scope.versionId).toBeGreaterThan(0)
    expect(scope.bookId).toBeGreaterThan(0)
    expect(scope.all).toBeFalsy()
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
    await expect(page.getByRole('button', { name: 'Editions', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Add edition', exact: true }).click()
    const add = page.getByRole('dialog', { name: 'Add edition', exact: true })
    await add.getByRole('combobox', { name: 'Edition name', exact: true }).fill('Spanish edition')
    await add.getByRole('button', { name: 'Close dialog', exact: true }).click()
    const discard = page.getByRole('dialog', { name: 'Discard unsaved changes?', exact: true })
    await expect(discard).toBeVisible()
    await discard.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(add.getByRole('combobox', { name: 'Edition name', exact: true })).toHaveValue(
      'Spanish edition',
    )
    // Escape asks too, and Escape again returns to the edits.
    await add.getByRole('textbox', { name: 'Volumes', exact: true }).press('Escape')
    await expect(discard).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(discard).toBeHidden()
    await expect(add).toBeVisible()
    await add.getByRole('combobox', { name: 'Language', exact: true }).fill('es')
    // The count MangaBaka gives is offered, not imposed.
    await expect(add.getByRole('textbox', { name: 'Volumes', exact: true })).toHaveValue('7')
    await add.getByRole('textbox', { name: 'Volumes', exact: true }).fill('0')
    await add.getByRole('button', { name: 'Add edition', exact: true }).click()
    await expect(add).toBeHidden()
    await expect(page.getByText('Spanish edition', { exact: true })).toBeVisible()
    const added = await request.get(`/api/v1/series/${id}/`)
    const detail = await added.json()
    const spanish = detail.editions.find(
      (c: { editionName: string }) => c.editionName === 'Spanish edition',
    )
    expect(spanish.custom).toBe(true)
    const freshSlots = await (await request.get(`/api/v1/series/${id}/books`)).json()
    expect(
      Object.values(freshSlots)
        .flat()
        .filter(
          (s: unknown) =>
            (s as { book: { editionId: number } }).book.editionId === spanish.id,
        ),
    ).toHaveLength(0)
    const chapters = detail.editions.find((c: { type: string }) => c.type === 'chapter')
    for (const number of [7050, 7100]) {
      const response = await request.post(`/api/v1/series/${id}/observations`, {
        data: {
          editionId: chapters.id,
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
    const reviewed = await (await request.get(`/api/v1/series/${id}/books`)).json()
    expect(
      Object.values(reviewed)
        .flat()
        .some(
          (book: unknown) => (book as { book: { sequenceNum: number } }).book.sequenceNum === 7050,
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
    await expect(page.getByRole('tab')).toHaveText(['Editions', /^Files/, 'Review', 'Sources', 'History'])
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
      path: '/library/Series/editions/1/versions/1/Series v01 [abc].cbz',
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
            from: 'editions/1/versions/1/Series v01 [abc].cbz',
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

// MangaBaka lists the digital and the paperback run as two collections of
// one edition. They are collected as one edition, and either can be taken
// out of it and put back.
test('an edition groups its catalog collections, which can be separated and merged back', async ({
  page,
  request,
}, testInfo) => {
  const added = await request.post('/api/v1/series/', {
    data: { mangabakaId: 103, monitor: 'all' },
  })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  const standard = async () =>
    (
      (await (await request.get(`/api/v1/series/${id}/`)).json()) as {
        editions: {
          id: number
          editionName: string
          type: string
          collections: { medium: string }[]
        }[]
      }
    ).editions.filter((e) => e.editionName === 'Standard Edition' && e.type === 'volume')
  try {
    let editions = await standard()
    expect(editions).toHaveLength(1)
    expect(editions[0].collections.map((c) => c.medium)).toEqual(['digital', 'paperback'])

    await page.goto(`/series/${id}`)
    const card = page.locator('.mantine-Card-root').filter({ hasText: 'Standard Edition' }).first()
    await expect(card).toContainText('en · Stub Publishing · digital + paperback')
    await page.screenshot({
      path: testInfo.outputPath('edition-grouped.png'),
      animations: 'disabled',
    })

    await card.getByRole('button', { name: /More actions/ }).click()
    await page.getByRole('menuitem', { name: 'More', exact: true }).click()
    await page
      .getByRole('menuitem', {
        name: 'paperback · Stub Publishing',
        exact: true,
      })
      .click()
    const confirm = page.getByRole('dialog', {
      name: 'Separate the paperback collection?',
    })
    await confirm.getByRole('button', { name: 'Separate', exact: true }).click()
    await expect(confirm).toBeHidden()
    await expect.poll(async () => (await standard()).length).toBe(2)
    await expect(card).toContainText('en · Stub Publishing · digital')
    await expect(card).not.toContainText('paperback')

    // The paperback is now an edition nobody collects, offered with the rest.
    await page.getByRole('button', { name: /Other editions \(1\)/ }).click()
    const other = page.locator('.mantine-Card-root').filter({ hasText: 'paperback' }).first()
    await expect(other).toContainText('Standard Edition')

    editions = await standard()
    const paper = editions.find((e) => e.collections[0].medium === 'paperback')!
    const digital = editions.find((e) => e.collections[0].medium === 'digital')!
    const merged = await request.post(`/api/v1/series/${id}/editions/${paper.id}/merge`, {
      data: { into: digital.id },
    })
    expect(merged.ok()).toBeTruthy()
    editions = await standard()
    expect(editions).toHaveLength(1)
    expect(editions[0].collections).toHaveLength(2)
  } finally {
    await request.delete(`/api/v1/series/${id}`).catch(() => {})
  }
})

// MangaBaka knows some series without listing their volumes. The page says
// so, and the edition added by hand is pre-filled from what MangaBaka's
// series record does give, each value saying where it comes from.
test('a series MangaBaka lists no volumes for explains itself and pre-fills a local edition', async ({
  page,
  request,
}, testInfo) => {
  const added = await request.post('/api/v1/series/', { data: { mangabakaId: 104 } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  try {
    await page.goto(`/series/${id}`)
    const notice = page.getByRole('alert').filter({ hasText: 'MangaBaka lists no volumes' })
    await expect(notice).toContainText('a count for the original run: 6 volumes')
    await notice.getByRole('button', { name: 'Add a volume edition', exact: true }).click()

    const add = page.getByRole('dialog', { name: 'Add edition', exact: true })
    await expect(add).toContainText('MangaBaka lists no editions for this series yet')
    await expect(add.getByRole('textbox', { name: 'Volumes', exact: true })).toHaveValue('6')
    await expect(add).toContainText('The original run has 6')
    // The disabled button's reason is spelled out.
    await expect(add).toContainText('Enter a language to add this edition.')
    await expect(add.getByRole('button', { name: 'Add edition', exact: true })).toBeDisabled()
    await add.getByRole('combobox', { name: 'Language', exact: true }).fill('fr')
    // Publisher and medium stay out of the way; MangaBaka's publisher shows on the toggle.
    await add.getByRole('button', { name: 'Publisher and medium: Éditions Stub' }).click()
    await expect(add.getByRole('combobox', { name: 'Publisher', exact: true })).toHaveValue(
      'Éditions Stub',
    )
    await expect(add).toContainText('Its fr publisher')
    await expect(add).toContainText(
      'Adds Standard Edition · fr · Éditions Stub as a local edition with volumes 1 to 6',
    )
    await add.screenshot({ path: testInfo.outputPath('add-edition.png'), animations: 'disabled' })
    await add.getByRole('button', { name: 'Add edition', exact: true }).click()
    await expect(add).toBeHidden()

    await expect(notice).toHaveCount(0)
    const card = page.locator('.mantine-Card-root').filter({ hasText: 'Standard Edition' }).first()
    await expect(card).toContainText('Local edition')
    await expect(card).toContainText('fr · Éditions Stub')
    await expect(card).toContainText('6 volumes')
    const detail = await (await request.get(`/api/v1/series/${id}/`)).json()
    const local = detail.editions.find((e: { type: string }) => e.type === 'volume')
    // Named from MangaBaka's list, so the link is a key match later.
    expect(local).toMatchObject({ custom: true, providerEditionId: 'edition-standard' })
  } finally {
    await request.delete(`/api/v1/series/${id}`).catch(() => {})
  }
})

// A book of a catalog edition shows the copies MangaBaka lists behind it,
// and an edit is marked as one.
test('a book shows its MangaBaka copies and marks what was edited', async ({
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
    const card = page.locator('.mantine-Card-root').filter({ hasText: 'Standard Edition' }).first()
    await expect(card.getByText('MangaBaka', { exact: true })).toBeVisible()
    await card.getByRole('radio', { name: 'List' }).check({ force: true })
    const row = card.locator('[data-book]').first()
    await row.getByRole('button', { name: /More|actions/i }).click()
    await page.getByRole('menuitem', { name: 'Edit', exact: true }).click()
    const edit = page.getByRole('dialog', { name: 'Edit book', exact: true })
    const copies = edit.getByLabel('Copies listed in MangaBaka')
    await expect(copies).toContainText('digital · Stub Publishing · ISBN 9780000000001')
    await expect(copies).toContainText('paperback · Stub Publishing · ISBN 9780010000001')
    await expect(copies).toContainText('The date shown is the earliest of these copies.')
    await edit.screenshot({ path: testInfo.outputPath('book-copies.png'), animations: 'disabled' })
    await edit.getByRole('textbox', { name: 'ISBN', exact: true }).fill('my-own-isbn')
    await edit.getByRole('button', { name: /Save/ }).click()
    await expect(edit).toBeHidden()
    await expect(row.getByText('Edited', { exact: true })).toBeVisible()
  } finally {
    await request.delete(`/api/v1/series/${id}`).catch(() => {})
  }
})
