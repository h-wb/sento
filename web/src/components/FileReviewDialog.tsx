import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { ReviewItem } from '../types/store'
import { EditorModal } from './EditorModal'
import { MappingEditor } from './MappingEditor'
import { QueryFeedback } from './QueryFeedback'

export function FileReviewDialog({ item, close }: { item: ReviewItem; close: () => void }) {
  const qc = useQueryClient()
  const query = useQuery({
    queryKey: ['series', item.seriesId, 'file-review', item.itemId],
    queryFn: async () => {
      const [detail, files, versions, coverage, books] = await Promise.all([
        api.getSeries(item.seriesId),
        api.files(item.seriesId),
        api.versions(item.seriesId),
        api.coverage(item.seriesId),
        api.versionBooks(item.seriesId),
      ])
      const file = files.find((f) => f.id === item.itemId)
      if (!file)
        throw new Error('This file is no longer available. Refresh activity and try again.')
      return { detail, file, versions, coverage, books }
    },
  })
  if (!query.data)
    return (
      <EditorModal title="Match imported file" dirty={false} busy={false} onClose={close}>
        <QueryFeedback
          pending={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
        />
      </EditorModal>
    )
  const { detail, file, versions, coverage, books } = query.data
  return (
    <MappingEditor
      seriesID={item.seriesId}
      value={file}
      editions={detail.editions}
      versions={versions}
      existing={coverage.filter((c) => c.fileId === file.id)}
      books={Object.values(books).flat()}
      proposal={item.proposal}
      save={async (refs) => {
        await api.mapFile(item.seriesId, file.id, refs)
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
