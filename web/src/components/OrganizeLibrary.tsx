import { useState } from 'react'
import { Alert, Button, Checkbox, Group, Modal, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { useOrganizeRun } from '../useOrganizeRun'
import OrganizeRunStatus from './OrganizeRunStatus'
import { QueryFeedback } from './QueryFeedback'

// Preview and apply the current Media settings across many series at once.
// The run happens in the background; closing the dialog does not stop it.
export default function OrganizeLibrary({
  seriesIds,
  onClose,
}: {
  // The series to organize; none means the whole library.
  seriesIds?: number[]
  onClose: () => void
}) {
  const { status, busy, started, finished, apply } = useOrganizeRun(seriesIds)
  const plan = useQuery({
    queryKey: ['library', 'organize', 'plan', seriesIds ?? []],
    queryFn: () => api.planOrganizeLibrary(seriesIds),
    enabled: status.isSuccess && !busy && !started,
    gcTime: 0,
  })
  const [rename, setRename] = useState(true)
  const [retag, setRetag] = useState(true)
  const plans = plan.data ?? []
  const changes = plans.flatMap((p) => p.changes)
  const renames = changes.filter((c) => c.rename).length
  const retags = changes.filter((c) => c.retag).length
  const selected = changes.filter((c) => (rename && c.rename) || (retag && c.retag)).length
  return (
    <Modal opened onClose={onClose} title="Organize files" size="min(900px, 96vw)">
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          Bring {seriesIds?.length ? 'the selected series’' : 'every series’'} files in line with the current Media settings. Books keep their pages and identity; a download
          that is still seeding is not touched.
        </Text>
        <QueryFeedback
          pending={status.isPending || (!busy && !started && plan.isPending)}
          error={status.error ?? plan.error}
          onRetry={() => void (status.error ? status.refetch() : plan.refetch())}
        />

        <OrganizeRunStatus status={status.data} busy={busy} finished={finished} />

        {!busy && !started && plan.data && (
          <>
            {plans.length === 0 ? (
              <Text size="sm">Everything is already named and tagged as configured.</Text>
            ) : (
              <Group gap="lg">
                <Checkbox
                  label={`Move and rename (${renames})`}
                  checked={rename}
                  disabled={renames === 0 || apply.isPending}
                  onChange={(e) => setRename(e.currentTarget.checked)}
                />
                <Checkbox
                  label={`Update embedded metadata (${retags})`}
                  checked={retag}
                  disabled={retags === 0 || apply.isPending}
                  onChange={(e) => setRetag(e.currentTarget.checked)}
                />
              </Group>
            )}
            {apply.error && <Alert color="red">{String(apply.error)}</Alert>}
            <Stack gap={6} mah="50vh" style={{ overflowY: 'auto' }}>
              {plans.map((p) => {
                const moved = p.changes.filter((c) => rename && c.rename).length
                const tagged = p.changes.filter((c) => retag && c.retag).length
                if (moved + tagged === 0) return null
                return (
                  <Group key={p.seriesId} justify="space-between" wrap="nowrap">
                    <Text size="sm" lineClamp={1}>
                      {p.title}
                    </Text>
                    <Text size="xs" c="dimmed" style={{ whiteSpace: 'nowrap' }}>
                      {[moved && `${moved} moved`, tagged && `${tagged} updated`]
                        .filter(Boolean)
                        .join(' · ')}
                    </Text>
                  </Group>
                )
              })}
            </Stack>
          </>
        )}

        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Close
          </Button>
          {!busy && !started && (
            <Button
              onClick={() => apply.mutate({ rename, retag })}
              loading={apply.isPending}
              disabled={selected === 0}
            >
              Apply to {selected} {selected === 1 ? 'file' : 'files'}
            </Button>
          )}
        </Group>
      </Stack>
    </Modal>
  )
}
