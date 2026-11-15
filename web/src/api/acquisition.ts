import type { BlocklistEntry, Decision } from '../types/store'
import type {
  SearchEvent,
  SearchScope,
  ActivityData,
} from '../types/service'
import type { SearchBody, SearchResponse, UnblockBody } from '../types/server'
import { base, readError, request, type Ledger } from './request'

export const acquisitionApi = {
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
    if (!res.ok || !res.body) throw new Error(await readError(res))
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
  retryQueue: (id: number, remoteId = '') =>
    request<void>(`/queue/${id}/retry`, { method: 'POST', body: JSON.stringify({ remoteId }) }),
  cancelQueue: (id: number) => request<void>(`/queue/${id}/cancel`, { method: 'POST' }),
  tryAnotherRelease: (ids: number[]) => request<void>('/queue/another', { method: 'POST', body: JSON.stringify({ ids }) }),
  processQueue: () => request<{ processed: number }>('/queue/process', { method: 'POST' }),
  activity: (seriesId = 0) => request<ActivityData>(`/activity?seriesId=${seriesId}`),
  history: (seriesId?: number, limit = 100) =>
    request<Decision[]>(
      seriesId ? `/series/${seriesId}/history?limit=${limit}` : `/history?limit=${limit}`,
    ),
  blocklist: () => request<BlocklistEntry[]>('/blocklist'),
  unblockReleases: (body: UnblockBody) => request<void>('/blocklist/unblock', { method: 'POST', body: JSON.stringify(body) }),
  unblockAll: () => request<void>('/blocklist', { method: 'DELETE' }),
}
