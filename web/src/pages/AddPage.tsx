import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Image,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { IconFolderSearch, IconSearch } from '@tabler/icons-react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { api, type ImportProposal, type LookupResult } from '../api'
import { notify, usePageTitle } from '../ui'

export default function AddPage() {
  usePageTitle('Add Series')
  const [query, setQuery] = useState('')
  const [monitor, setMonitor] = useState('default')
  const [profileId, setProfileId] = useState(0) // 0 = instance default
  const navigate = useNavigate()
  const { data: profiles } = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })

  // Known MangaBaka ids, so results already in the library link instead
  // of offering a duplicate add.
  const { data: existing } = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const inLibrary = new Map((existing ?? []).map((s) => [s.mangabakaId, s.id]))

  const lookup = useMutation({ mutationFn: (q: string) => api.lookup(q), onError: notify.err })
  const add = useMutation({
    mutationFn: (r: LookupResult) => api.addSeries({ mangabakaId: r.mangabakaId, monitor, profileId }),
    onSuccess: (s) => {
      notify.ok(`Added "${s.title}"`)
      navigate(`/series/${s.id}`)
    },
    onError: notify.err,
  })

  const search = () => query.trim() && lookup.mutate(query)
  const results = lookup.data
  const busy = lookup.isPending || add.isPending

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
          data-autofocus
          autoFocus
        />
        <Button onClick={search} loading={busy}>
          Search
        </Button>
      </Group>
      <Select
        mb="md"
        w={300}
        size="xs"
        label="Monitor editions on add"
        description="Changeable later per series under Volumes → Editions"
        allowDeselect={false}
        value={monitor}
        onChange={(v) => v && setMonitor(v)}
        data={[
          { value: 'default', label: 'Default edition (standard, preferred language)' },
          { value: 'all', label: 'All volume editions' },
          { value: 'none', label: 'None' },
        ]}
      />
      {(profiles?.length ?? 0) > 1 && (
        <Select
          mb="md"
          w={300}
          size="xs"
          label="Release profile"
          allowDeselect={false}
          value={String(profileId)}
          onChange={(v) => v && setProfileId(Number(v))}
          data={[
            { value: '0', label: 'Instance default' },
            ...(profiles ?? []).map((p) => ({ value: String(p.id), label: p.name })),
          ]}
        />
      )}
      <Stack>
        {results?.map((r) => {
          const existingID = inLibrary.get(r.mangabakaId)
          return (
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
                    {existingID && (
                      <Badge size="xs" color="green" variant="light">
                        in library
                      </Badge>
                    )}
                  </Group>
                </div>
                {existingID ? (
                  <Button size="xs" variant="default" component={Link} to={`/series/${existingID}`}>
                    Open
                  </Button>
                ) : (
                  <Button size="xs" onClick={() => add.mutate(r)} disabled={busy}>
                    Add
                  </Button>
                )}
              </Group>
            </Card>
          )
        })}
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
