import { expect, test } from '@playwright/test'

test('profiles keep independent scores and required or refused formats', async ({ page, request }) => {
  const response = await request.post('/api/v1/formats', {
    data: { name: 'Preferred test group', conditions: [{ type: 'group', value: '^LuCaZ$', required: false, negate: false }] },
  })
  expect(response.ok()).toBeTruthy()
  const format = await response.json()
  let profileId = 0
  try {
    await page.goto('/profiles')
    await page.getByRole('button', { name: 'Add profile', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Add profile', exact: true })
    await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Group test policy')
    await dialog.getByRole('textbox', { name: 'Rule for Preferred test group', exact: true }).click()
    await page.getByRole('option', { name: 'Required', exact: true }).click()
    await dialog.getByRole('textbox', { name: 'Rule for Raw (untranslated)', exact: true }).click()
    await page.getByRole('option', { name: 'Refused', exact: true }).click()
    await dialog.getByRole('textbox', { name: 'Score for Preferred test group', exact: true }).fill('250')
    await dialog.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(dialog).toBeHidden()
    const profiles = await (await request.get('/api/v1/profiles')).json()
    const saved = profiles.find((p: { name: string }) => p.name === 'Group test policy')
    profileId = saved.id
    expect(saved.required).toEqual([format.id])
    expect(saved.refused).toHaveLength(1)
    expect(saved.formatScores[format.id]).toBe(250)
    const standard = profiles.find((p: { name: string }) => p.name === 'Standard')
    expect(standard.formatScores[format.id]).toBeUndefined()
    await page.getByRole('button', { name: 'Edit Group test policy', exact: true }).click()
    const edit = page.getByRole('dialog', { name: 'Edit Group test policy', exact: true })
    await expect(edit.getByRole('textbox', { name: 'Rule for Preferred test group', exact: true })).toHaveValue('Required')
    await expect(edit.getByRole('textbox', { name: 'Rule for Raw (untranslated)', exact: true })).toHaveValue('Refused')
    const exported = await (await request.get(`/api/v1/profiles/${profileId}/export`)).json()
    expect(exported.formats.find((f: { name: string }) => f.name === format.name).rule).toBe('required')
  } finally {
    if (profileId) await request.delete(`/api/v1/profiles/${profileId}`)
    await request.delete(`/api/v1/formats/${format.id}`)
  }
})
