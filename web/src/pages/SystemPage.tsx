import { useEffect, useState } from 'react'
import { Alert, Badge, Card, Group, Table, Text, Title } from '@mantine/core'
import { api, type Indexer, type SystemStatus } from '../api'

export default function SystemPage() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [indexers, setIndexers] = useState<Indexer[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.status().then(setStatus).catch((e) => setError(String(e)))
    api.indexers().then(setIndexers).catch(() => setIndexers([]))
  }, [])

  return (
    <>
      <Title order={2} mb="md">
        System
      </Title>
      {error && <Alert color="red">{error}</Alert>}
      {status && (
        <Card withBorder maw={420} mb="lg">
          <Group justify="space-between">
            <Text>Version</Text>
            <Badge variant="light">{status.version}</Badge>
          </Group>
          <Group justify="space-between" mt="xs">
            <Text>Database</Text>
            <Badge color={status.database.ok ? 'green' : 'red'}>
              {status.database.ok ? 'ok' : 'down'}
            </Badge>
          </Group>
        </Card>
      )}

      <Title order={4} mb="xs">
        Indexers &amp; sources
      </Title>
      <Text size="sm" c="dimmed" mb="sm">
        Every site Sento can search, across all providers.
      </Text>
      {indexers && indexers.length === 0 && (
        <Text c="dimmed" size="sm">
          No providers configured — set Suwayomi and/or Prowlarr in the config.
        </Text>
      )}
      {indexers && indexers.length > 0 && (
        <Table maw={640} striped>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Site</Table.Th>
              <Table.Th>Provider</Table.Th>
              <Table.Th>Protocol</Table.Th>
              <Table.Th>Lang</Table.Th>
              <Table.Th>Enabled</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {indexers.map((i) => (
              <Table.Tr key={`${i.provider}-${i.id}`}>
                <Table.Td>
                  <Text size="sm">{i.name}</Text>
                </Table.Td>
                <Table.Td>
                  <Badge size="xs" variant="light">
                    {i.provider}
                  </Badge>
                </Table.Td>
                <Table.Td>
                  <Badge size="xs" variant="outline" color={i.protocol === 'torrent' ? 'blue' : 'teal'}>
                    {i.protocol}
                  </Badge>
                </Table.Td>
                <Table.Td>
                  <Text size="xs" c="dimmed">
                    {i.lang ?? '—'}
                  </Text>
                </Table.Td>
                <Table.Td>
                  <Badge size="xs" color={i.enabled ? 'green' : 'gray'}>
                    {i.enabled ? 'yes' : 'no'}
                  </Badge>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </>
  )
}
