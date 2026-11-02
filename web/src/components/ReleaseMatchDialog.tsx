import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Group, Select, Stack, Text } from '@mantine/core'
import { api } from '../api'
import type { ReleaseFileMatch } from '../types/service'
import type { CollectedVersion, Edition, ReviewItem } from '../types/store'
import { cardLabel, fileName } from './editionText'
import { EditorModal } from './EditorModal'
import { QueryFeedback } from './QueryFeedback'
import { useEditor } from './useEditor'
import { notify } from '../ui'

// Match a whole release at once: one edition (and version) choice, then
// each file maps by its own parsed numbers. Files whose numbers do not
// resolve stay for individual review — nothing is guessed.
export function ReleaseMatchDialog({
  seriesID,
  title,
  files,
  close,
}: {
  seriesID: number
  title: string
  files: ReviewItem[]
  close: () => void
}) {
  const qc = useQueryClient()
  const query = useQuery({
    queryKey: ['series', seriesID, 'release-match'],
    queryFn: async () => {
      const [detail, versions] = await Promise.all([
        api.getSeries(seriesID),
        api.versions(seriesID),
      ])
      return { editions: detail.editions, versions }
    },
  })
  const [versionID, setVersionID] = useState<string | null>(null)
  const edit = useEditor()
  const fileIDs = files.map((f) => f.itemId)
  const preview = useQuery({
    queryKey: ['series', seriesID, 'release-match', versionID, fileIDs],
    queryFn: () => api.previewReleaseMatch(seriesID, Number(versionID), fileIDs),
    enabled: versionID != null,
  })
  if (!query.data)
    return (
      <EditorModal title="Match whole release" dirty={false} busy={false} onClose={close}>
        <QueryFeedback
          pending={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
        />
      </EditorModal>
    )
  const { editions, versions } = query.data
  const version = versions.find((t) => String(t.id) === versionID)
  const resolvable = preview.data?.filter((m) => m.books.length > 0).length ?? 0
  const save = async () => {
    const matches = await api.matchRelease(
      seriesID,
      Number(versionID),
      fileIDs,
    ).finally(() => Promise.all(
      ['activity', 'review', 'queue', 'series'].map((key) => qc.invalidateQueries({ queryKey: [key] })),
    ))
    const mapped = matches.filter((m: ReleaseFileMatch) => m.books.length > 0)
    const left = matches.filter((m: ReleaseFileMatch) => m.books.length === 0)
    if (mapped.length) notify.ok(`Matched ${mapped.length} of ${matches.length} files by their numbers`)
    if (left.length)
      notify.err(
        `${left.length} file(s) left for review: ${left
          .map((m: ReleaseFileMatch) => `${fileName(files.find((f) => f.itemId === m.fileId)?.title ?? '')} — ${m.note}`)
          .join('; ')}`,
      )
    const warnings = mapped.filter((m) => m.note)
    if (warnings.length) notify.err(warnings.map((m) => m.note).join('; '))
    close()
  }
  return (
    <EditorModal
      title="Match whole release"
      dirty={versionID != null}
      busy={edit.busy}
      onClose={close}
    >
      <Stack>
        <div>
          <Text size="sm" fw={500}>
            {title}
          </Text>
          <Text size="xs" c="dimmed">
            {files.length} files waiting for review
          </Text>
        </div>
        <Text size="sm">
          Pick the card this release collects. Each file is then mapped by its own numbers — a
          file whose numbers do not resolve is left for its own review, never guessed.
        </Text>
        {versions.length === 0 && (
          <Alert variant="light" color="orange">
            This series has no collected versions yet. Add a card to an edition first.
          </Alert>
        )}
        <Select
          label="Card"
          placeholder="Choose a card"
          data={versions.map((t: CollectedVersion) => ({
            value: String(t.id),
            label: cardLabel(editions.find((c: Edition) => c.id === t.editionId)!, t, versions),
          }))}
          value={versionID}
          onChange={setVersionID}
        />
        {version && (
          <Stack gap={4}>
            <QueryFeedback
              pending={preview.isPending}
              error={preview.error}
              onRetry={() => void preview.refetch()}
            />
            {files.map((f) => {
              const p = preview.data?.find((m) => m.fileId === f.itemId)
              return (
                <Group key={f.id} justify="space-between" gap="xs" wrap="nowrap">
                  <Text size="xs" style={{ overflowWrap: 'anywhere' }} title={f.title}>
                    {fileName(f.title)}
                  </Text>
                  <Text size="xs" c={p?.books.length ? 'dimmed' : 'orange'} style={{ flexShrink: 0 }}>
                    {p?.label || p?.note || ''}
                  </Text>
                </Group>
              )
            })}
          </Stack>
        )}
        {version && preview.data && resolvable === 0 && (
          <Alert variant="light" color="orange">
            No file resolves against this card. Try another card, or review the files
            individually.
          </Alert>
        )}
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Group justify="flex-end">
          <Button
            loading={edit.busy}
            disabled={!version || preview.isFetching || !!preview.error || resolvable === 0}
            onClick={() => void edit.run(save)}
          >
            Match {resolvable > 0 ? `${resolvable} of ${files.length} files` : ''}
          </Button>
        </Group>
      </Stack>
    </EditorModal>
  )
}
