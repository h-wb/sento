// Typed client for the Sento API. All payload types are generated from the
// Go structs (mise run gen) into ./types/ — the backend is the schema.

import type { ChapterSlot, VolumeSlot } from './types/ledger'
import type { SeriesRef } from './types/metadata'
import type {
  BrowseProwlarrBody,
  ChapterLedgerPatch,
  CreateCollectionBody,
  ExtensionRepos,
  IndexerBody,
  IndexerPatch,
  MergeCollectionResponse,
  NamingPreview,
  OverridePatch,
  SearchResponse,
  SeriesDetail,
  SlotsResponse,
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
  ProwlarrIndexer,
  ScanResult,
  SearchEvent,
  SeriesIndexerView,
  TargetSlot,
} from './types/service'
import type {
  Binding,
  BlocklistEntry,
  CalendarEntry,
  ChapterLedger,
  Collection,
  Decision,
  File as SeriesFile,
  QueueItem,
  Series,
} from './types/store'
import type { Manga as SourceManga, Preference, Source } from './types/sourcehost'

export type {
  Binding,
  BrowseProwlarrBody,
  ConnectionSettings,
  ConnectionTest,
  ExtensionView,
  IndexerBody,
  IndexerPatch,
  IndexerView,
  Preference,
  ProwlarrIndexer,
  BlocklistEntry,
  CalendarEntry,
  ChapterLedger,
  ChapterSlot,
  Collection,
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
  TargetSlot,
  VolumeSlot,
}
export type Slots = SlotsResponse
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
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  status: () => request<SystemStatus>('/system/status'),
  lookup: (q: string) => request<LookupResult[]>(`/lookup?q=${encodeURIComponent(q)}`),
  calendar: (days = 60) => request<CalendarEntry[]>(`/calendar?days=${days}`),

  listSeries: () => request<Series[]>('/series/'),
  addSeries: (body: Partial<AddSeriesRequest> & Pick<AddSeriesRequest, 'mangabakaId'>) =>
    request<Series>('/series/', { method: 'POST', body: JSON.stringify(body) }),
  getSeries: (id: number) => request<SeriesDetail>(`/series/${id}/`),
  deleteSeries: (id: number) => request<void>(`/series/${id}/`, { method: 'DELETE' }),
  setSeriesMonitored: (id: number, monitored: boolean) =>
    request<void>(`/series/${id}/`, { method: 'PUT', body: JSON.stringify({ monitored }) }),
  refreshSeries: (id: number) => request<unknown>(`/series/${id}/refresh`, { method: 'POST' }),
  slots: (id: number) => request<Slots>(`/series/${id}/slots`),
  setCollectionMonitored: (id: number, cid: number, monitored: boolean) =>
    request<unknown>(`/series/${id}/collections/${cid}`, {
      method: 'PUT',
      body: JSON.stringify({ monitored }),
    }),
  createCollection: (id: number, body: CreateCollectionBody) =>
    request<Collection>(`/series/${id}/collections`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  mergeCollection: (id: number, cid: number, into: number) =>
    request<MergeCollectionResponse>(`/series/${id}/collections/${cid}/merge`, {
      method: 'POST',
      body: JSON.stringify({ into }),
    }),
  deleteCollection: (id: number, cid: number) =>
    request<void>(`/series/${id}/collections/${cid}`, { method: 'DELETE' }),
  updateChapterLedger: (id: number, body: ChapterLedgerPatch) =>
    request<ChapterLedger>(`/series/${id}/chapter-ledger`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  files: (id: number) => request<SeriesFile[]>(`/series/${id}/files`),

  connections: () => request<ConnectionSettings>('/settings/connections'),
  extensions: () => request<ExtensionView[]>('/extensions'),
  extensionRepos: () => request<ExtensionRepos>('/extensions/repos'),
  setExtensionRepos: (urls: string[]) =>
    request<ExtensionRepos>('/extensions/repos', { method: 'PUT', body: JSON.stringify({ urls }) }),
  sourcePreferences: (sid: string) =>
    request<Preference[]>(`/sources/${encodeURIComponent(sid)}/preferences`),
  setSourcePreference: (sid: string, key: string, value: unknown) =>
    request<void>(`/sources/${encodeURIComponent(sid)}/preferences`, {
      method: 'PUT',
      body: JSON.stringify({ key, value }),
    }),
  installExtension: (pkg: string) =>
    request<void>(`/extensions/${encodeURIComponent(pkg)}`, { method: 'POST' }),
  uninstallExtension: (pkg: string) =>
    request<void>(`/extensions/${encodeURIComponent(pkg)}`, { method: 'DELETE' }),
  saveConnections: (c: ConnectionSettings) =>
    request<ConnectionSettings>('/settings/connections', { method: 'PUT', body: JSON.stringify(c) }),
  testConnections: (c: ConnectionSettings) =>
    request<ConnectionTest[]>('/settings/connections/test', {
      method: 'POST',
      body: JSON.stringify(c),
    }),

  mediaSettings: () => request<MediaSettings>('/settings/media'),
  saveMediaSettings: (m: MediaSettings) =>
    request<MediaSettings>('/settings/media', { method: 'PUT', body: JSON.stringify(m) }),
  previewNaming: (m: MediaSettings) =>
    request<NamingPreview>('/settings/media/preview', { method: 'POST', body: JSON.stringify(m) }),

  sources: () => request<Source[]>('/sources'),
  indexers: () => request<IndexerView[]>('/indexers'),
  addIndexer: (body: IndexerBody) =>
    request<IndexerView>('/indexers', { method: 'POST', body: JSON.stringify(body) }),
  updateIndexer: (iid: number, body: IndexerBody) =>
    request<IndexerView>(`/indexers/${iid}`, { method: 'PUT', body: JSON.stringify(body) }),
  patchIndexer: (iid: number, patch: IndexerPatch) =>
    request<IndexerView>(`/indexers/${iid}`, { method: 'PATCH', body: JSON.stringify(patch) }),
  deleteIndexer: (iid: number) => request<void>(`/indexers/${iid}`, { method: 'DELETE' }),
  testIndexer: (iid: number) => request<void>(`/indexers/${iid}/test`, { method: 'POST' }),
  browseProwlarr: (body: BrowseProwlarrBody) =>
    request<ProwlarrIndexer[]>('/indexers/browse', { method: 'POST', body: JSON.stringify(body) }),
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
    body: { sourceId: string; sourceMangaKey: string; sourceName?: string; mangaTitle?: string },
  ) =>
    request<{ id: number }>(`/series/${id}/bindings`, { method: 'POST', body: JSON.stringify(body) }),
  setBindingState: (id: number, bid: number, state: 'active' | 'disabled') =>
    request<void>(`/series/${id}/bindings/${bid}`, {
      method: 'PUT',
      body: JSON.stringify({ state }),
    }),
  deleteBinding: (id: number, bid: number) =>
    request<void>(`/series/${id}/bindings/${bid}`, { method: 'DELETE' }),

  search: (
    id: number,
    opts: {
      ledger: Ledger
      dryRun?: boolean
      grab?: string
      force?: boolean
      slot?: TargetSlot
    },
  ) =>
    request<SearchResponse>(`/series/${id}/search`, {
      method: 'POST',
      body: JSON.stringify(opts),
    }),
  // Streaming dry-run search: provider batches arrive as they complete.
  searchStream: async (
    id: number,
    opts: { ledger: Ledger; slot?: TargetSlot },
    onEvent: (ev: SearchEvent) => void,
  ) => {
    const res = await fetch(`${base}/series/${id}/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...opts, dryRun: true, stream: true }),
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
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      let nl
      while ((nl = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, nl).trim()
        buf = buf.slice(nl + 1)
        if (line) onEvent(JSON.parse(line) as SearchEvent)
      }
    }
  },
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
    request<{ seriesId: number; scan: ScanResult | null }>('/library/import', {
      method: 'POST',
      body: JSON.stringify({ dir, mangabakaId }),
    }),

  queue: (all = false) => request<QueueItem[]>(`/queue${all ? '?all=1' : ''}`),
  processQueue: () => request<{ processed: number }>('/queue/process', { method: 'POST' }),
  blocklistQueueItem: (qid: number) =>
    request<void>(`/queue/${qid}/blocklist`, { method: 'POST' }),

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
