import { Affix, Button, Divider, Group, Paper, Text, Transition } from '@mantine/core'
import { IconEye, IconEyeOff, IconFolders, IconTrash } from '@tabler/icons-react'
import type { Dispatch, SetStateAction } from 'react'

type BulkMonitor = {
  isPending: boolean
  mutate: (v: { ids: number[]; monitored: boolean }) => void
}

export function BulkBar({
  view,
  visibleSelected,
  bulkMonitor,
  setOrganizing,
  setRemoving,
  setSelected,
}: {
  view: 'grid' | 'table'
  visibleSelected: number[]
  bulkMonitor: BulkMonitor
  setOrganizing: Dispatch<SetStateAction<number[] | null>>
  setRemoving: Dispatch<SetStateAction<number[] | null>>
  setSelected: Dispatch<SetStateAction<Set<number>>>
}) {
  return (
    // Floating bulk action bar — slides up from the bottom on selection.
    <Affix position={{ bottom: 24, left: '50%' }} style={{ transform: 'translateX(-50%)' }}>
      <Transition transition="slide-up" mounted={view === 'table' && visibleSelected.length > 0}>
        {(styles) => (
          <Paper shadow="md" radius="xl" withBorder py="xs" pl="lg" pr="xs" style={styles}>
            <Group gap="sm" wrap="nowrap">
              <Text size="sm" fw={600}>
                {visibleSelected.length} selected
              </Text>
              <Divider orientation="vertical" />
              <Button
                size="xs"
                radius="xl"
                variant="light"
                leftSection={<IconEye size={14} />}
                loading={bulkMonitor.isPending}
                onClick={() => bulkMonitor.mutate({ ids: visibleSelected, monitored: true })}
              >
                Monitor
              </Button>
              <Button
                size="xs"
                radius="xl"
                variant="light"
                color="gray"
                leftSection={<IconEyeOff size={14} />}
                loading={bulkMonitor.isPending}
                onClick={() => bulkMonitor.mutate({ ids: visibleSelected, monitored: false })}
              >
                Unmonitor
              </Button>
              <Button
                size="xs"
                radius="xl"
                variant="light"
                color="gray"
                leftSection={<IconFolders size={14} />}
                disabled={bulkMonitor.isPending}
                onClick={() => setOrganizing(visibleSelected)}
              >
                Organize files
              </Button>
              <Button
                size="xs"
                radius="xl"
                variant="light"
                color="red"
                leftSection={<IconTrash size={14} />}
                disabled={bulkMonitor.isPending}
                onClick={() => setRemoving(visibleSelected)}
              >
                Delete
              </Button>
              <Button size="xs" radius="xl" variant="subtle" onClick={() => setSelected(new Set())}>
                Clear
              </Button>
            </Group>
          </Paper>
        )}
      </Transition>
    </Affix>
  )
}
