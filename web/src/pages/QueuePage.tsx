import { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Group,
  Switch,
  Table,
  Text,
  Title,
} from '@mantine/core'
import { api, type QueueItem } from '../api'

const statusColor: Record<string, string> = {
  queued: 'gray',
  downloading: 'blue',
  importing: 'cyan',
  imported: 'green',
  failed: 'red',
}

export default function QueuePage() {
  const [items, setItems] = useState<QueueItem[] | null>(null)
  const [showAll, setShowAll] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api.queue(showAll).then(setItems).catch((e) => setError(String(e)))
  }, [showAll])

  useEffect(() => {
    load()
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [load])

  const process = async () => {
    setBusy(true)
    try {
      await api.processQueue()
      load()
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

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
          <Button size="xs" onClick={process} loading={busy}>
            Process now
          </Button>
        </Group>
      </Group>
      {error && (
        <Alert color="red" mb="md">
          {error}
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
                <Text size="sm">{i.title}</Text>
                {i.error && (
                  <Text size="xs" c="red">
                    {i.error}
                  </Text>
                )}
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
    </>
  )
}
