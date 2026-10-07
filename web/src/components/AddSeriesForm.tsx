import { useState } from 'react'
import { Anchor, Badge, Button, Card, Divider, Group, Image, Select, Stack, Text, TextInput } from '@mantine/core'
import { IconBooks, IconSearch } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type LookupResult } from '../api'
import { notify } from '../ui'
import { EmptyState } from './EmptyState'
import { QueryFeedback } from './QueryFeedback'

export function AddSeriesForm({ onDone, onOpenImporter }: {
  onDone: (seriesId: number) => void
  onOpenImporter?: () => void
}) {
  const queryClient = useQueryClient()
  const [query, setQuery] = useState('')
  const [monitor, setMonitor] = useState('default')
  const [profileId, setProfileId] = useState(0)
  const [searchedQuery, setSearchedQuery] = useState('')
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const existing = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const inLibrary = new Map((existing.data ?? []).map((s) => [s.mangabakaId, s.id]))
  const lookup = useMutation({ mutationFn: (q: string) => api.lookup(q) })
  const add = useMutation({
    mutationFn: (r: LookupResult) => api.addSeries({ mangabakaId: r.mangabakaId, monitor, profileId }),
    onSuccess: async (s) => {
      await queryClient.invalidateQueries({ queryKey: ['series'] })
      notify.ok(`Added "${s.title}"`)
      onDone(s.id)
    },
    onError: notify.err,
  })
  const busy = lookup.isPending || add.isPending
  const search = () => {
    if (!query.trim() || busy) return
    setSearchedQuery(query.trim())
    lookup.mutate(query.trim())
  }
  return (
    <>
      <form onSubmit={(e) => { e.preventDefault(); search() }}>
        <Group gap="sm" wrap="nowrap" align="flex-start">
          <TextInput placeholder="Search MangaBaka…" aria-label="Search MangaBaka"
            leftSection={<IconSearch size={16} />} value={query}
            onChange={(e) => setQuery(e.currentTarget.value)} style={{ flex: 1, minWidth: 0 }}
            disabled={add.isPending} data-autofocus />
          <Button type="submit" loading={lookup.isPending} disabled={!query.trim() || add.isPending}>Search</Button>
        </Group>
      </form>
      <Group align="flex-start" gap="md" mt="md" mb="lg">
        <Select size="sm" label="Monitor on add" description="Which editions to track for downloads"
          style={{ flex: 1, minWidth: 200 }} allowDeselect={false} value={monitor} disabled={busy}
          onChange={(v) => v && setMonitor(v)} data={[
            { value: 'default', label: 'Default edition' },
            { value: 'all', label: 'All volume editions' },
            { value: 'none', label: 'None — add without monitoring' },
          ]} />
        {(profiles.data?.length ?? 0) > 1 && (
          <Select size="sm" label="Release profile" description="Rules for choosing releases"
            style={{ flex: 1, minWidth: 180 }} allowDeselect={false} value={String(profileId)} disabled={busy}
            onChange={(v) => v && setProfileId(Number(v))} data={[
              { value: '0', label: 'Instance default' },
              ...(profiles.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
            ]} />
        )}
      </Group>
      <QueryFeedback error={existing.error || profiles.error} onRetry={() => { void existing.refetch(); void profiles.refetch() }} />
      <QueryFeedback pending={lookup.isPending} error={lookup.error} onRetry={search} />
      {!lookup.isPending && !lookup.error && lookup.data && (
        <Stack gap="sm" aria-live="polite">
          <Text size="xs" c="dimmed">{lookup.data.length} {lookup.data.length === 1 ? 'result' : 'results'} for “{searchedQuery}”</Text>
          {lookup.data.map((r) => {
            const existingID = inLibrary.get(r.mangabakaId)
            return (
              <Card key={r.mangabakaId} padding="sm">
                <Group wrap="nowrap" align="flex-start" gap="sm">
                  <Image src={r.coverUrl} w={48} h={72} radius="sm" alt="" fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='48' height='72'%3E%3Crect width='48' height='72' fill='%2352524e'/%3E%3C/svg%3E" />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Text fw={500} size="sm">{r.title}</Text>
                    <Group gap={6} mt={6}>
                      <Text size="xs" c="dimmed" tt="capitalize">{[r.type, r.status, r.year].filter(Boolean).join(' · ')}</Text>
                      {existingID && <Badge size="xs" color="green">In library</Badge>}
                    </Group>
                  </div>
                  {existingID ? (
                    <Button size="xs" variant="default" onClick={() => onDone(existingID)} disabled={add.isPending}>Open</Button>
                  ) : (
                    <Button size="xs" onClick={() => add.mutate(r)} loading={add.isPending && add.variables?.mangabakaId === r.mangabakaId}
                      disabled={busy || existing.isPending || !!existing.error || profiles.isPending || !!profiles.error}>Add</Button>
                  )}
                </Group>
              </Card>
            )
          })}
          {lookup.data.length === 0 && <EmptyState icon={<IconSearch size={24} />} title="No matching series"
            description="Try another title, its original name, or a shorter search." />}
        </Stack>
      )}
      {!lookup.data && !lookup.isPending && !lookup.error && <EmptyState icon={<IconBooks size={26} stroke={1.5} />}
        title="Search for a series" description="Search by title, then choose the manga you want to add. Monitoring and release profiles can be changed later." />}
      {onOpenImporter && <><Divider mt="lg" /><Group justify="center" mt="md">
        <Anchor component="button" type="button" size="xs" c="dimmed" onClick={onOpenImporter} disabled={add.isPending}>
          Already have manga on disk? Import a folder →
        </Anchor>
      </Group></>}
    </>
  )
}
