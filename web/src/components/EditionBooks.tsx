import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Badge,
  Button,
  Card,
  Checkbox,
  Group,
  Pagination,
  Select,
  Stack,
  Text,
} from '@mantine/core'
import { api } from '../api'
import type {
  Collection,
  DownloadTarget,
  Work,
  File as SeriesFile,
  AssetCoverage,
} from '../types/store'
import type { UnitSlot } from '../types/service'
import type { SlotSelection } from './SlotModal'
import { QueryFeedback } from './QueryFeedback'

const labels: Record<string, string> = {
  owned: 'Owned',
  pending: 'Pending',
  missing: 'Missing',
  not_released: 'Upcoming',
  unmonitored: 'Paused',
  unknown: 'Date unknown',
  upgrade_eligible: 'Upgrade wanted',
  unsatisfactory: 'Different copy on disk',
  needs_review: 'Needs review',
  storage_unavailable: 'Storage unavailable',
}
function bookSelection(c: Collection, t: DownloadTarget, s: UnitSlot): SlotSelection {
  return {
    ledger: c.type === 'chapter' ? 'chapter' : 'volume',
    collectionId: c.id,
    targetId: t.id,
    workId: s.work.id,
    number: s.work.sequenceNum,
    label: s.work.sequenceStr,
    collectionName: `${c.editionName} · ${t.name}`,
    fileIds: s.fileIds,
    monitored: s.monitored,
    wanted: s.wanted,
  }
}
export default function EditionBooks({
  seriesID,
  edition: c,
  target: t,
  files,
  coverage,
  onSlot,
  onEdit,
  onFile,
  run,
  busy,
}: {
  seriesID: number
  edition: Collection
  target: DownloadTarget
  files: SeriesFile[]
  coverage: AssetCoverage[]
  onSlot: (s: SlotSelection) => void
  onEdit: (w: Work) => void
  onFile: (f: SeriesFile) => void
  run: (fn: () => Promise<unknown>) => Promise<void>
  busy: boolean
}) {
  const [page, setPage] = useState(1)
  const [filter, setFilter] = useState('all')
  const q = useQuery({
    queryKey: ['series', seriesID, 'books', t.id, page, filter],
    queryFn: () => api.targetPage(seriesID, t.id, page, filter),
  })
  const books = q.data?.items ?? []
  return (
    <Stack gap="xs">
      <Group justify="space-between">
        <Select
          aria-label="Filter books"
          size="xs"
          w={200}
          value={filter}
          onChange={(v) => {
            setFilter(v ?? 'all')
            setPage(1)
          }}
          data={[
            { value: 'all', label: 'All books' },
            { value: 'wanted', label: 'Wanted' },
            { value: 'on_disk', label: 'On disk' },
            { value: 'review', label: 'Needs attention' },
            { value: 'unmonitored', label: 'Paused' },
          ]}
        />
        <Group gap="xs">
          <Button
            size="compact-xs"
            variant="subtle"
            disabled={busy || !books.length}
            onClick={() =>
              void run(() =>
                api.bulkMonitor(
                  seriesID,
                  t.id,
                  books.map((s) => s.work.id),
                  true,
                ),
              )
            }
          >
            Collect this page
          </Button>
          <Button
            size="compact-xs"
            variant="subtle"
            disabled={busy || !books.length}
            onClick={() =>
              void run(() =>
                api.bulkMonitor(
                  seriesID,
                  t.id,
                  books.map((s) => s.work.id),
                  false,
                ),
              )
            }
          >
            Pause this page
          </Button>
        </Group>
      </Group>
      <QueryFeedback pending={q.isPending} error={q.error} onRetry={() => void q.refetch()} />
      {!q.isPending && !q.error && !books.length && (
        <Text size="sm" c="dimmed">
          {filter === 'all'
            ? `No ${c.type === 'chapter' ? 'chapters' : 'volumes'} added yet.`
            : 'No books match this filter.'}
        </Text>
      )}
      {books.map((s) => (
        <Card key={s.work.id} withBorder padding="sm">
          <Group justify="space-between" align="flex-start">
            <Group gap="xs">
              <Checkbox
                aria-label={`Collect ${c.type} ${s.work.sequenceStr}`}
                checked={s.monitored}
                disabled={busy}
                onChange={(e) =>
                  void run(() =>
                    api.unitMonitor(seriesID, t.id, s.work.id, e.currentTarget.checked),
                  )
                }
              />
              <Button
                size="compact-sm"
                variant="subtle"
                onClick={() => onSlot(bookSelection(c, t, s))}
              >
                {s.work.sequenceStr}
              </Button>
              {s.work.countType !== 'main' && (
                <Text size="xs" c="dimmed">
                  {s.work.countType === 'extra' ? 'Bonus / special' : 'Other'}
                </Text>
              )}
            </Group>
            <Badge
              size="xs"
              color={s.fileIds.length ? 'green' : s.state === 'pending' ? 'blue' : 'gray'}
            >
              {labels[s.state] ?? s.state}
            </Badge>
          </Group>
          <Text size="xs" c="dimmed" mt={4}>
            {s.reason || s.work.releaseDate || 'Publication date unknown'}
          </Text>
          <Group gap="xs" mt={6}>
            <Button size="compact-xs" variant="subtle" onClick={() => onEdit(s.work)}>
              Edit
            </Button>
            {(s.onDiskFileIds ?? s.fileIds).length > 0 && (
              <Button
                size="compact-xs"
                variant="subtle"
                onClick={() => {
                  const f = files.find((f) => f.id === (s.onDiskFileIds ?? s.fileIds)[0])
                  if (f) onFile(f)
                }}
              >
                View files
              </Button>
            )}
            {s.fileIds.map((fid) => {
              const a = coverage.find(
                (a) => a.fileId === fid && a.targetId === t.id && a.workId === s.work.id,
              )
              return (
                a && (
                  <Button
                    key={fid}
                    size="compact-xs"
                    variant="subtle"
                    disabled={busy}
                    onClick={() =>
                      void run(() => api.pinCoverage(seriesID, { ...a, pinned: !a.pinned }))
                    }
                  >
                    {a.pinned ? 'Unpin copy' : 'Pin copy'}
                  </Button>
                )
              )
            })}
          </Group>
        </Card>
      ))}
      {(q.data?.pages ?? 1) > 1 && (
        <Pagination size="sm" total={q.data!.pages} value={q.data!.page} onChange={setPage} />
      )}
    </Stack>
  )
}
