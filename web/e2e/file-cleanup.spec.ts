import { expect, test } from '@playwright/test'

test('file cleanup previews protection, confirms retention, and restores from trash', async ({
  page,
  request,
}) => {
  const added = await request.post('/api/v1/series/', {
    data: { mangabakaId: 104, monitor: 'all' },
  })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  const eligible = {
    fileId: 20001,
    path: '/library/Example/old.cbz',
    size: 4096,
    editions: ['Older edition'],
    category: 'uncollected',
    reason: 'Only used by editions you are not collecting',
    eligible: true,
  }
  const protectedFile = {
    fileId: 20002,
    path: '/library/Example/shared.cbz',
    size: 8192,
    editions: ['Older edition', 'Collected edition'],
    category: '',
    reason: 'An edition still uses this file',
    eligible: false,
  }
  let moved = false
  let selection: unknown
  await page.route(`**/api/v1/series/${id}/files/cleanup`, async (route) => {
    if (route.request().method() === 'POST') {
      selection = route.request().postDataJSON()
      moved = true
      return route.fulfill({ json: { moved: [eligible.fileId], skipped: [] } })
    }
    return route.fulfill({ json: moved ? [protectedFile] : [eligible, protectedFile] })
  })
  await page.route(`**/api/v1/series/${id}/files/trash`, (route) =>
    route.fulfill({
      json: moved
        ? [
            {
              fileId: eligible.fileId,
              seriesId: id,
              path: eligible.path,
              trashPath: '/library/Example/.sento-trash/20001/old.cbz',
              size: 4096,
              trashedAt: '2026-11-02 12:00:00',
              purgeAfter: '2026-12-02 12:00:00',
            },
          ]
        : [],
    }),
  )
  await page.route(`**/api/v1/series/${id}/files/trash/${eligible.fileId}/restore`, (route) => {
    moved = false
    return route.fulfill({ status: 204 })
  })
  try {
    await page.goto(`/series/${id}?view=files`)
    await page.getByRole('button', { name: 'Clean up files', exact: true }).click()
    const modal = page.getByRole('dialog', { name: 'Clean up files', exact: true })
    await expect(
      modal.getByRole('checkbox', { name: 'Select old.cbz', exact: true }),
    ).not.toBeChecked()
    await expect(
      modal.getByRole('checkbox', { name: 'Select shared.cbz', exact: true }),
    ).toBeDisabled()
    await expect(
      modal.getByRole('button', { name: 'Move selected to trash', exact: true }),
    ).toBeDisabled()
    await modal.getByRole('checkbox', { name: 'Select eligible files in this view' }).check()
    await modal.getByRole('button', { name: 'Move selected to trash', exact: true }).click()
    const confirmation = page.getByRole('dialog', { name: 'Move 1 file to trash?' })
    await expect(confirmation.getByText(/kept for 30 days/)).toBeVisible()
    await expect(confirmation.getByText(/Download-client payloads stay in place/)).toBeVisible()
    await confirmation.getByRole('button', { name: 'Move to trash', exact: true }).click()
    await expect
      .poll(() => selection)
      .toEqual({ editionId: 0, fileIds: [eligible.fileId], retentionDays: 30 })
    await modal.getByRole('tab', { name: 'Trash (1)' }).click()
    await expect(modal.getByTestId('trashed-file')).toHaveCount(1)
    await modal.getByRole('button', { name: 'Delete permanently', exact: true }).click()
    const permanent = page.getByRole('dialog', { name: 'Permanently delete this file?' })
    await expect(permanent.getByText(/cannot be undone/)).toBeVisible()
    await permanent.getByRole('button', { name: 'Cancel', exact: true }).click()
    await modal.getByRole('button', { name: 'Restore', exact: true }).click()
    await expect(modal.getByText('Trash is empty.', { exact: true })).toBeVisible()
    await modal.getByRole('tab', { name: 'Preview', exact: true }).click()
    await expect(modal.getByTestId('cleanup-file')).toHaveCount(2)
    await page.setViewportSize({ width: 390, height: 844 })
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
      .toBe(true)
  } finally {
    await request.delete(`/api/v1/series/${id}`).catch(() => {})
  }
})
