import { useState } from 'react'
import { Alert, Button, Checkbox, Group, Modal, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { useOrganizeRun } from '../useOrganizeRun'
import OrganizeRunStatus from './OrganizeRunStatus'
import { QueryFeedback } from './QueryFeedback'

// Preview what the current Media settings would change in a series'
// existing files — where they live and the metadata inside them — and
// apply it as a background run, which closing the dialog does not stop.
export default function OrganizeFiles({
  seriesID,
  onClose,
}: {
  seriesID: number
  onClose: () => void
}) {
  const { status, busy, started, finished, apply } = useOrganizeRun([seriesID])
  const plan = useQuery({
    queryKey: ['series', seriesID, 'organize'],
    queryFn: () => api.planOrganize(seriesID),
    enabled: status.isSuccess && !busy && !started,
    gcTime: 0,
  })
  const [rename, setRename] = useState(true)
  const [retag, setRetag] = useState(true)
  const changes = plan.data ?? []
  const renames = changes.filter((c) => c.rename)
  const retags = changes.filter((c) => c.retag)
  const skipped = changes.filter((c) => c.skip)
  const selected = changes.filter((c) => (rename && c.rename) || (retag && c.retag))
  const previewing = !busy && !started
  return (
    <Modal opened onClose={onClose} title="Organize files" size="min(900px, 96vw)">
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          Bring this series' files in line with the current Media settings. Books keep their pages
          and identity; a download that is still seeding is not touched.
        </Text>
        <QueryFeedback
          pending={status.isPending || (previewing && plan.isPending)}
          error={status.error ?? plan.error}
          onRetry={() => void (status.error ? status.refetch() : plan.refetch())}
        />
        <OrganizeRunStatus status={status.data} busy={busy} finished={finished} />
        {previewing && plan.data && renames.length + retags.length === 0 && (
          <Text size="sm">Everything is already named and tagged as configured.</Text>
        )}
        {previewing && renames.length + retags.length > 0 && (
          <Group gap="lg">
            <Checkbox
              label={`Move and rename (${renames.length})`}
              checked={rename}
              disabled={renames.length === 0 || apply.isPending}
              onChange={(e) => setRename(e.currentTarget.checked)}
            />
            <Checkbox
              label={`Update embedded metadata (${retags.length})`}
              checked={retag}
              disabled={retags.length === 0 || apply.isPending}
              onChange={(e) => setRetag(e.currentTarget.checked)}
            />
          </Group>
        )}
        {apply.error && <Alert color="red">{String(apply.error)}</Alert>}
        {previewing && (
          <Stack gap={6} mah="50vh" style={{ overflowY: 'auto' }}>
            {selected.slice(0, 200).map((c) => (
              <div key={c.fileId}>
                <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                  {c.from}
                </Text>
                <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
                  {rename && c.rename ? `→ ${c.to}` : '→ same place'}
                  {retag && c.retag && (
                    <Text span size="xs" c="dimmed">
                      {' '}
                      · metadata updated
                    </Text>
                  )}
                </Text>
              </div>
            ))}
            {selected.length > 200 && (
              <Text size="xs" c="dimmed">
                and {selected.length - 200} more
              </Text>
            )}
          </Stack>
        )}
        {previewing && skipped.length > 0 && (
          <Text size="xs" c="dimmed">
            {skipped.length} {skipped.length === 1 ? 'file is' : 'files are'} left alone: not
            matched to a book yet, or not on disk.
          </Text>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Close
          </Button>
          {previewing && (
            <Button
              onClick={() => apply.mutate({ rename, retag })}
              loading={apply.isPending}
              disabled={selected.length === 0}
            >
              Apply to {selected.length} {selected.length === 1 ? 'file' : 'files'}
            </Button>
          )}
        </Group>
      </Stack>
    </Modal>
  )
}
