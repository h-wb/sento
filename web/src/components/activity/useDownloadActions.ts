import type { Dispatch, SetStateAction } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../api'
import type { ActivityDownload } from '../../types/service'
import { notify } from '../../ui'
import type { BookSelection } from '../BookDialog'
import { replacementSearch } from '../bookSelection'
import type { Confirm } from '../ConfirmAction'
import type { DownloadActions } from '../DownloadActivityRow'

export function useDownloadActions({
  seriesID,
  onSources,
  mutation,
  confirm,
  refresh,
  setAttach,
  setRemoteID,
  setRecovery,
}: {
  seriesID: number
  onSources?: () => void
  mutation: { isPending: boolean; mutate: (work: () => Promise<unknown>) => void }
  confirm: Confirm
  refresh: () => Promise<void>
  setAttach: Dispatch<SetStateAction<ActivityDownload | null>>
  setRemoteID: Dispatch<SetStateAction<string>>
  setRecovery: Dispatch<SetStateAction<{ seriesID: number; sel: BookSelection } | null>>
}) {
  const navigate = useNavigate()
  const another = (ids: number[]) =>
    mutation.mutate(async () => {
      await api.tryAnotherRelease(ids)
      notify.ok('Looking for other releases')
    })
  const actions: DownloadActions = {
    busy: mutation.isPending,
    sources: (sid) => {
      if (sid === seriesID && onSources) onSources()
      else navigate(`/series/${sid}?sources=1`)
    },
    another,
    retry: (ids) =>
      mutation.mutate(async () => {
        for (const id of Array.isArray(ids) ? ids : [ids]) await api.retryQueue(id)
        notify.ok('Downloads will be checked again')
      }),
    recheck: (id) => mutation.mutate(() => api.checkFiles(id)),
    attach: (d) => {
      setAttach(d)
      setRemoteID('')
    },
    cancel: (d) =>
      confirm({
        title: 'Stop tracking this download?',
        message:
          'Release its book reservations. Files in your library stay. Torrents and their files remain in the download client.',
        label: 'Stop tracking',
        onConfirm: async () => {
          await api.cancelQueue(d.job.id)
          await refresh()
        },
      }),
    replace: (d) => {
      const book = d.unresolvedBooks?.[0] ?? d.job.books?.[0]
      const sel = replacementSearch(book, d.job)
      if (!sel) return
      setRecovery({
        seriesID: d.job.seriesId,
        sel,
      })
    },
  }
  return actions
}
