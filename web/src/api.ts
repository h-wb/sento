import type {
  EditionOverview,
  FileChange,
  HealthReport,
  OrganizeResult,
  ReleaseFileMatch,
  RemovedSeries,
  WantedRow,
} from './types/service'
import type {
  CollectedVersion,
  Book,
  EditionMetadataPatch,
  CatalogLinkPreview,
  AssetCoverage,
  BookRef,
  TrashedFile,
  BookObservation,
  Work,
} from './types/store'
import type { CleanupFile, CleanupResult, BookStatus, FileMappingResult } from './types/service'
import type { ReviewItem, ReviewResolution } from './types/store'
// Typed client for the Sento API. All payload types are generated from the
// Go structs (mise run gen) into ./types/ — the backend is the schema.

import type { SeriesRef, Edition as CatalogEdition } from './types/metadata'
import type {
  FileCleanupBody,
  SearchBody,
  BrowseProwlarrBody,
  EditionPatch,
  CreateEditionBody,
  ExtensionRepos,
  IndexerBody,
  IndexerPatch,
  MergeEditionResponse,
  NamingPreview,
  OverridePatch,
  SearchResponse,
  SeriesDetail,
  StatusResponse,
} from './types/server'
import type {
  AddSeriesRequest,
  ConnectionSettings,
  ConnectionTest,
  ExtensionView,
  ImportProposal,
  MediaSettings,
  ReleaseCandidate,
  IndexerView,
  IndexerSearchCheck,
  ProwlarrIndexer,
  ScanResult,
  SearchEvent,
  SeriesIndexerView,
  SearchScope,
} from './types/service'
import type {
  Binding,
  BlocklistEntry,
  CalendarEntry,
  Edition,
  Decision,
  File as SeriesFile,
  QueueItem,
  Series,
} from './types/store'
import type { TaskStatus } from './types/scheduler'
import type { Manga as SourceManga, Preference, Source } from './types/extensions'
import type { Condition, Format, Profile } from './types/profile'
import type { ProfileDoc } from './types/service'

export type {
  Binding,
  BrowseProwlarrBody,
  ConnectionSettings,
  ConnectionTest,
  ExtensionView,
  FileMappingResult,
  IndexerBody,
  IndexerPatch,
  IndexerView,
  Preference,
  ProwlarrIndexer,
  BlocklistEntry,
  CalendarEntry,
  Edition,
  EditionPatch,
  Decision,
  ImportProposal,
  MediaSettings,
  NamingPreview,
  QueueItem,
  ReleaseCandidate,
  ScanResult,
  SearchEvent,
  SearchResponse,
  Series,
  SeriesDetail,
  SeriesFile,
  SeriesIndexerView,
  Source,
  SourceManga,
  SearchScope,
  TaskStatus,
  Condition,
  Format,
  Profile,
  ProfileDoc,
}
export type SystemStatus = StatusResponse
export type LookupResult = SeriesRef
export type SiteOverride = OverridePatch
export type Ledger = 'volume' | 'chapter'

const base = '/api/v1'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(base + path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const body = await res.json()
      if (body.error) msg = body.error
    } catch {
      /* keep status message */
    }
    throw new Error(msg)
  }
  // Empty bodies (204, 202, …) are valid void responses.
  const text = await res.text()
  return text ? (JSON.parse(text) as T) : (undefined as T)
}

