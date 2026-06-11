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
import { IconFolderSearch, IconSearch } from '@tabler/icons-react'
import { useMutation } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { api, type ImportProposal, type LookupResult } from '../api'

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

      <ImportSection />
    </>
  )
}

// ImportSection bulk-imports an existing library: scan a root folder,
// confirm the proposed MangaBaka matches, files link in place (D14).
function ImportSection() {
  const [root, setRoot] = useState('')
  const [done, setDone] = useState<Record<string, string>>({}) // dir → outcome

  const scan = useMutation({
    mutationFn: () => api.importScan(root || undefined),
    onSuccess: () => setDone({}),
  })
  const importOne = useMutation({
    mutationFn: (p: { dir: string; mangabakaId: number }) =>
      api.importFolder(p.dir, p.mangabakaId),
    onSuccess: (r, p) =>
      setDone((d) => ({
        ...d,
        [p.dir]: r.scan
          ? `imported — ${r.scan.linked} linked, ${r.scan.unrequested} unrequested`
          : 'imported',
      })),
    onError: (e, p) => setDone((d) => ({ ...d, [p.dir]: String(e) })),
  })

  const proposalLabel = (p: ImportProposal) => {
    const best = p.candidates?.find((c) => c.mangabakaId === p.bestId)
    if (p.bestId && !best) return `mangabaka:${p.bestId}` // folder-name hint
    if (best) return `${best.title}${best.year ? ` (${best.year})` : ''}`
    return 'no match'
  }

  return (
    <Stack mt="xl" gap="xs">
      <Title order={4}>Import existing library</Title>
      <Text size="sm" c="dimmed">
        Scan a folder of series directories: each is matched against MangaBaka and its files are
        linked in place — nothing is moved or renamed.
      </Text>
      <Group>
        <TextInput
          placeholder="Root folder (empty = configured library path)"
          value={root}
          onChange={(e) => setRoot(e.currentTarget.value)}
          style={{ flex: 1, maxWidth: 480 }}
        />
        <Button
          variant="default"
          leftSection={<IconFolderSearch size={16} />}
          loading={scan.isPending}
          onClick={() => scan.mutate()}
        >
          Scan
        </Button>
      </Group>
      {scan.error && <Alert color="red">{String(scan.error)}</Alert>}
      {scan.data?.length === 0 && <Text c="dimmed">No subfolders found.</Text>}
      <Stack gap={6}>
        {scan.data?.map((p) => (
          <Card key={p.dir} withBorder padding="xs">
            <Group justify="space-between" wrap="nowrap">
              <div style={{ minWidth: 0, flex: 1 }}>
                <Text size="sm" style={{ wordBreak: 'break-all' }}>
                  {p.dir}
                </Text>
                <Group gap={6} mt={2}>
                  {p.existingSeriesId ? (
                    <Badge size="xs" color="green" variant="light">
                      already in library
                    </Badge>
                  ) : (
                    <>
                      <Text size="xs" c="dimmed">
                        → {proposalLabel(p)}
                      </Text>
                      <Badge
                        size="xs"
                        color={p.confidence >= 0.9 ? 'green' : p.confidence >= 0.7 ? 'yellow' : 'red'}
                        variant="light"
                      >
                        {Math.round(p.confidence * 100)}%
                      </Badge>
                    </>
                  )}
                  {done[p.dir] && (
                    <Text size="xs" c={done[p.dir].startsWith('imported') ? 'green' : 'red'}>
                      {done[p.dir]}
                    </Text>
                  )}
                </Group>
              </div>
              {p.existingSeriesId ? (
                <Button
                  size="compact-xs"
                  variant="subtle"
                  component={Link}
                  to={`/series/${p.existingSeriesId}`}
                >
                  Open
                </Button>
              ) : (
                <Button
                  size="compact-xs"
                  disabled={!p.bestId || !!done[p.dir] || importOne.isPending}
                  onClick={() => p.bestId && importOne.mutate({ dir: p.dir, mangabakaId: p.bestId })}
                >
                  Import
                </Button>
              )}
            </Group>
          </Card>
        ))}
      </Stack>
    </Stack>
  )
}
