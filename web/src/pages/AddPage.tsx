import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  Group,
  Image,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { api, type LookupResult } from '../api'

export default function AddPage() {
  const [query, setQuery] = useState('')
  const [monitorAll, setMonitorAll] = useState(true)
  const navigate = useNavigate()

  const lookup = useMutation({ mutationFn: (q: string) => api.lookup(q) })
  const add = useMutation({
    mutationFn: (r: LookupResult) => api.addSeries({ mangabakaId: r.mangabakaId, monitorAll }),
    onSuccess: (s) => navigate(`/series/${s.id}`),
  })

  const search = () => query.trim() && lookup.mutate(query)
  const results = lookup.data
  const busy = lookup.isPending || add.isPending
  const error = lookup.error ?? add.error

  return (
    <>
      <Title order={2} mb="md">
        Add Series
      </Title>
      <Group mb="xs">
        <TextInput
          placeholder="Search MangaBaka…"
          leftSection={<IconSearch size={16} />}
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          onKeyDown={(e) => e.key === 'Enter' && search()}
          style={{ flex: 1, maxWidth: 480 }}
        />
        <Button onClick={search} loading={busy}>
          Search
        </Button>
      </Group>
      <Checkbox
        mb="md"
        label="Monitor all volume editions on add"
        checked={monitorAll}
        onChange={(e) => setMonitorAll(e.currentTarget.checked)}
      />
      {error && (
        <Alert color="red" mb="md">
          {String(error)}
        </Alert>
      )}
      <Stack>
        {results?.map((r) => (
          <Card key={r.mangabakaId} withBorder padding="sm">
            <Group wrap="nowrap" align="flex-start">
              <Image src={r.coverUrl} w={60} h={90} radius="sm" alt="" />
              <div style={{ flex: 1 }}>
                <Text fw={600}>{r.title}</Text>
                <Group gap={4} mt={4}>
                  <Badge size="xs">{r.type}</Badge>
                  <Badge size="xs" variant="light">
                    {r.status}
                  </Badge>
                  {r.year ? (
                    <Text size="xs" c="dimmed">
                      {r.year}
                    </Text>
                  ) : null}
                </Group>
              </div>
              <Button size="xs" onClick={() => add.mutate(r)} disabled={busy}>
                Add
              </Button>
            </Group>
          </Card>
        ))}
        {results?.length === 0 && <Text c="dimmed">No results.</Text>}
      </Stack>
    </>
  )
}