export const api = {
  setBindingEdition: (id: number, bid: number, editionId: number) =>
    request<void>(`/series/${id}/bindings/${bid}`, {
      method: 'PUT',
      body: JSON.stringify({ editionId }),
    }),
  bookMonitor: (id: number, versionId: number, bookId: number, monitored: boolean) =>
    request<void>(`/series/${id}/book-monitor`, {
      method: 'PUT',
      body: JSON.stringify({ versionId, bookId, monitored }),
    }),
  versions: (id: number) => request<CollectedVersion[]>(`/series/${id}/versions`),
  correctFileFacts: (id: number, fid: number, patch: { language?: string; group?: string }) =>
    request<void>(`/series/${id}/files/${fid}/facts`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),
  planOrganize: (id: number) => request<FileChange[]>(`/series/${id}/files/organize`),
  organize: (id: number, body: { rename: boolean; retag: boolean }) =>
    request<OrganizeResult>(`/series/${id}/files/organize`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  checkFiles: (id: number) => request<void>(`/series/${id}/check-files`, { method: 'POST' }),
  editionOverview: (id: number) => request<EditionOverview>(`/series/${id}/edition-overview`),
  // Older servers answered this route with a paged object. Say so instead
  // of letting the page fail on a shape it cannot read.
  versionBooksOf: async (id: number, version: number) => {
    const books = await request<BookStatus[]>(`/series/${id}/versions/${version}/books`)
    if (!Array.isArray(books)) {
      throw new Error(
        'The Sento server is older than this page and returned books in an outdated format. Restart or update the server, then reload.',
      )
    }
    return books.map((b) => ({ ...b, fileIds: b.fileIds ?? [], onDiskFileIds: b.onDiskFileIds ?? [] }))
  },
  bulkMonitor: (id: number, version: number, bookIds: number[], monitored: boolean) =>
    request<void>(`/series/${id}/versions/${version}/monitor`, {
      method: 'POST',
      body: JSON.stringify({ bookIds, monitored }),
    }),
  trustBinding: (id: number, bid: number, trustChapters: boolean) =>
    request<void>(`/series/${id}/bindings/${bid}`, {
      method: 'PUT',
      body: JSON.stringify({ trustChapters }),
    }),
  review: (seriesId = 0) => request<ReviewItem[]>(`/review?seriesId=${seriesId}`),
  retryQueue: (id: number, remoteId = '') =>
    request<void>(`/queue/${id}/retry`, { method: 'POST', body: JSON.stringify({ remoteId }) }),
  cancelQueue: (id: number) => request<void>(`/queue/${id}/cancel`, { method: 'POST' }),
  versionBooks: (id: number) => request<Record<string, BookStatus[]>>(`/series/${id}/books`),
  saveVersion: (id: number, version: CollectedVersion) =>
    request<CollectedVersion>(`/series/${id}/versions${version.id ? `/${version.id}` : ''}`, {
      method: version.id ? 'PUT' : 'POST',
      body: JSON.stringify(version),
    }),
  deleteVersion: (id: number, tid: number) =>
    request<void>(`/series/${id}/versions/${tid}`, { method: 'DELETE' }),
  saveBook: (id: number, book: Book) =>
    request<Book>(`/series/${id}/books${book.id ? `/${book.id}` : ''}`, {
      method: book.id ? 'PUT' : 'POST',
      body: JSON.stringify(book),
    }),
  splitBox: (id: number, uid: number, count: number) =>
    request<Book[]>(`/series/${id}/books/${uid}/split`, {
      method: 'POST',
      body: JSON.stringify({ count }),
    }),
  resetBook: (id: number, uid: number) =>
    request<void>(`/series/${id}/books/${uid}/catalog-reset`, { method: 'POST' }),
  deleteBook: (id: number, uid: number) =>
    request<void>(`/series/${id}/books/${uid}`, { method: 'DELETE' }),
  editEdition: (id: number, cid: number, patch: EditionMetadataPatch) =>
    request<void>(`/series/${id}/editions/${cid}/metadata`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  mergePreview: (id: number, cid: number, into: number) =>
    request<CatalogLinkPreview>(`/series/${id}/editions/${cid}/merge?preview=1`, {
      method: 'POST',
      body: JSON.stringify({ into }),
    }),
  catalogLinkPreview: (id: number, cid: number, catalogId: number) =>
    request<CatalogLinkPreview>(`/series/${id}/editions/${cid}/catalog-link?preview=1`, {
      method: 'POST',
      body: JSON.stringify({ catalogId }),
    }),
  linkCatalog: (id: number, cid: number, catalogId: number) =>
    request<void>(`/series/${id}/editions/${cid}/catalog-link`, {
      method: 'POST',
      body: JSON.stringify({ catalogId }),
    }),
  // "Not the same edition": stops the link from being suggested again.
  declineCatalogLink: (id: number, cid: number, catalogId: number) =>
    request<void>(`/series/${id}/editions/${cid}/catalog-link?decline=1`, {
      method: 'POST',
      body: JSON.stringify({ catalogId }),
    }),
  coverage: (id: number) => request<AssetCoverage[]>(`/series/${id}/coverage`),
  pinCoverage: (id: number, coverage: AssetCoverage) =>
    request<void>(`/series/${id}/coverage/pin`, {
      method: 'PUT',
      body: JSON.stringify(coverage),
    }),
  mapFile: (id: number, fid: number, books: BookRef[], replace = false) =>
    request<FileMappingResult>(`/series/${id}/files/${fid}/mapping`, {
      method: 'PUT',
      body: JSON.stringify({ books, ...(replace ? { replace: true } : {}) }),
    }),
  // Decline a pack of file reviews at once: every file reviewed, unmapped.
  resolveFileReviews: (id: number, fileIds: number[]) =>
    request<ReviewResolution>(`/series/${id}/review/resolve`, {
      method: 'POST',
      body: JSON.stringify({ fileIds }),
    }),
  // Apply one collected version to a whole pack: each file maps by its own
  // parsed numbers; unresolved files come back with the reason.
  matchRelease: (id: number, versionId: number, fileIds: number[]) =>
    request<ReleaseFileMatch[]>(`/series/${id}/review/match`, {
      method: 'POST',
      body: JSON.stringify({ versionId, fileIds }),
    }),
  observations: (id: number) => request<BookObservation[]>(`/series/${id}/observations`),
  reviewObservation: (id: number, oid: number, accept: boolean) =>
    request<void>(`/series/${id}/observations/${oid}`, {
      method: 'PUT',
      body: JSON.stringify({ accept }),
    }),

  status: () => request<SystemStatus>('/system/status'),
  healthChecks: () => request<HealthReport>('/system/checks'),
  runHealthChecks: () => request<HealthReport>('/system/checks', { method: 'POST' }),
  systemTasks: () => request<TaskStatus[]>('/system/tasks'),
  runTask: (slug: string) => request<void>(`/system/tasks/${slug}/run`, { method: 'POST' }),
  lookup: (q: string) => request<LookupResult[]>(`/lookup?q=${encodeURIComponent(q)}`),
  calendar: (days = 60) => request<CalendarEntry[]>(`/calendar?days=${days}`),
  wanted: () => request<WantedRow[]>('/wanted'),

  listSeries: () => request<Series[]>('/series/'),
  addSeries: (body: Partial<AddSeriesRequest> & Pick<AddSeriesRequest, 'mangabakaId'>) =>
    request<Series>('/series/', { method: 'POST', body: JSON.stringify(body) }),
  getSeries: (id: number) => request<SeriesDetail>(`/series/${id}/`),
  deleteSeries: (id: number, opts: { deleteFiles?: boolean; removeDownloads?: boolean } = {}) =>
    request<RemovedSeries>(
      `/series/${id}/?deleteFiles=${!!opts.deleteFiles}&removeDownloads=${!!opts.removeDownloads}`,
      { method: 'DELETE' },
    ),
  setSeriesMonitored: (id: number, monitored: boolean) =>
    request<void>(`/series/${id}/`, {
      method: 'PUT',
      body: JSON.stringify({ monitored }),
    }),
  setSeriesProfile: (id: number, profileId: number) =>
    request<void>(`/series/${id}/`, {
      method: 'PUT',
      body: JSON.stringify({ profileId }),
    }),
  refreshSeries: (id: number) => request<unknown>(`/series/${id}/refresh`, { method: 'POST' }),
  updateEdition: (id: number, cid: number, patch: EditionPatch) =>
    request<void>(`/series/${id}/editions/${cid}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  setEditionMonitored: (id: number, cid: number, monitored: boolean) =>
    request<void>(`/series/${id}/editions/${cid}`, {
      method: 'PUT',
      body: JSON.stringify({ monitored }),
    }),
  createEdition: (id: number, body: CreateEditionBody) =>
    request<Edition>(`/series/${id}/editions`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  bookWorks: (id: number, bookId: number) => request<Work[]>(`/series/${id}/books/${bookId}/works`),
  separateCollection: (id: number, collectionId: number) =>
    request<{ editionId: number }>(`/series/${id}/collections/${collectionId}/separate`, {
      method: 'POST',
    }),
  catalogEditions: () => request<CatalogEdition[]>('/catalog/editions'),
  mergeEditions: (id: number, cid: number, into: number) =>
    request<MergeEditionResponse>(`/series/${id}/editions/${cid}/merge`, {
      method: 'POST',
      body: JSON.stringify({ into }),
    }),
  deleteEdition: (id: number, cid: number) =>
    request<void>(`/series/${id}/editions/${cid}`, { method: 'DELETE' }),
  files: (id: number) => request<SeriesFile[]>(`/series/${id}/files`),

  listProfiles: () => request<Profile[]>('/profiles/'),
  createProfile: (p: Omit<Profile, 'id'>) =>
    request<Profile>('/profiles/', { method: 'POST', body: JSON.stringify(p) }),
  updateProfile: (id: number, p: Omit<Profile, 'id'>) =>
    request<void>(`/profiles/${id}/`, {
      method: 'PUT',
      body: JSON.stringify(p),
    }),
  deleteProfile: (id: number) => request<void>(`/profiles/${id}/`, { method: 'DELETE' }),
  exportProfile: (id: number) => request<ProfileDoc>(`/profiles/${id}/export`),
  importProfile: (doc: ProfileDoc) =>
    request<Profile>('/profiles/import', {
      method: 'POST',
      body: JSON.stringify(doc),
    }),
  defaultProfile: () => request<{ profileId: number }>('/profiles/default'),
  setDefaultProfile: (profileId: number) =>
    request<void>('/profiles/default', {
      method: 'PUT',
      body: JSON.stringify({ profileId }),
    }),
  listFormats: () => request<Format[]>('/formats/'),
  createFormat: (f: Omit<Format, 'id'>) =>
    request<Format>('/formats/', { method: 'POST', body: JSON.stringify(f) }),
  updateFormat: (id: number, f: Omit<Format, 'id'>) =>
    request<void>(`/formats/${id}`, { method: 'PUT', body: JSON.stringify(f) }),
  deleteFormat: (id: number) => request<void>(`/formats/${id}`, { method: 'DELETE' }),

  connections: () => request<ConnectionSettings>('/settings/connections'),
  extensions: () => request<ExtensionView[]>('/extensions'),
  extensionRepos: () => request<ExtensionRepos>('/extensions/repos'),
  setExtensionRepos: (urls: string[]) =>
    request<ExtensionRepos>('/extensions/repos', {
      method: 'PUT',
      body: JSON.stringify({ urls }),
    }),
  // Preferences are addressed by host source id (settings.sourceId of a
  // source entry), not entry id: the indexer modal edits them before the
  // entry exists.
  sourcePreferences: (sourceId: string) =>
    request<Preference[]>(`/extensions/sources/${encodeURIComponent(sourceId)}/preferences`),
  setSourcePreference: (sourceId: string, key: string, value: unknown) =>
    request<void>(`/extensions/sources/${encodeURIComponent(sourceId)}/preferences`, {
      method: 'PUT',
      body: JSON.stringify({ key, value }),
    }),
  installExtension: (pkg: string) =>
    request<void>(`/extensions/${encodeURIComponent(pkg)}`, { method: 'POST' }),
  extensionSources: (pkg: string) =>
    request<Source[]>(`/extensions/${encodeURIComponent(pkg)}/sources`),
  saveConnections: (c: ConnectionSettings) =>
    request<ConnectionSettings>('/settings/connections', {
      method: 'PUT',
      body: JSON.stringify(c),
    }),
  testConnections: (c: ConnectionSettings) =>
    request<ConnectionTest[]>('/settings/connections/test', {
      method: 'POST',
      body: JSON.stringify(c),
    }),

  mediaSettings: () => request<MediaSettings>('/settings/media'),
  saveMediaSettings: (m: MediaSettings) =>
    request<MediaSettings>('/settings/media', {
      method: 'PUT',
      body: JSON.stringify(m),
    }),
  previewNaming: (m: MediaSettings) =>
    request<NamingPreview>('/settings/media/preview', {
      method: 'POST',
      body: JSON.stringify(m),
    }),

  sources: () => request<Source[]>('/sources'),
  indexers: () => request<IndexerView[]>('/indexers'),
  addIndexer: (body: IndexerBody) =>
    request<IndexerView>('/indexers', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  updateIndexer: (iid: number, body: IndexerBody) =>
    request<IndexerView>(`/indexers/${iid}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  patchIndexer: (iid: number, patch: IndexerPatch) =>
    request<IndexerView>(`/indexers/${iid}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),
  deleteIndexer: (iid: number) => request<void>(`/indexers/${iid}`, { method: 'DELETE' }),
  testIndexer: (iid: number) => request<void>(`/indexers/${iid}/test`, { method: 'POST' }),
  testIndexerSearch: (iid: number, query: string) =>
    request<IndexerSearchCheck>(`/indexers/${iid}/search-test`, { method: 'POST', body: JSON.stringify({ query }) }),
  browseProwlarr: (body: BrowseProwlarrBody) =>
    request<ProwlarrIndexer[]>('/indexers/browse', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  seriesIndexers: (id: number) => request<SeriesIndexerView[]>(`/series/${id}/indexers`),
  setSeriesIndexerOverride: (id: number, indexerId: number, ov: OverridePatch) =>
    request<unknown>(`/series/${id}/indexers/${indexerId}`, {
      method: 'PUT',
      body: JSON.stringify(ov),
    }),
  sourceSearch: (sid: string, q: string) =>
    request<SourceManga[]>(`/sources/${sid}/search?q=${encodeURIComponent(q)}`),
  bindings: (id: number) => request<Binding[]>(`/series/${id}/bindings`),
  addBinding: (
    id: number,
    body: {
      sourceId: string
      sourceMangaKey: string
      sourceName?: string
      mangaTitle?: string
    },
  ) =>
    request<{ id: number }>(`/series/${id}/bindings`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  setBindingState: (id: number, bid: number, state: 'active' | 'disabled') =>
    request<void>(`/series/${id}/bindings/${bid}`, {
      method: 'PUT',
      body: JSON.stringify({ state }),
    }),
  deleteBinding: (id: number, bid: number) =>
    request<void>(`/series/${id}/bindings/${bid}`, { method: 'DELETE' }),

  search: (
    id: number,
    opts: Partial<SearchBody> & { ledger: Ledger },
  ) =>
    request<SearchResponse>(`/series/${id}/search`, {
      method: 'POST',
      body: JSON.stringify(opts),
    }),
  // Streaming dry-run search: provider batches arrive as they complete.
  searchStream: async (
    id: number,
    opts: { ledger: Ledger; scope?: SearchScope },
    onEvent: (ev: SearchEvent) => void,
    signal?: AbortSignal,
  ) => {
    const res = await fetch(`${base}/series/${id}/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...opts, dryRun: true, stream: true }),
      signal,
    })
    if (!res.ok || !res.body) {
      let msg = `HTTP ${res.status}`
      try {
        const body = await res.json()
        if (body.error) msg = body.error
      } catch {
        /* keep status message */
      }
      throw new Error(msg)
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    let finished = false
    const emit = (line: string) => {
      if (!line.trim()) return
      const ev = JSON.parse(line) as SearchEvent
      if (ev.type === 'done' || ev.type === 'error') finished = true
      onEvent(ev)
    }
    try {
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        let nl
        while ((nl = buf.indexOf('\n')) >= 0) {
          emit(buf.slice(0, nl))
          buf = buf.slice(nl + 1)
        }
      }
      buf += decoder.decode()
      emit(buf)
      if (!finished)
        throw new Error(
          'Search ended before all sources replied. These results may be incomplete; search again to retry.',
        )
    } finally {
      reader.releaseLock()
    }
  },
  cleanupPreview: (id: number, editionId?: number) => request<CleanupFile[]>(`/series/${id}/files/cleanup${editionId ? `?editionId=${editionId}` : ''}`),
  cleanupFiles: (id: number, body: FileCleanupBody) => request<CleanupResult>(`/series/${id}/files/cleanup`, { method: 'POST', body: JSON.stringify(body) }),
  fileTrash: (id: number) => request<TrashedFile[]>(`/series/${id}/files/trash`),
  restoreTrash: (id: number, fid: number) => request<void>(`/series/${id}/files/trash/${fid}/restore`, { method: 'POST' }),
  purgeTrash: (id: number, fid: number) => request<void>(`/series/${id}/files/trash/${fid}`, { method: 'DELETE' }),
  deleteFile: (fid: number) => request<void>(`/files/${fid}`, { method: 'DELETE' }),

  scanSeries: (id: number, path?: string) =>
    request<ScanResult>(`/series/${id}/import`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),
  importScan: (root?: string) =>
    request<ImportProposal[]>('/library/import/scan', {
      method: 'POST',
      body: JSON.stringify({ root: root ?? '' }),
    }),
  importFolder: (dir: string, mangabakaId: number) =>
    request<{ seriesId: number; scan: ScanResult | null; existing?: boolean }>('/library/import', {
      method: 'POST',
      body: JSON.stringify({ dir, mangabakaId }),
    }),

  activity: (seriesId = 0) => request<import('./types/service').ActivityData>(`/activity?seriesId=${seriesId}`),
  queue: (all = false) => request<QueueItem[]>(`/queue${all ? '?all=1' : ''}`),
  tryAnotherRelease: (ids: number[]) => request<void>('/queue/another', { method: 'POST', body: JSON.stringify({ ids }) }),
  processQueue: () => request<{ processed: number }>('/queue/process', { method: 'POST' }),
  blocklistQueueItem: (qid: number) => request<void>(`/queue/${qid}/blocklist`, { method: 'POST' }),

  history: (seriesId?: number, limit = 100) =>
    request<Decision[]>(
      seriesId ? `/series/${seriesId}/history?limit=${limit}` : `/history?limit=${limit}`,
    ),
  blocklist: () => request<BlocklistEntry[]>('/blocklist'),
  unblock: (id: number) => request<void>(`/blocklist/${id}`, { method: 'DELETE' }),
}

export function fmtNum(hundredths: number): string {
  const whole = Math.floor(hundredths / 100)
  const frac = hundredths % 100
  if (frac === 0) return String(whole)
  return (hundredths / 100).toString()
}

export function fmtSize(bytes: number): string {
  if (bytes >= 1 << 30) return (bytes / (1 << 30)).toFixed(1) + ' GiB'
  if (bytes >= 1 << 20) return (bytes / (1 << 20)).toFixed(1) + ' MiB'
  return Math.round(bytes / 1024) + ' KiB'
}
