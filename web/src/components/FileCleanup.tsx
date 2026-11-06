import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Checkbox,
  Group,
  Modal,
  Select,
  Stack,
  Tabs,
  Text,
} from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, fmtSize } from '../api'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'
import { fmtWhen, notify } from '../ui'

export default function FileCleanup({
  seriesID,
  editionID,
  onClose,
}: {
  seriesID: number
  editionID?: number
  onClose: () => void
}) {
  const qc = useQueryClient()
  const [tab, setTab] = useState<string | null>('preview')
  const [category, setCategory] = useState<string | null>('all')
  const [days, setDays] = useState<string | null>('30')
  const [selected, setSelected] = useState<number[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const { confirm, confirmation } = useConfirmAction()
  const preview = useQuery({
    queryKey: ['series', seriesID, 'cleanup', editionID ?? 0],
    queryFn: () => api.cleanupPreview(seriesID, editionID),
  })
  const trash = useQuery({
    queryKey: ['series', seriesID, 'trash'],
    queryFn: () => api.fileTrash(seriesID),
  })
  const candidates = preview.data ?? []
  const visible = candidates.filter((f) => category === 'all' || f.category === category)
  const eligible = visible.filter((f) => f.eligible)
  const chosen = candidates.filter((f) => selected.includes(f.fileId) && f.eligible)
  const total = chosen.reduce((sum, f) => sum + f.size, 0)
  const refresh = async () => {
    await Promise.all(
      ['series', 'activity', 'review', 'wanted'].map((key) =>
        qc.invalidateQueries({ queryKey: [key] }),
      ),
    )
  }
  const work = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError('')
    try {
      await fn()
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  const move = () =>
    confirm({
      title: `Move ${chosen.length} ${chosen.length === 1 ? 'file' : 'files'} to trash?`,
      message: `${fmtSize(total)} selected. These files will be kept for ${days} days, then permanently deleted by housekeeping. Download-client payloads stay in place; hardlinks can keep disk space in use. Collection settings stay as they are.`,
      label: 'Move to trash',
      onConfirm: async () => {
        setBusy(true)
        try {
          const moved: number[] = []
          const skipped: string[] = []
          for (let offset = 0; offset < chosen.length; offset += 25) {
            const result = await api.cleanupFiles(seriesID, {
              editionId: editionID ?? 0,
              fileIds: chosen.slice(offset, offset + 25).map((f) => f.fileId),
              retentionDays: Number(days),
            })
            moved.push(...result.moved)
            skipped.push(...result.skipped.map((f) => `File ${f.fileId}: ${f.reason}`))
            setSelected((ids) => ids.filter((id) => !result.moved.includes(id)))
          }
          setSelected([])
          setError(skipped.join('\n'))
          notify.ok(`${moved.length} ${moved.length === 1 ? 'file' : 'files'} moved to trash`)
        } finally {
          await refresh()
          setBusy(false)
        }
      },
    })
  return (
    <>
      <Modal
        opened
        title="Clean up files"
        size="xl"
        onClose={() => {
          if (!busy) onClose()
        }}
        closeOnEscape={!busy}
        closeOnClickOutside={!busy}
        withCloseButton={!busy}
      >
        <Stack>
          <Text size="sm" c="dimmed">
            Choose files to remove from your library. Shared files still selected by a collected
            version, pinned files and unfinished imports are protected. Turning Collect off keeps
            files until you choose to clean them up.
          </Text>
          <Tabs value={tab} onChange={setTab}>
            <Tabs.List>
              <Tabs.Tab value="preview">Preview</Tabs.Tab>
              <Tabs.Tab value="trash">Trash ({trash.data?.length ?? 0})</Tabs.Tab>
            </Tabs.List>
            <Tabs.Panel value="preview" pt="md">
              <Stack>
                <QueryFeedback
                  pending={preview.isPending}
                  error={preview.error}
                  onRetry={() => void preview.refetch()}
                />
                <Group>
                  <Select
                    aria-label="Cleanup category"
                    value={category}
                    onChange={setCategory}
                    data={[
                      { value: 'all', label: 'All files' },
                      { value: 'uncollected', label: 'Uncollected editions' },
                      { value: 'unmapped', label: 'Unmapped files' },
                      { value: 'unselected', label: 'Unselected copies' },
                    ]}
                    allowDeselect={false}
                  />
                  <Select
                    aria-label="Trash retention"
                    value={days}
                    onChange={setDays}
                    data={['7', '30', '90'].map((value) => ({
                      value,
                      label: `Keep in trash for ${value} days`,
                    }))}
                    allowDeselect={false}
                    disabled={busy}
                  />
                </Group>
                {eligible.length > 0 && (
                  <Checkbox
                    label="Select eligible files in this view"
                    disabled={busy}
                    checked={eligible.every((f) => selected.includes(f.fileId))}
                    indeterminate={
                      eligible.some((f) => selected.includes(f.fileId)) &&
                      !eligible.every((f) => selected.includes(f.fileId))
                    }
                    onChange={(e) =>
                      setSelected(
                        e.currentTarget.checked
                          ? [...new Set([...selected, ...eligible.map((f) => f.fileId)])]
                          : selected.filter((id) => !eligible.some((f) => f.fileId === id)),
                      )
                    }
                  />
                )}
                {visible.map((f) => (
                  <Group key={f.fileId} align="flex-start" wrap="nowrap" data-testid="cleanup-file">
                    <Checkbox
                      aria-label={`Select ${f.path.split('/').pop()}`}
                      disabled={!f.eligible || busy}
                      checked={selected.includes(f.fileId)}
                      onChange={(e) =>
                        setSelected(
                          e.currentTarget.checked
                            ? [...selected, f.fileId]
                            : selected.filter((id) => id !== f.fileId),
                        )
                      }
                      mt={4}
                    />
                    <Stack gap={2} style={{ minWidth: 0, flex: 1 }}>
                      <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
                        {f.path.split('/').pop()}
                      </Text>
                      <Text size="xs" c="dimmed">
                        {f.editions.join(' · ') || 'Unmapped'} · {fmtSize(f.size)}
                      </Text>
                      <Text size="xs" c={f.eligible ? 'dimmed' : 'orange'}>
                        {f.reason}
                      </Text>
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        {f.path}
                      </Text>
                    </Stack>
                    {!f.eligible && (
                      <Badge color="gray" size="xs">
                        Protected
                      </Badge>
                    )}
                  </Group>
                ))}
                {!preview.isPending && !preview.error && visible.length === 0 && (
                  <Text size="sm" c="dimmed">
                    No files in this view.
                  </Text>
                )}
                <Group justify="space-between">
                  <Text size="sm">
                    {chosen.length} selected · {fmtSize(total)}
                  </Text>
                  <Button color="red" disabled={chosen.length === 0 || busy} onClick={move}>
                    Move selected to trash
                  </Button>
                </Group>
              </Stack>
            </Tabs.Panel>
            <Tabs.Panel value="trash" pt="md">
              <Stack>
                <QueryFeedback
                  pending={trash.isPending}
                  error={trash.error}
                  onRetry={() => void trash.refetch()}
                />
                {trash.data?.map((f) => (
                  <Group key={f.fileId} align="flex-start" wrap="nowrap" data-testid="trashed-file">
                    <Stack gap={2} style={{ flex: 1, minWidth: 0 }}>
                      <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
                        {f.path.split('/').pop()}
                      </Text>
                      <Text size="xs" c="dimmed">
                        {fmtSize(f.size)} · {f.pending ? 'Trash move pending; deletion is paused' : `Expires ${fmtWhen(f.purgeAfter)}`}
                      </Text>
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        {f.path}
                      </Text>
                    </Stack>
                    <Group gap="xs">
                      <Button
                        size="compact-xs"
                        variant="default"
                        disabled={busy}
                        onClick={() => void work(() => api.restoreTrash(seriesID, f.fileId))}
                      >
                        Restore
                      </Button>
                      <Button
                        size="compact-xs"
                        color="red"
                        variant="subtle"
                        disabled={busy || f.pending}
                        onClick={() =>
                          confirm({
                            title: 'Permanently delete this file?',
                            message:
                              'The library file and its saved mappings will be removed. This cannot be undone. Download-client payloads stay in place.',
                            label: 'Delete permanently',
                            onConfirm: async () => {
                              await api.purgeTrash(seriesID, f.fileId)
                              await refresh()
                            },
                          })
                        }
                      >
                        Delete permanently
                      </Button>
                    </Group>
                  </Group>
                ))}
                {!trash.isPending && !trash.error && trash.data?.length === 0 && (
                  <Text size="sm" c="dimmed">
                    Trash is empty.
                  </Text>
                )}
              </Stack>
            </Tabs.Panel>
          </Tabs>
          {error && (
            <Alert color="red" style={{ whiteSpace: 'pre-wrap' }}>
              {error}
            </Alert>
          )}
        </Stack>
      </Modal>
      {confirmation}
    </>
  )
}
