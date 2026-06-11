import { useState } from 'react'
import {
  Anchor,
  Badge,
  Button,
  Center,
  Group,
  Loader,
  Stack,
  Switch,
  Table,
  Text,
  Title,
  Tooltip,
} from '@mantine/core'
import { IconDownload } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { fmtWhen, notify, usePageTitle } from '../ui'

const statusColor: Record<string, string> = {
  queued: 'gray',
  downloading: 'blue',
  importing: 'cyan',
  imported: 'green',
  failed: 'red',
}

export default function QueuePage() {
  usePageTitle('Activity')
  const [showAll, setShowAll] = useState(true)
  const queryClient = useQueryClient()

  const { data: items, error } = useQuery({
    queryKey: ['queue', showAll],
    queryFn: () => api.queue(showAll),
    refetchInterval: 5000,
  })
  const process = useMutation({
    mutationFn: api.processQueue,
    onSuccess: (r) => notify.ok(`Queue processed — ${r.processed} item(s) imported`),
    onError: notify.err,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['queue'] }),
  })
  const { data: blocked } = useQuery({ queryKey: ['blocklist'], queryFn: api.blocklist })
  const block = useMutation({
    mutationFn: (qid: number) => api.blocklistQueueItem(qid),
    onSuccess: () => notify.ok('Release blocklisted — it will never be grabbed again'),
    onError: notify.err,
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['queue'] })
      queryClient.invalidateQueries({ queryKey: ['blocklist'] })
    },
  })
  const unblock = useMutation({
    mutationFn: (id: number) => api.unblock(id),
    onSuccess: () => notify.ok('Unblocked'),
    onError: notify.err,
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
      {error && (
        <Text size="sm" c="red" mb="sm">
          {String(error)}
        </Text>
      )}

      {items && items.length === 0 ? (
        <Center mih={200}>
          <Stack align="center" gap="xs">
            <IconDownload size={36} opacity={0.4} />
            <Text c="dimmed">Nothing in the queue.</Text>
            <Text size="xs" c="dimmed">
              Grabs from searches and the scheduler appear here and import automatically.
            </Text>
          </Stack>
        </Center>
      ) : (
        <Table striped highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Title</Table.Th>
              <Table.Th>Protocol</Table.Th>
              <Table.Th>Status</Table.Th>
              <Table.Th>When</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {items?.map((i) => (
              <Table.Tr key={i.id}>
                <Table.Td>
                  <Group gap={6} wrap="nowrap">
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <Anchor component={Link} to={`/series/${i.seriesId}`} size="sm" c="inherit">
                        {i.title}
                      </Anchor>
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
                  <Badge
                    size="xs"
                    color={statusColor[i.status] ?? 'gray'}
                    leftSection={
                      i.status === 'downloading' || i.status === 'importing' ? (
                        <Loader size={8} color="white" />
                      ) : undefined
                    }
                  >
                    {i.status}
                  </Badge>
                </Table.Td>
                <Table.Td>
                  <Text size="xs" c="dimmed" title={i.createdAt}>
                    {fmtWhen(i.createdAt)}
                  </Text>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
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
