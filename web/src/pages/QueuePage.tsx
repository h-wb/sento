import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Group,
  Switch,
  Table,
  Text,
  Title,
  Tooltip,
} from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'

const statusColor: Record<string, string> = {
  queued: 'gray',
  downloading: 'blue',
  importing: 'cyan',
  imported: 'green',
  failed: 'red',
}

export default function QueuePage() {
  const [showAll, setShowAll] = useState(true)
  const queryClient = useQueryClient()

  const { data: items, error } = useQuery({
    queryKey: ['queue', showAll],
    queryFn: () => api.queue(showAll),
    refetchInterval: 5000,
  })
  const process = useMutation({
    mutationFn: api.processQueue,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['queue'] }),
  })
  const { data: blocked } = useQuery({ queryKey: ['blocklist'], queryFn: api.blocklist })
  const block = useMutation({
    mutationFn: (qid: number) => api.blocklistQueueItem(qid),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['queue'] })
      queryClient.invalidateQueries({ queryKey: ['blocklist'] })
    },
  })
  const unblock = useMutation({
    mutationFn: (id: number) => api.unblock(id),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['blocklist'] }),
  })

  return (
    <>
      <Group justify="space-between" mb="md">
        <Title order={2}>Activity</Title>
        <Group>
          <Switch
            label="Include finished"
            checked={showAll}
            onChange={(e) => setShowAll(e.currentTarget.checked)}
          />
          <Button size="xs" onClick={() => process.mutate()} loading={process.isPending}>
            Process now
          </Button>
        </Group>
      </Group>
      {(error || process.error) && (
        <Alert color="red" mb="md">
          {String(error ?? process.error)}
        </Alert>
      )}
      <Table striped highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Title</Table.Th>
            <Table.Th>Protocol</Table.Th>
            <Table.Th>Status</Table.Th>
            <Table.Th>Created</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {items?.map((i) => (
            <Table.Tr key={i.id}>
              <Table.Td>
                <Group gap={6} wrap="nowrap">
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <Text size="sm">{i.title}</Text>
                    {i.error && (
                      <Text size="xs" c="red">
                        {i.error}
                      </Text>
                    )}
                  </div>
                  {i.status === 'failed' && (
                    <Tooltip label="Blocklist this release (never grab it again)">
                      <Button
                        size="compact-xs"
                        variant="subtle"
                        color="orange"
                        onClick={() => block.mutate(i.id)}
                      >
                        Blocklist
                      </Button>
                    </Tooltip>
                  )}
                </Group>
              </Table.Td>
              <Table.Td>
                <Badge size="xs" variant="outline">
                  {i.protocol}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Badge size="xs" color={statusColor[i.status] ?? 'gray'}>
                  {i.status}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Text size="xs" c="dimmed">
                  {i.createdAt}
                </Text>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
      {items?.length === 0 && (
        <Text c="dimmed" mt="md">
          Queue is empty.
        </Text>
      )}

      {blocked && blocked.length > 0 && (
        <>
          <Title order={4} mt="xl" mb="xs">
            Blocklist
          </Title>
          <Table maw={860}>
            <Table.Tbody>
              {blocked.map((b) => (
                <Table.Tr key={b.id}>
                  <Table.Td>
                    <Text size="xs" style={{ wordBreak: 'break-all' }}>
                      {b.key}
                    </Text>
                    <Text size="xs" c="dimmed" lineClamp={1}>
                      {b.reason}
                    </Text>
                  </Table.Td>
                  <Table.Td w={90}>
                    <Badge size="xs" variant="outline">
                      {b.protocol}
                    </Badge>
                  </Table.Td>
                  <Table.Td w={90}>
                    <Button size="compact-xs" variant="subtle" onClick={() => unblock.mutate(b.id)}>
                      Unblock
                    </Button>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </>
      )}
    </>
  )
}
