// Typed client for the Sento API (specs/data-api.md §2).

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

export type SystemStatus = {
  app: string
  version: string
  database: { ok: boolean }
}

export type LookupResult = {
  mangabakaId: number
  title: string
  year?: number
  type: string
  status: string
  coverUrl?: string
}

export type Series = {
  id: number
  mangabakaId: number
  title: string
  type: string
  status: string
  year?: number
  totalChapters: number
  finalVolume: number
  description?: string
  coverUrl?: string
  monitored: boolean
  path?: string
  addedAt: string
}

export type Collection = {
  id: number
  seriesId: number
  editionName: string
  language: string
  publisher: string
  medium: string
  type: string
  monitored: boolean
  countMain: number
}

export type ChapterLedger = {
  seriesId: number
  monitored: boolean
  language: string
  cutoffNum: number
}

export type VolumeSlot = {
  number: number
  sequenceStr: string
  countType: string
  releaseDate?: string
  monitored: boolean
  released: boolean
  fileIds?: number[]
  wanted: boolean
}

export type ChapterSlot = {
  number: number
  monitored: boolean
  fileIds?: number[]
  wanted: boolean
}

export type Slots = {
  volumes: Record<string, VolumeSlot[]>
  chapters?: ChapterSlot[]
}

export type Source = { id: string; name: string; lang: string }

export type SourceManga = {
  id: number
  title: string
  url: string
  sourceId: string
}

export type Binding = {
  id: number
  seriesId: number
  sourceId: string
  sourceMangaId: number
  sourceName: string
  state: string
  priority: number
}

export type QueueItem = {
  id: number
  seriesId: number
  protocol: string
  title: string
  status: string
  error?: string
  createdAt: string
}

export type Ledger = 'volume' | 'chapter'

export type ReleaseCandidate = {
  guid: string
  provider: string
  protocol: 'torrent' | 'source'
  site: string
  siteId: string
  title: string
  size?: number
  seeders?: number
  chapterId?: number
  scanlator?: string
  covered?: number[]
  score: number
  rejections?: string[]
}

export type SearchResponse = {
  ledger: Ledger
  candidates: ReleaseCandidate[]
  queued: number[]
  searched: string[]
  dropped: number
  providerErrors?: string[]
  bindingNotes?: string[]
}

// One NDJSON frame of a streaming search (POST {stream: true}).
export type SearchEvent = {
  type: 'sites' | 'batch' | 'done' | 'error'
  ledger?: Ledger
  searched?: string[]
  provider?: string
  sites?: string[] // searched entries this batch covers
  candidates?: ReleaseCandidate[]
  dropped?: number
  error?: string
  bindingNotes?: string[]
}

export type SiteView = {
  provider: string
  siteId: string
  name: string
  protocol: 'torrent' | 'source'
  lang?: string
  reachable: boolean
  enabled: boolean
  useVolumes: boolean
  useChapters: boolean
  priority: number
}

export type SiteOverride = {
  enabled?: boolean | null
  useVolumes?: boolean | null
  useChapters?: boolean | null
}

export type SeriesSiteView = SiteView & {
  override?: SiteOverride | null
  binding?: Binding | null
}

export type SeriesFile = {
  id: number
  path: string
  size: number
  provenance: string
  unrequested: boolean
}

export const api = {
  status: () => request<SystemStatus>('/system/status'),
  lookup: (q: string) => request<LookupResult[]>(`/lookup?q=${encodeURIComponent(q)}`),

  listSeries: () => request<Series[]>('/series/'),
  addSeries: (body: { mangabakaId: number; monitorAll?: boolean; chapterCutoff?: number }) =>
    request<Series>('/series/', { method: 'POST', body: JSON.stringify(body) }),
  getSeries: (id: number) =>
    request<{ series: Series; collections: Collection[]; chapterLedger: ChapterLedger | null }>(
      `/series/${id}/`,
    ),
  deleteSeries: (id: number) => request<void>(`/series/${id}/`, { method: 'DELETE' }),
  refreshSeries: (id: number) => request<unknown>(`/series/${id}/refresh`, { method: 'POST' }),
  slots: (id: number) => request<Slots>(`/series/${id}/slots`),
  setCollectionMonitored: (id: number, cid: number, monitored: boolean) =>
    request<unknown>(`/series/${id}/collections/${cid}`, {
      method: 'PUT',
      body: JSON.stringify({ monitored }),
    }),
  updateChapterLedger: (id: number, body: Partial<ChapterLedger>) =>
    request<ChapterLedger>(`/series/${id}/chapter-ledger`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  files: (id: number) => request<SeriesFile[]>(`/series/${id}/files`),

  sources: () => request<Source[]>('/sources'),
  indexerSettings: () => request<SiteView[]>('/settings/indexers'),
  updateIndexerSetting: (
    provider: string,
    siteId: string,
    patch: { enabled?: boolean; useVolumes?: boolean; useChapters?: boolean; priority?: number },
  ) =>
    request<SiteView>(`/settings/indexers/${provider}/${encodeURIComponent(siteId)}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  seriesIndexers: (id: number) => request<SeriesSiteView[]>(`/series/${id}/indexers`),
  setSeriesIndexerOverride: (id: number, provider: string, siteId: string, ov: SiteOverride) =>
    request<unknown>(`/series/${id}/indexers/${provider}/${encodeURIComponent(siteId)}`, {
      method: 'PUT',
      body: JSON.stringify(ov),
    }),
  sourceSearch: (sid: string, q: string) =>
    request<{ mangas: SourceManga[] }>(`/sources/${sid}/search?q=${encodeURIComponent(q)}`),
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
      slot?: { collectionId: number; number: number }
    },
  ) =>
    request<SearchResponse>(`/series/${id}/search`, {
      method: 'POST',
      body: JSON.stringify(opts),
    }),
  // Streaming dry-run search: provider batches arrive as they complete.
  searchStream: async (
    id: number,
    opts: { ledger: Ledger; slot?: { collectionId: number; number: number } },
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
    request<{ scanned: number; linked: number; unrequested: number }>(`/series/${id}/import`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),

  queue: (all = false) => request<QueueItem[]>(`/queue${all ? '?all=1' : ''}`),
  processQueue: () => request<{ processed: number }>('/queue/process', { method: 'POST' }),
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
