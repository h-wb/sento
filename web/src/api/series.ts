import type { CollectedVersion, Book, AssetCoverage, Work, Series } from '../types/store'
import type {
  CopyReaderNames,
  LibraryOrganizeRequest,
  LibraryOrganizeStatus,
  SeriesOrganizePlan,
  SeriesProgress,
  ReaderNamesRequest,
  FileChange,
  RemovedSeries,
  ScanResult,
  ImportProposal,
  AddSeriesRequest,
  SeriesIndexerView,
  BookStatus,
} from '../types/service'
import type {
  SeriesDetail,
  OverridePatch,
  IDResponse,
  SeparateCollectionResponse,
  ImportFolderResponse,
} from '../types/server'
import { request } from './request'

export const seriesApi = {
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
  // What readers show for each card, or would with an edition's unsaved
  // reader templates.
  readerNames: (id: number, body: ReaderNamesRequest = {}) =>
    request<CopyReaderNames[]>(`/series/${id}/reader-names`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  seriesIndexers: (id: number) => request<SeriesIndexerView[]>(`/series/${id}/indexers`),
  setSeriesIndexerOverride: (id: number, indexerId: number, ov: OverridePatch) =>
    request<unknown>(`/series/${id}/indexers/${indexerId}`, {
      method: 'PUT',
      body: JSON.stringify(ov),
    }),
  setBindingEdition: (id: number, bid: number, editionId: number) =>
    request<void>(`/series/${id}/bindings/${bid}`, {
      method: 'PUT',
      body: JSON.stringify({ editionId }),
    }),
  trustBinding: (id: number, bid: number, trustChapters: boolean) =>
    request<void>(`/series/${id}/bindings/${bid}`, {
      method: 'PUT',
      body: JSON.stringify({ trustChapters }),
    }),
  addBinding: (
    id: number,
    body: {
      sourceId: string
      sourceMangaKey: string
      sourceName?: string
      mangaTitle?: string
    },
  ) =>
    request<IDResponse>(`/series/${id}/bindings`, {
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
  bookMonitor: (id: number, versionId: number, bookId: number, monitored: boolean) =>
    request<void>(`/series/${id}/book-monitor`, {
      method: 'PUT',
      body: JSON.stringify({ versionId, bookId, monitored }),
    }),
  versions: (id: number) => request<CollectedVersion[]>(`/series/${id}/versions`),
  planOrganize: (id: number) => request<FileChange[]>(`/series/${id}/files/organize`),
  planOrganizeLibrary: (ids?: number[]) =>
    request<SeriesOrganizePlan[]>(
      `/library/organize${ids?.length ? `?series=${ids.join(',')}` : ''}`,
    ),
  organizeLibrary: (body: LibraryOrganizeRequest) =>
    request<LibraryOrganizeStatus>('/library/organize', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  organizeLibraryStatus: () => request<LibraryOrganizeStatus>('/library/organize/status'),
  libraryProgress: () => request<SeriesProgress[]>('/library/progress'),
  checkFiles: (id: number) => request<void>(`/series/${id}/check-files`, { method: 'POST' }),
  // Older servers answered this route with a paged object. Say so instead
  // of letting the page fail on a shape it cannot read.
  versionBooksOf: async (id: number, version: number) => {
    const books = await request<BookStatus[]>(`/series/${id}/versions/${version}/books`)
    if (!Array.isArray(books)) {
      throw new Error(
        'The Sento server is older than this page and returned books in an outdated format. Restart or update the server, then reload.',
      )
    }
    return books.map((b) => ({
      ...b,
      fileIds: b.fileIds ?? [],
      onDiskFileIds: b.onDiskFileIds ?? [],
    }))
  },
  bulkMonitor: (id: number, version: number, bookIds: number[], monitored: boolean) =>
    request<void>(`/series/${id}/versions/${version}/monitor`, {
      method: 'POST',
      body: JSON.stringify({ bookIds, monitored }),
    }),
  versionBooks: (id: number) => request<Record<string, BookStatus[]>>(`/series/${id}/books`),
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
  bookWorks: (id: number, bookId: number) => request<Work[]>(`/series/${id}/books/${bookId}/works`),
  separateCollection: (id: number, collectionId: number) =>
    request<SeparateCollectionResponse>(`/series/${id}/collections/${collectionId}/separate`, {
      method: 'POST',
    }),
  coverage: (id: number) => request<AssetCoverage[]>(`/series/${id}/coverage`),
  pinCoverage: (id: number, coverage: AssetCoverage) =>
    request<void>(`/series/${id}/coverage/pin`, {
      method: 'PUT',
      body: JSON.stringify(coverage),
    }),
  reviewObservation: (id: number, oid: number, accept: boolean) =>
    request<void>(`/series/${id}/observations/${oid}`, {
      method: 'PUT',
      body: JSON.stringify({ accept }),
    }),
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
    request<ImportFolderResponse>('/library/import', {
      method: 'POST',
      body: JSON.stringify({ dir, mangabakaId }),
    }),
}
