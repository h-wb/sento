import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Group, Select, Stack, Text } from '@mantine/core'
import { api } from '../api'
import type { BookStatus, ReleaseFileMatch } from '../types/service'
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
      const [detail, versions, books] = await Promise.all([
        api.getSeries(seriesID),
        api.versions(seriesID),
        api.versionBooks(seriesID),
      ])
      return { editions: detail.editions, versions, books }
    },
  })
  const [versionID, setVersionID] = useState<string | null>(null)
  const edit = useEditor()
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
  const { editions, versions, books } = query.data
  const version = versions.find((t) => String(t.id) === versionID)
  const edition = version ? editions.find((c) => c.id === version.editionId) : undefined
  // What each file's own numbers say against the chosen card. The server
  // resolves authoritatively on save; this is the preview.
  const resolve = (f: ReviewItem): { label: string; left: boolean } => {
    if (!version || !edition) return { label: 'Pick a card first', left: true }
    if (!f.hint?.numbers?.length)
      return { label: 'Name does not say which books it holds', left: true }
    if (f.hint.ledger && f.hint.ledger !== edition.type)
      return { label: `Holds ${f.hint.ledger}s; this card collects ${edition.type}s`, left: true }
    const held = (books[String(version.id)] ?? []).filter((b: BookStatus) =>
      f.hint!.numbers!.includes(b.book.sequenceStr),
    )
    if (!held.length) return { label: `Numbers are not ${edition.type}s of this card`, left: true }
    const numbers = held.map((b: BookStatus) => b.book.sequenceStr).join(', ')
    return { label: `${edition.type} ${numbers}`, left: false }
  }
  const resolvable = version ? files.filter((f) => !resolve(f).left).length : 0
  const save = async () => {
    const matches = await api.matchRelease(
      seriesID,
      Number(versionID),
      files.map((f) => f.itemId),
    )
    const mapped = matches.filter((m: ReleaseFileMatch) => m.books.length > 0)
    const left = matches.filter((m: ReleaseFileMatch) => m.books.length === 0)
    if (mapped.length) notify.ok(`Matched ${mapped.length} of ${matches.length} files by their numbers`)
    if (left.length)
      notify.err(
        `${left.length} file(s) left for review: ${left
          .map((m: ReleaseFileMatch) => `${fileName(files.find((f) => f.itemId === m.fileId)?.title ?? '')} — ${m.note}`)
          .join('; ')}`,
      )
    await Promise.all(
      ['activity', 'review', 'queue', 'series'].map((key) => qc.invalidateQueries({ queryKey: [key] })),
    )
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
            {files.map((f) => {
              const p = resolve(f)
              return (
                <Group key={f.id} justify="space-between" gap="xs" wrap="nowrap">
                  <Text size="xs" style={{ overflowWrap: 'anywhere' }} title={f.title}>
                    {fileName(f.title)}
                  </Text>
                  <Text size="xs" c={p.left ? 'orange' : 'dimmed'} style={{ flexShrink: 0 }}>
                    {p.label}
                  </Text>
                </Group>
              )
            })}
          </Stack>
        )}
        {version && resolvable === 0 && (
          <Alert variant="light" color="orange">
            No file resolves against this card. Try another card, or review the files
            individually.
          </Alert>
        )}
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Group justify="flex-end">
          <Button
            loading={edit.busy}
            disabled={!version || resolvable === 0}
            onClick={() => void edit.run(save)}
          >
            Match {resolvable > 0 ? `${resolvable} of ${files.length} files` : ''}
          </Button>
        </Group>
      </Stack>
    </EditorModal>
  )
}
