import { useState } from 'react'
import { Alert, Badge, Button, Card, Group, Stack, Text, TextInput, Title, Select, Tabs } from '@mantine/core'
import { IconFolderSearch, IconSearch } from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { api, type ImportProposal } from '../api'
import { usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { AddSeriesForm } from '../components/AddSeriesForm'

export default function AddPage() {
  usePageTitle('Add Series')
  const navigate = useNavigate()

  return (
    <>
      <PageHeader
        title="Add Series"
        description="Search MangaBaka and add a series to your library."
      />
      <Tabs defaultValue={new URLSearchParams(window.location.search).get('tab') === 'import' ? 'import' : 'search'}>
        <Tabs.List mb="lg">
          <Tabs.Tab value="search" leftSection={<IconSearch size={15} />}>Find a series</Tabs.Tab>
          <Tabs.Tab value="import" leftSection={<IconFolderSearch size={15} />}>Import from disk</Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel value="search" maw={720}><AddSeriesForm onDone={(id) => navigate(`/series/${id}`)} /></Tabs.Panel>
        <Tabs.Panel value="import"><ImportSection /></Tabs.Panel>
      </Tabs>
    </>
  )
}

// ImportSection bulk-imports an existing library: scan a root folder,
// confirm the proposed MangaBaka matches, files link in place (D14).
function ImportSection() {
  const queryClient = useQueryClient()
  const [root, setRoot] = useState('')
  const [matches, setMatches] = useState<Record<string, string | null>>({})
  const [imported, setImported] = useState<Record<string, number>>({})
  const [done, setDone] = useState<Record<string, string>>({}) // dir → outcome

  const scan = useMutation({
    mutationFn: () => api.importScan(root || undefined),
    onSuccess: () => { setDone({}); setMatches({}) },
  })
  const importOne = useMutation({
    mutationFn: (p: { dir: string; mangabakaId: number }) =>
      api.importFolder(p.dir, p.mangabakaId),
    onSuccess: (r, p) => {
      const linked = r.scan ? ` · ${r.scan.linked} files linked` : ''
      setDone((d) => ({ ...d, [p.dir]: (r.existing ? 'Linked to the series in your library' : 'Imported') + linked }))
      setImported((prev) => ({ ...prev, [p.dir]: r.seriesId }))
      void queryClient.invalidateQueries({ queryKey: ['series'] })
    },
    onError: (e, p) => setDone((d) => ({ ...d, [p.dir]: String(e) })),
  })

  const proposalLabel = (p: ImportProposal) => {
    const best = p.candidates?.find((c) => c.mangabakaId === p.bestId)
    if (p.bestId && !best) return `mangabaka:${p.bestId}` // folder-name hint
    if (best) return `${best.title}${best.year ? ` (${best.year})` : ''}`
    return 'no match'
  }

  return (
    <Stack gap="md" maw={860}>
      <Title order={4}>Import existing library</Title>
      <Text size="sm" c="dimmed">
        Scan a folder of series directories: each is matched against MangaBaka and its files are
        linked in place — nothing is moved or renamed.
      </Text>
      <form onSubmit={(e) => { e.preventDefault(); if (!scan.isPending && !importOne.isPending) scan.mutate() }}><Group align="flex-end">
        <TextInput
          label="Library folder"
          description="A path on the Sento server; leave blank to use the configured library folder"
          placeholder="/manga"
          disabled={scan.isPending || importOne.isPending}
          value={root}
          onChange={(e) => setRoot(e.currentTarget.value)}
          style={{ flex: 1, maxWidth: 480 }}
        />
        <Button
          variant="default"
          leftSection={<IconFolderSearch size={16} />}
          loading={scan.isPending}
          type="submit"
          disabled={importOne.isPending}
        >
          Scan
        </Button>
      </Group></form>
      {scan.error && <Alert color="red">{String(scan.error)}</Alert>}
      {scan.data?.length === 0 && <Text c="dimmed">No subfolders found.</Text>}
      <Stack gap={6}>
        {scan.data?.map((p) => {
          const existingId = p.existingSeriesId || imported[p.dir]
          const match = Object.hasOwn(matches, p.dir) ? matches[p.dir] : p.bestId ? String(p.bestId) : null
          const candidates = (p.candidates ?? []).map((c) => ({ value: String(c.mangabakaId), label: `${c.title}${c.year ? ` (${c.year})` : ''}` }))
          if (p.bestId && !candidates.some((c) => c.value === String(p.bestId))) candidates.push({ value: String(p.bestId), label: proposalLabel(p) })
          return <Card key={p.dir} withBorder padding="xs">
            <Group justify="space-between" align="flex-end">
              <div style={{ minWidth: 0, flex: 1 }}>
                <Text size="sm" style={{ wordBreak: 'break-all' }}>
                  {p.dir}
                </Text>
                <Group gap={6} mt={2}>
                  {existingId ? (
                    <Badge size="xs" color="green" variant="light">
                      already in library
                    </Badge>
                  ) : (
                    <>
                      <Text size="xs" c="dimmed">
                        Suggested: {proposalLabel(p)}
                      </Text>
                      <Badge
                        size="xs"
                        color={p.confidence >= 0.9 ? 'green' : p.confidence >= 0.7 ? 'yellow' : 'red'}
                        variant="light"
                      >
                        {Math.round(p.confidence * 100)}% match
                      </Badge>
                    </>
                  )}
                  {done[p.dir] && (
                    <Text size="xs" c={imported[p.dir] ? 'green' : 'red'}>
                      {done[p.dir]}
                    </Text>
                  )}
                </Group>
                {!existingId && <Select mt="sm" size="sm" label="Match to series" placeholder="No match found"
                  searchable clearable data={candidates} value={match}
                  disabled={importOne.isPending} onChange={(value) => setMatches((prev) => ({ ...prev, [p.dir]: value }))} />}
              </div>
              {existingId ? (
                <Button
                  size="compact-xs"
                  variant="subtle"
                  component={Link}
                  to={`/series/${existingId}`}
                >
                  Open
                </Button>
              ) : (
                <Button
                  size="compact-xs"
                  disabled={!match || importOne.isPending}
                  loading={importOne.isPending && importOne.variables?.dir === p.dir}
                  onClick={() => match && importOne.mutate({ dir: p.dir, mangabakaId: Number(match) })}
                >
                  {done[p.dir] && !imported[p.dir] ? 'Retry import' : 'Import'}
                </Button>
              )}
            </Group>
          </Card>
        })}
      </Stack>
    </Stack>
  )
}
