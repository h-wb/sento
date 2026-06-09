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
  sourceSeenMax: number
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

export type ChapterCandidate = {
  slotNumber: number
  chapterId: number
  chapterName: string
  scanlator?: string
  sourceId: string
}

export type VolumeCandidate = {
  guid: string
  title: string
  indexer: string
  size: number
  seeders: number
  covered?: number[]
  score: number
  rejections?: string[]
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
  sourceSearch: (sid: string, q: string) =>
    request<{ mangas: SourceManga[] }>(`/sources/${sid}/search?q=${encodeURIComponent(q)}`),
  bindings: (id: number) => request<Binding[]>(`/series/${id}/bindings`),
  addBinding: (id: number, body: { sourceId: string; sourceMangaId: number; sourceName?: string }) =>
    request<{ id: number }>(`/series/${id}/bindings`, { method: 'POST', body: JSON.stringify(body) }),
  deleteBinding: (id: number, bid: number) =>
    request<void>(`/series/${id}/bindings/${bid}`, { method: 'DELETE' }),

  searchChapters: (id: number, dryRun: boolean) =>
    request<{ candidates: ChapterCandidate[]; queued: number[] }>(`/series/${id}/search/chapters`, {
      method: 'POST',
      body: JSON.stringify({ dryRun }),
    }),
  searchVolumes: (id: number, dryRun: boolean, grab?: string) =>
    request<{ candidates: VolumeCandidate[]; queued: number[] }>(`/series/${id}/search/volumes`, {
      method: 'POST',
      body: JSON.stringify({ dryRun, grab }),
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
