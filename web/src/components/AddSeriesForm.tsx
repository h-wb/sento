import { useState } from 'react'
import { Anchor, Badge, Button, Card, Group, Image, Select, Stack, Text, TextInput } from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api, type LookupResult } from '../api'
import { notify } from '../ui'

// The quick search-and-add flow, reused by the global modal and the /add page.
// `onDone` fires when a series is added or an existing one is opened (the modal
// uses it to close itself and navigate); `onOpenImporter` is an optional escape
// hatch to the bulk import view.
export function AddSeriesForm({
  onDone,
  onOpenImporter,
}: {
  onDone: (seriesId: number) => void
  onOpenImporter?: () => void
}) {
  const [query, setQuery] = useState('')
  const [monitor, setMonitor] = useState('default')
  const [profileId, setProfileId] = useState(0) // 0 = instance default
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
      onDone(s.id)
    },
    onError: notify.err,
  })

  const search = () => query.trim() && lookup.mutate(query)
  const results = lookup.data
  const busy = lookup.isPending || add.isPending

  return (
    <>
      <Group mb="xs">
        <TextInput
          placeholder="Search MangaBaka…"
          leftSection={<IconSearch size={16} />}
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          onKeyDown={(e) => e.key === 'Enter' && search()}
          style={{ flex: 1 }}
          data-autofocus
        />
        <Button onClick={search} loading={busy}>
          Search
        </Button>
      </Group>
      <Group align="flex-start" gap="md" mb="md" wrap="wrap">
        <Select
          w={260}
          size="xs"
          label="Monitor collections on add"
          description="Changeable later per series"
          allowDeselect={false}
          value={monitor}
          onChange={(v) => v && setMonitor(v)}
          data={[
            { value: 'default', label: 'Default collection (standard, preferred language)' },
            { value: 'all', label: 'All volume collections' },
            { value: 'none', label: 'None' },
          ]}
        />
        {(profiles?.length ?? 0) > 1 && (
          <Select
            w={220}
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
      </Group>
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
                  <Button size="xs" variant="default" onClick={() => onDone(existingID)}>
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
      {onOpenImporter && (
        <Group justify="center" mt="lg">
          <Anchor component="button" type="button" size="sm" c="dimmed" onClick={onOpenImporter}>
            Importing an existing library? Open the importer →
          </Anchor>
        </Group>
      )}
    </>
  )
}
