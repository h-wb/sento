import { useState } from 'react'
import { Alert, Button, Checkbox, Group, Modal, Stack, Text } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { notify } from '../ui'
import { QueryFeedback } from './QueryFeedback'

// Preview and apply what the current Media settings would change in a
// series' existing files: where they live and the metadata inside them.
export default function OrganizeFiles({
  seriesID,
  onClose,
}: {
  seriesID: number
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const plan = useQuery({
    queryKey: ['series', seriesID, 'organize'],
    queryFn: () => api.planOrganize(seriesID),
    gcTime: 0,
  })
  const [rename, setRename] = useState(true)
  const [retag, setRetag] = useState(true)
  const changes = plan.data ?? []
  const renames = changes.filter((c) => c.rename)
  const retags = changes.filter((c) => c.retag)
  const skipped = changes.filter((c) => c.skip)
  const selected = changes.filter((c) => (rename && c.rename) || (retag && c.retag))
  const apply = useMutation({
    mutationFn: () => api.organize(seriesID, { rename, retag }),
    onSuccess: async (r) => {
      await queryClient.invalidateQueries({ queryKey: ['series', seriesID] })
      if (r.errors.length === 0) {
        notify.ok(`${r.renamed} moved · ${r.retagged} updated`)
        onClose()
      }
    },
  })
  const failed = apply.data?.errors ?? []
  return (
    <Modal
      opened
      onClose={onClose}
      title="Organize files"
      size="min(900px, 96vw)"
      closeOnClickOutside={!apply.isPending}
    >
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          Bring this series' files in line with the current Media settings. Books keep their pages
          and identity; a download that is still seeding is not touched.
        </Text>
        <QueryFeedback
          pending={plan.isPending}
          error={plan.error}
          onRetry={() => void plan.refetch()}
        />
        {plan.data && renames.length + retags.length === 0 && (
          <Text size="sm">Everything is already named and tagged as configured.</Text>
        )}
        {renames.length + retags.length > 0 && (
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
        {failed.length > 0 && (
          <Alert
            color="orange"
            title={`${failed.length} ${failed.length === 1 ? 'file' : 'files'} could not be changed`}
          >
            {failed.map((e) => (
              <Text key={e} size="xs" style={{ overflowWrap: 'anywhere' }}>
                {e}
              </Text>
            ))}
          </Alert>
        )}
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
        {skipped.length > 0 && (
          <Text size="xs" c="dimmed">
            {skipped.length} {skipped.length === 1 ? 'file is' : 'files are'} left alone: not
            matched to a book yet, or not on disk.
          </Text>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose} disabled={apply.isPending}>
            Close
          </Button>
          <Button
            onClick={() => apply.mutate()}
            loading={apply.isPending}
            disabled={selected.length === 0}
          >
            Apply to {selected.length} {selected.length === 1 ? 'file' : 'files'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
