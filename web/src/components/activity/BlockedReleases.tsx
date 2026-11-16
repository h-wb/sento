import { Button, Checkbox, Group, Pagination, Stack, Text, TextInput } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../../api'
import { plural } from '../../format'
import { notify } from '../../ui'
import classes from '../Activity.module.css'
import { activityTime } from '../activityModel'
import { useConfirmAction } from '../ConfirmAction'
import { QueryFeedback } from '../QueryFeedback'
import { usePaged } from '../usePaged'

export function BlockedReleases() {
  const qc = useQueryClient()
  const { confirm, confirmation } = useConfirmAction()
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [filter, setFilter] = useState('')
  const q = useQuery({ queryKey: ['blocklist'], queryFn: api.blocklist })
  const unblock = useMutation({
    mutationFn: (ids: number[] | 'all') =>
      ids === 'all' ? api.unblockAll() : api.unblockReleases({ ids }),
    onError: notify.err,
    onSuccess: (_, ids) => {
      notify.ok(ids !== 'all' && ids.length === 1 ? 'Release unblocked' : 'Releases unblocked')
      setSelected(new Set())
      void qc.invalidateQueries({ queryKey: ['blocklist'] })
    },
  })
  const entries = q.data ?? []
  const shown = entries.filter((b) =>
    `${b.title ?? ''} ${b.key} ${b.reason}`.toLowerCase().includes(filter.trim().toLowerCase()),
  )
  const { page, setPage, pages, slice: visible } = usePaged(shown, 25)
  const selectedIDs = entries.filter((b) => selected.has(b.id)).map((b) => b.id)
  const toggle = (ids: number[], checked: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      for (const id of ids) {
        if (checked) next.add(id)
        else next.delete(id)
      }
      return next
    })
  return (
    <Stack gap="xs" mt="sm">
      <QueryFeedback pending={q.isPending} error={q.error} onRetry={() => void q.refetch()} />
      {entries.length > 0 && (
        <>
          <Group justify="space-between">
            <Text size="sm">
              {entries.length} blocked {plural(entries.length, 'release')}
            </Text>
            <Group gap="xs">
              <Button
                size="compact-xs"
                variant="default"
                disabled={!selectedIDs.length || unblock.isPending}
                onClick={() => unblock.mutate(selectedIDs)}
              >
                Unblock selected ({selectedIDs.length})
              </Button>
              <Button
                size="compact-xs"
                variant="default"
                disabled={unblock.isPending}
                onClick={() =>
                  confirm({
                    title: `Unblock all ${entries.length} ${plural(entries.length, 'release')}?`,
                    message:
                      'This clears every block, including manual blocks and releases outside the current filters. These releases become eligible for normal searches and automatic acquisition again. Previous failed attempts stay in History.',
                    label: 'Unblock all',
                    onConfirm: () => unblock.mutateAsync('all'),
                  })
                }
              >
                Unblock all
              </Button>
            </Group>
          </Group>
          <TextInput
            size="xs"
            aria-label="Filter blocked releases"
            placeholder="Filter by release or reason"
            value={filter}
            onChange={(event) => {
              setFilter(event.currentTarget.value)
              setPage(1)
            }}
          />
          {visible.length > 0 && (
            <Checkbox
              size="xs"
              label="Select all releases on this page"
              disabled={unblock.isPending}
              checked={visible.every((b) => selected.has(b.id))}
              indeterminate={
                visible.some((b) => selected.has(b.id)) && !visible.every((b) => selected.has(b.id))
              }
              onChange={(event) =>
                toggle(
                  visible.map((b) => b.id),
                  event.currentTarget.checked,
                )
              }
            />
          )}
          {shown.length === 0 && (
            <Text size="sm" c="dimmed">
              No blocked releases match this filter.
            </Text>
          )}
        </>
      )}
      {q.data?.length === 0 && (
        <Text size="sm" c="dimmed">
          No blocked releases.
        </Text>
      )}
      {visible.map((b) => (
        <Group key={b.id} justify="space-between" align="flex-start" wrap="nowrap">
          <Checkbox
            size="xs"
            aria-label={`Select ${b.title || b.key}`}
            checked={selected.has(b.id)}
            disabled={unblock.isPending}
            onChange={(event) => toggle([b.id], event.currentTarget.checked)}
          />
          <div style={{ minWidth: 0, flex: 1 }}>
            <Text size="xs" className={classes.title}>
              {b.title || b.key}
            </Text>
            <Text size="xs" c="dimmed">
              {b.reason}
            </Text>
            <Text size="xs" c="dimmed">
              {new Date(activityTime(b.createdAt)).toLocaleString()}
            </Text>
          </div>
          <Button
            size="compact-xs"
            variant="default"
            disabled={unblock.isPending}
            onClick={() => unblock.mutate([b.id])}
          >
            Unblock
          </Button>
        </Group>
      ))}
      {pages > 1 && <Pagination size="sm" total={pages} value={page} onChange={setPage} />}
      {confirmation}
    </Stack>
  )
}
