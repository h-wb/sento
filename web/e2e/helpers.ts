import { expect, type APIRequestContext } from '@playwright/test'

// Add a series by its MangaBaka id and return the new series' id — the
// setup most specs share.
export async function addSeries(request: APIRequestContext, mangabakaId: number): Promise<number> {
  const added = await request.post('/api/v1/series/', { data: { mangabakaId, monitor: 'all' } })
  expect(added.ok()).toBeTruthy()
  const { id } = await added.json()
  return id
}
