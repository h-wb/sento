import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Select, Stack, Text } from '@mantine/core'
import { api } from '../api'
import type { ReviewItem } from '../types/store'
import { cardLabel } from './editionText'
import { EditorModal } from './EditorModal'
import { QueryFeedback } from './QueryFeedback'
import { ReviewChoices } from './ReviewChoices'

// Review one release together, keeping content confirmation separate from the
// selected copy and the disposition of an unused incoming archive.
export function ReleaseMatchDialog({ seriesID, title, files, close }: {
  seriesID: number
  title: string
  files: ReviewItem[]
  close: () => void
}) {
  const query = useQuery({
    queryKey: ['series', seriesID, 'release-match'],
    queryFn: async () => {
      const [detail, versions] = await Promise.all([api.getSeries(seriesID), api.versions(seriesID)])
      return { editions: detail.editions, versions }
    },
  })
  const [versionID, setVersionID] = useState<string | null>('0')
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  return (
    <EditorModal title="Match whole release" dirty={dirty || versionID !== '0'} busy={busy} onClose={close} size="lg">
      <Stack>
        <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>{title}</Text>
        <Text size="xs" c="dimmed">{files.length} files waiting for review</Text>
        <QueryFeedback pending={query.isPending} error={query.error} onRetry={() => void query.refetch()} />
        {query.data && <>
          <Select label="Card" value={versionID} disabled={busy} onChange={setVersionID}
            data={[
              { value: '0', label: 'Use proposed editions' },
              ...query.data.versions.map((t) => ({ value: String(t.id), label: cardLabel(query.data!.editions.find((c) => c.id === t.editionId)!, t, query.data!.versions) })),
            ]} />
          {files.length > 500 && <Text size="sm">Reviewing the first 500 files. The rest stay available for the next batch.</Text>}
          <ReviewChoices
            seriesID={seriesID}
            request={{ versionId: Number(versionID) || undefined, files: files.slice(0, 500).map((f) => ({ fileId: f.itemId, books: null })) }}
            applied={async () => close()}
            onDirtyChange={setDirty}
            onBusyChange={setBusy}
          />
        </>}
      </Stack>
    </EditorModal>
  )
}
