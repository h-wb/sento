import { useEffect, useRef, useState } from 'react'
import { api, type ReleaseCandidate } from '../../api'
import type { AcquisitionPlan as Plan } from '../../types/service'
import { errorMessage } from '../../ui'
import type { BookSelection } from '../BookDialog'

// Accumulated state of one streaming search: batches land per provider.
export type StreamResult = {
  plan?: Plan
  queries: string[]
  searched: string[]
  pending: string[] // "provider/site" entries still searching
  candidates: ReleaseCandidate[]
  dropped: number
  notes: string[]
  errors: string[]
  done: boolean
  stopped?: boolean
}

export function applySearchEvent(prev: StreamResult | null, ev: Parameters<Parameters<typeof api.searchStream>[2]>[0]): StreamResult | null {
            if (!prev) return prev
            switch (ev.type) {
              case 'sites':
                return {
                  ...prev,
                  notes: ev.bindingNotes ?? [],
                  searched: ev.searched ?? [],
                  pending: ev.searched ?? [],
                }
              case 'batch':
                return {
                  ...prev,
                  queries: [...new Set([...prev.queries, ...(ev.queries ?? [])])],
                  pending: prev.pending.filter((p) => !(ev.sites ?? []).includes(p)),
                  candidates: [...prev.candidates, ...(ev.candidates ?? [])],
                  dropped: prev.dropped + (ev.dropped ?? 0),
                  notes: [...prev.notes, ...(ev.bindingNotes ?? [])],
                  errors: ev.error ? [...prev.errors, ev.error] : prev.errors,
                  plan: ev.plan ?? prev.plan,
                }
              case 'done':
                return { ...prev, pending: [], done: true, plan: ev.plan ?? prev.plan }
              case 'error':
                return {
                  ...prev,
                  pending: [],
                  done: true,
                  errors: [...prev.errors, ev.error ?? 'search failed'],
                }
              default:
                return prev
            }
}

export function useReleaseSearch(seriesID: number, sel: BookSelection, editionWide: boolean, allBooks: boolean, busy: boolean) {
  const searchController = useRef<AbortController | null>(null)
  useEffect(() => () => searchController.current?.abort(), [])
  const [searching, setSearching] = useState(false)
  const [result, setResult] = useState<StreamResult | null>(null)
  // Streaming search: each provider's batch renders as soon as it lands,
  // so fast extension sources show up while torrent indexers still load.
  const searchScope = (everyBook: boolean) => ({
    replaceQueueId: sel.replaceQueueId,
    versionId: sel.versionId,
    bookId: sel.bookId,
    all: sel.all,
    allBooks: editionWide && everyBook ? true : undefined,
  })
  const interactiveSearch = async (everyBook = allBooks) => {
    if (searching || busy) return
    const controller = new AbortController()
    searchController.current = controller
    setSearching(true)
    setResult({
      queries: [],
      searched: [],
      pending: [],
      candidates: [],
      dropped: 0,
      notes: [],
      errors: [],
      done: false,
    })
    try {
      await api.searchStream(
        seriesID,
        { ledger: sel.ledger, scope: searchScope(everyBook) },
        (ev) => {
          setResult((prev) => applySearchEvent(prev, ev))
        },
        controller.signal,
      )
    } catch (e) {
      if (controller.signal.aborted) setResult((prev) => (prev ? { ...prev, stopped: true } : prev))
      if (!controller.signal.aborted)
        setResult((prev) =>
          prev
            ? {
                ...prev,
                errors: [...prev.errors, errorMessage(e)],
              }
            : prev,
        )
    } finally {
      setSearching(false)
      setResult((prev) => (prev ? { ...prev, pending: [], done: true } : prev))
    }
  }

  // The interactive-search buttons open the modal already searching.
  // Deferred so the modal paints first (and StrictMode's double-invoke
  // cancels via cleanup).
  useEffect(() => {
    if (!sel.autoSearch) return
    const id = setTimeout(() => void interactiveSearch(), 0)
    return () => clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return { searchController, searching, result, searchScope, interactiveSearch }
}
