import { queryOptions, type Query, type QueryClient } from '@tanstack/react-query'
import { api } from './api'

// One place for the server-state queries several components define, and for
// the invalidation groups behind "the library changed" and friends.

export const seriesQueries = {
  list: () => queryOptions({ queryKey: ['series'], queryFn: api.listSeries }),
  detail: (id: number) =>
    queryOptions({ queryKey: ['series', id], queryFn: () => api.getSeries(id) }),
  versions: (id: number) =>
    queryOptions({ queryKey: ['series', id, 'versions'], queryFn: () => api.versions(id) }),
  overview: (id: number) =>
    queryOptions({
      queryKey: ['series', id, 'edition-overview'],
      queryFn: () => api.editionOverview(id),
    }),
  coverage: (id: number) =>
    queryOptions({ queryKey: ['series', id, 'coverage'], queryFn: () => api.coverage(id) }),
  readerNames: (id: number) =>
    queryOptions({ queryKey: ['series', id, 'reader-names'], queryFn: () => api.readerNames(id) }),
  versionBooks: (id: number) =>
    queryOptions({
      queryKey: ['series', id, 'version-books'],
      queryFn: () => api.versionBooks(id),
    }),
  files: (id: number) =>
    queryOptions({ queryKey: ['series', id, 'files'], queryFn: () => api.files(id) }),
  books: (id: number, versionId: number) =>
    queryOptions({
      queryKey: ['series', id, 'books', versionId],
      queryFn: () => api.versionBooksOf(id, versionId),
    }),
}

export const activityQuery = (seriesID = 0) =>
  queryOptions({
    queryKey: ['activity', seriesID],
    queryFn: () => api.activity(seriesID),
    refetchInterval: 10000,
  })

export const connectionsQuery = queryOptions({
  queryKey: ['connections'],
  queryFn: api.connections,
})

export const extensionsQuery = queryOptions({ queryKey: ['extensions'], queryFn: api.extensions })

export const profilesQuery = queryOptions({ queryKey: ['profiles'], queryFn: api.listProfiles })

export const defaultProfileQuery = queryOptions({
  queryKey: ['default-profile'],
  queryFn: api.defaultProfile,
})

export const formatsQuery = queryOptions({ queryKey: ['formats'], queryFn: api.listFormats })

// "The library changed": series (and everything under it), activity, wanted
// and the blocklist. With keepReviewChoices, the open review editors keep
// their snapshot until the saved receipt is dismissed.
export function invalidateLibrary(qc: QueryClient, opts?: { keepReviewChoices?: boolean }) {
  const predicate = opts?.keepReviewChoices
    ? (q: Query) => !q.queryKey.includes('review-choices')
    : undefined
  return Promise.all(
    (['series', 'activity', 'wanted', 'blocklist'] as const).map((key) =>
      qc.invalidateQueries({ queryKey: [key], predicate }),
    ),
  )
}

// The extension host's view of the world: catalog, repos, indexers, health.
export function invalidateExtensions(qc: QueryClient) {
  return Promise.all(
    (['extensions', 'extension-repos', 'indexers', 'health'] as const).map((key) =>
      qc.invalidateQueries({ queryKey: [key] }),
    ),
  )
}

export function invalidateProfiles(qc: QueryClient) {
  return Promise.all(
    (['profiles', 'formats', 'default-profile'] as const).map((key) =>
      qc.invalidateQueries({ queryKey: [key] }),
    ),
  )
}
