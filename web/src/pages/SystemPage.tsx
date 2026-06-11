import { Alert, Badge, Card, Group, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { usePageTitle } from '../ui'

export default function SystemPage() {
  usePageTitle('System')
  const { data: status, error } = useQuery({ queryKey: ['status'], queryFn: api.status })

  return (
    <>
      <Title order={2} mb="md">
        System
      </Title>
      {error && <Alert color="red">{String(error)}</Alert>}
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

      <Text size="sm" c="dimmed">
        Indexer configuration moved to the Indexers page.
      </Text>
    </>
  )
}
