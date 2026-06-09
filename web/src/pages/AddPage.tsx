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
import { useNavigate } from 'react-router-dom'
import { api, type LookupResult } from '../api'

export default function AddPage() {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<LookupResult[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [monitorAll, setMonitorAll] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  const search = async () => {
    if (!query.trim()) return
    setBusy(true)
    setError(null)
    try {
      setResults(await api.lookup(query))
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  const add = async (r: LookupResult) => {
    setBusy(true)
    setError(null)
    try {
      const s = await api.addSeries({ mangabakaId: r.mangabakaId, monitorAll })
      navigate(`/series/${s.id}`)
    } catch (e) {
      setError(String(e))
      setBusy(false)
    }
  }

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
          {error}
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
              <Button size="xs" onClick={() => add(r)} disabled={busy}>
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
