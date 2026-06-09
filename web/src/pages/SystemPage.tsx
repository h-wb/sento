import { useEffect, useState } from 'react'
import { Alert, Badge, Card, Group, Text, Title } from '@mantine/core'
import { api, type SystemStatus } from '../api'

export default function SystemPage() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.status().then(setStatus).catch((e) => setError(String(e)))
  }, [])

  return (
    <>
      <Title order={2} mb="md">
        System
      </Title>
      {error && <Alert color="red">{error}</Alert>}
      {status && (
        <Card withBorder maw={420}>
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
    </>
  )
}
