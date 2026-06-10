// Typed client for the Sento API. All payload types are generated from the
// Go structs (mise run gen) into ./types/ — the backend is the schema.

import type { ChapterSlot, VolumeSlot } from './types/ledger'
import type { SeriesRef } from './types/metadata'
import type {
  ChapterLedgerPatch,
  IndexerSettingPatch,
  OverridePatch,
  SearchResponse,
  SeriesDetail,
  SlotsResponse,
  StatusResponse,
} from './types/server'
import type {
  AddSeriesRequest,
  ReleaseCandidate,
  ScanResult,
  SearchEvent,
  SeriesSiteView,
  SiteView,
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
import type { Manga as SourceManga, MangaPage, Source } from './types/suwayomi'

export type {
  Binding,
  BlocklistEntry,
  CalendarEntry,
  ChapterLedger,
  ChapterSlot,
  Collection,
  Decision,
  QueueItem,
  ReleaseCandidate,
  ScanResult,
  SearchEvent,
  SearchResponse,
  Series,
  SeriesDetail,
  SeriesFile,
  SeriesSiteView,
  SiteView,
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
  refreshSeries: (id: number) => request<unknown>(`/series/${id}/refresh`, { method: 'POST' }),
  slots: (id: number) => request<Slots>(`/series/${id}/slots`),
  setCollectionMonitored: (id: number, cid: number, monitored: boolean) =>
    request<unknown>(`/series/${id}/collections/${cid}`, {
      method: 'PUT',
      body: JSON.stringify({ monitored }),
    }),
  updateChapterLedger: (id: number, body: ChapterLedgerPatch) =>
    request<ChapterLedger>(`/series/${id}/chapter-ledger`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  files: (id: number) => request<SeriesFile[]>(`/series/${id}/files`),

  sources: () => request<Source[]>('/sources'),
  indexerSettings: () => request<SiteView[]>('/settings/indexers'),
  updateIndexerSetting: (provider: string, siteId: string, patch: IndexerSettingPatch) =>
    request<SiteView>(`/settings/indexers/${provider}/${encodeURIComponent(siteId)}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  seriesIndexers: (id: number) => request<SeriesSiteView[]>(`/series/${id}/indexers`),
  setSeriesIndexerOverride: (id: number, provider: string, siteId: string, ov: OverridePatch) =>
    request<unknown>(`/series/${id}/indexers/${provider}/${encodeURIComponent(siteId)}`, {
      method: 'PUT',
      body: JSON.stringify(ov),
    }),
  sourceSearch: (sid: string, q: string) =>
    request<MangaPage>(`/sources/${sid}/search?q=${encodeURIComponent(q)}`),
  bindings: (id: number) => request<Binding[]>(`/series/${id}/bindings`),
  addBinding: (id: number, body: { sourceId: string; sourceMangaId: number; sourceName?: string }) =>
    request<{ id: number }>(`/series/${id}/bindings`, { method: 'POST', body: JSON.stringify(body) }),
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
