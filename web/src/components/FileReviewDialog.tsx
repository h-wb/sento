import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Text } from '@mantine/core'
import { api } from '../api'
import type { ReviewItem } from '../types/store'
import { EditorModal } from './EditorModal'
import { MappingEditor } from './MappingEditor'
import { QueryFeedback } from './QueryFeedback'

export function FileReviewDialog({ item, close }: { item: ReviewItem; close: () => void }) {
  const qc = useQueryClient()
  // Shared series caches, so opening from the series page is instant.
  const detail = useQuery({ queryKey: ['series', item.seriesId], queryFn: () => api.getSeries(item.seriesId) })
  const files = useQuery({ queryKey: ['series', item.seriesId, 'files'], queryFn: () => api.files(item.seriesId) })
  const versions = useQuery({ queryKey: ['series', item.seriesId, 'versions'], queryFn: () => api.versions(item.seriesId) })
  const coverage = useQuery({ queryKey: ['series', item.seriesId, 'coverage'], queryFn: () => api.coverage(item.seriesId) })
  const books = useQuery({ queryKey: ['series', item.seriesId, 'version-books'], queryFn: () => api.versionBooks(item.seriesId) })
  const queries = [detail, files, versions, coverage, books]
  const file = files.data?.find((f) => f.id === item.itemId)
  const error =
    queries.find((q) => q.error)?.error ??
    (files.data && !file ? new Error('This file is no longer available. Refresh activity and try again.') : undefined)
  const query = {
    isPending: queries.some((q) => q.isPending),
    error,
    refetch: () => Promise.all(queries.map((q) => q.refetch())),
    data:
      detail.data && files.data && file && versions.data && coverage.data && books.data
        ? { detail: detail.data, file, versions: versions.data, coverage: coverage.data, books: books.data }
        : undefined,
  }
  if (!query.data)
    return (
      <EditorModal title="Match imported file" dirty={false} busy={false} onClose={close}>
        {query.isPending && <Text size="sm" c="dimmed">Loading the file, its copies and the series’ books…</Text>}
        <QueryFeedback
          pending={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
        />
      </EditorModal>
    )
  const { detail: d, file: target, versions: vs, coverage: cov, books: bk } = query.data
  return (
    <MappingEditor
      seriesID={item.seriesId}
      value={target}
      editions={d.editions}
      versions={vs}
      existing={cov.filter((c) => c.fileId === target.id)}
      books={Object.values(bk).flat()}
      proposal={item.proposal}
      hint={item.hint}
      applied={async () => {
        await Promise.all(
          ['activity', 'review', 'queue', 'series'].map((key) =>
            qc.invalidateQueries({ queryKey: [key] }),
          ),
        )
        close()
      }}
      close={close}
    />
  )
}
