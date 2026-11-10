import { expect, test } from '@playwright/test'

const idle = { queued: false, running: false, total: 0, done: 0, renamed: 0, retagged: 0, series: [] }

// The Media page names a misspelled token beside its field, takes a token
// from the table into the template being edited, and after a save that
// changes how files are named offers the preview of what existing files
// would do. Settings are kept in the page: other tests read the defaults.
test('media settings check templates and lead from a save to organizing', async ({ page }, testInfo) => {
  let saved: unknown
  await page.route('**/api/v1/settings/media', async (route) => {
    if (route.request().method() === 'PUT') {
      saved = route.request().postDataJSON()
      return route.fulfill({ json: saved })
    }
    return saved ? route.fulfill({ json: saved }) : route.continue()
  })
  await page.route('**/api/v1/library/organize/status', (route) => route.fulfill({ json: idle }))
  await page.route('**/api/v1/library/organize', (route) =>
    route.fulfill({
      json: [
        {
          seriesId: 1,
          title: 'Deterministic Diaries',
          changes: [
            { fileId: 1, from: 'Diaries/Diaries v01.cbz', to: 'Diaries (2004)/Diaries v01.cbz', rename: true, retag: false },
            { fileId: 2, from: 'Diaries/review/x.cbz', to: 'Diaries/review/x.cbz', rename: false, retag: false, skip: 'Not matched to a book yet.' },
          ],
        },
        {
          seriesId: 2,
          title: 'Stub Blade',
          changes: [{ fileId: 3, from: 'Blade/Blade v01.cbz', to: 'Blade (1999)/Blade v01.cbz', rename: true, retag: true }],
        },
      ],
    }),
  )
  await page.goto('/media')

  const folder = page.getByRole('textbox', { name: 'Series folder' })
  await folder.fill('{Seris Title}')
  await expect(page.getByText('series folder template uses an unknown token {Seris Title}')).toBeVisible()

  // A token from the table goes into the template last focused, at the caret.
  await folder.fill('{Series Title} ')
  await page.getByRole('button', { name: 'Insert {year}', exact: true }).click()
  await expect(folder).toHaveValue('{Series Title} {year}')
  await expect(folder).toBeFocused()
  await expect(page.getByText('Preview: ONE PIECE 1997', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'Save changes', exact: true }).click()
  await expect(page.getByText('Existing files are unchanged')).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('media-saved.png'), fullPage: true })
  await page.getByRole('button', { name: 'Preview changes', exact: true }).click()

  // Each series opens to its own file changes.
  const dialog = page.getByRole('dialog', { name: 'Organize files', exact: true })
  await expect(dialog.getByRole('button', { name: /Deterministic Diaries.*1 moved/ })).toBeVisible()
  await expect(dialog.getByText('→ Diaries (2004)/Diaries v01.cbz')).toHaveCount(0)
  await dialog.getByRole('button', { name: /Deterministic Diaries/ }).click()
  await expect(dialog.getByText('→ Diaries (2004)/Diaries v01.cbz')).toBeVisible()
  await expect(dialog.getByText('1 file is left alone: Not matched to a book yet.')).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Apply to 2 files', exact: true })).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('organize-library.png') })
})

// Renaming an edition for readers changes nothing on disk by itself: the
// save leads straight to the preview of the files that still carry the old
// name, and the card says what readers will show.
test('an edition named for readers offers to organize its files', async ({ page, request }) => {
  const added = await request.post('/api/v1/series/', { data: { mangabakaId: 103, monitor: 'all' } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  try {
    const file = {
      id: 1,
      seriesId: id,
      path: '/library/Series/Standard Edition [en]/Series v01.cbz',
      size: 1024,
      provenance: 'torrent',
      presence: 'present',
      facts: {},
      claims: {},
    }
    await page.route(`**/api/v1/series/${id}/files`, (route) => route.fulfill({ json: [file] }))
    await page.route('**/api/v1/library/organize/status', (route) => route.fulfill({ json: idle }))
    await page.route(`**/api/v1/series/${id}/files/organize`, (route) =>
      route.fulfill({
        json: [
          {
            fileId: 1,
            from: 'Standard Edition [en]/Series v01.cbz',
            to: 'Standard Edition [en]/Series v01.cbz',
            rename: false,
            retag: true,
          },
        ],
      }),
    )
    await page.goto(`/series/${id}`)
    await page.getByRole('button', { name: 'More actions for Standard Edition' }).click()
    await page.getByRole('menuitem', { name: 'Edit edition', exact: true }).click()
    const edit = page.getByRole('dialog', { name: 'Edit edition', exact: true })
    await edit.getByLabel('Series name in readers').first().click()
    await page.getByRole('option', { name: 'Series and edition name' }).click()
    await expect(edit.getByText('“Deterministic Diaries Standard Edition”')).toBeVisible()
    await edit.getByRole('button', { name: 'Save edition', exact: true }).click()

    const organize = page.getByRole('dialog', { name: 'Organize files', exact: true })
    await expect(organize.getByText(/name in readers is saved/)).toBeVisible()
    await expect(organize.getByRole('checkbox', { name: 'Update embedded metadata (1)' })).toBeChecked()
    await organize.getByRole('button', { name: 'Close', exact: true }).click()
    await expect(page.getByText('in readers: Deterministic Diaries Standard Edition')).toBeVisible()
  } finally {
    await request.delete(`/api/v1/series/${id}/`)
  }
})
