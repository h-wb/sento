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
    await edition.getByRole('button', { name: 'Add collected version', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Add collected version', exact: true })
    await dialog.getByRole('textbox', { name: 'Version name', exact: true }).fill('Preferred group')
    await dialog.getByRole('textbox', { name: 'Required release group', exact: true }).fill('LuCaZ')
    await dialog.getByRole('button', { name: 'Save version', exact: true }).click()
    await expect(dialog).toBeHidden()
    const targets = await (await request.get(`/api/v1/series/${id}/targets`)).json()
    expect(targets.filter((t: { name: string }) => t.name === 'Preferred group')).toHaveLength(1)
    await edition.getByRole('button', { name: 'Add unit', exact: true }).click()
    const unit = page.getByRole('dialog', { name: 'Add unit', exact: true })
    await unit.getByRole('textbox', { name: 'Display label', exact: true }).fill('0')
    await unit.getByRole('button', { name: 'Save unit', exact: true }).click()
    await expect(unit).toBeHidden()
    await expect(edition.getByRole('button', { name: '0', exact: true })).toBeVisible()
    await edition.getByRole('button', { name: '0', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Volume 0', exact: true })).toBeVisible()
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Close dialog', exact: true })
      .click()
    await edition.getByRole('button', { name: 'Edit edition', exact: true }).click()
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
