import { Button, Center, SimpleGrid, Skeleton, Stack, Text } from '@mantine/core'
import { useLocalStorage } from '@mantine/hooks'
import { IconBooks, IconPlus } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useAddSeries } from '../addSeries'
import { api } from '../api'
import { BulkBar } from '../components/library/BulkBar'
import { LibraryToolbar } from '../components/library/LibraryToolbar'
import { PosterGrid } from '../components/library/PosterGrid'
import { SeriesTable } from '../components/library/SeriesTable'
import OrganizeLibrary from '../components/OrganizeLibrary'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { RemoveSeries } from '../components/RemoveSeries'
import { seriesQueries } from '../queries'
import type { Series } from '../types/store'
import type { SortField } from '../components/library/types'
import { notify, usePageTitle } from '../ui'
import classes from './LibraryPage.module.css'

function compare(a: Series, b: Series, field: SortField): number {
  const av = a[field]
  const bv = b[field]
  if (typeof av === 'number' || typeof bv === 'number') return (Number(av) || 0) - (Number(bv) || 0)
  return String(av ?? '').localeCompare(String(bv ?? ''))
}

export default function LibraryPage() {
  usePageTitle('Library')
  const queryClient = useQueryClient()
  const openAddSeries = useAddSeries()
  const { data: series, error, isPending, refetch } = useQuery(seriesQueries.list())
  // Under ['series'], so whatever refreshes the list refreshes these too.
  const { data: progressRows } = useQuery({ queryKey: ['series', 'progress'], queryFn: api.libraryProgress })
  const progress = new Map((progressRows ?? []).map((p) => [p.seriesId, p]))
  // "4/7": books owned of those the series collects that are out.
  const owned = (id: number) => {
    const p = progress.get(id)
    if (!p || p.collected === 0) return null
    return {
      complete: p.owned >= p.collected,
      share: (100 * p.owned) / p.collected,
      label: `${p.owned}/${p.collected}`,
      title: `${p.owned} of ${p.collected} collected books in the library`,
    }
  }
  const [filter, setFilter] = useState('')
  const [scope, setScope] = useState<'all' | 'monitored' | 'unmonitored'>('all')
  const [sortField, setSortField] = useState<SortField>('sortTitle')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  // Persist the chosen layout so it sticks across visits.
  const [view, setView] = useLocalStorage<'grid' | 'table'>({
    key: 'library-view',
    defaultValue: 'grid',
  })

  const shown = (series ?? [])
    .filter((s) => s.title.toLowerCase().includes(filter.trim().toLowerCase()))
    .filter((s) => scope === 'all' || (scope === 'monitored' ? s.monitored : !s.monitored))
    .sort((a, b) => {
      const r = compare(a, b, sortField)
      return sortDir === 'asc' ? r : -r
    })

  const toggleSort = (field: SortField) => {
    if (field === sortField) setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    else {
      setSortField(field)
      setSortDir('asc')
    }
  }

  // Selection helpers (table view only).
  const shownIds = shown.map((s) => s.id)
  const visibleSelected = shownIds.filter((id) => selected.has(id))
  const allSelected = shownIds.length > 0 && shownIds.every((id) => selected.has(id))
  const someSelected = shownIds.some((id) => selected.has(id))
  const toggleOne = (id: number) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  const toggleAll = () =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (allSelected) shownIds.forEach((id) => next.delete(id))
      else shownIds.forEach((id) => next.add(id))
      return next
    })

  const bulkMonitor = useMutation({
    mutationFn: ({ ids, monitored }: { ids: number[]; monitored: boolean }) =>
      Promise.all(ids.map((id) => api.setSeriesMonitored(id, monitored))),
    onSuccess: (_d, { ids, monitored }) =>
      notify.ok(`${ids.length} series ${monitored ? 'monitored' : 'unmonitored'}`),
    onError: notify.err,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['series'] }),
  })
  // The series being removed, once the user asked to.
  const [removing, setRemoving] = useState<number[] | null>(null)
  const [organizing, setOrganizing] = useState<number[] | null>(null)

  return (
    <>
      <PageHeader
        title="Library"
        count={series?.length}
        description="Manage your series and keep track of new releases."
        action={<Button size="sm" leftSection={<IconPlus size={16} />} onClick={openAddSeries}>Add series</Button>}
      />
      {series && series.length > 0 && (
        <LibraryToolbar series={series} scope={scope} setScope={setScope} setSelected={setSelected} filter={filter} setFilter={setFilter} sortField={sortField} sortDir={sortDir} toggleSort={toggleSort} view={view} setView={setView} />
      )}
      <QueryFeedback error={error} onRetry={() => void refetch()} />

      {isPending &&
        (view === 'grid' ? (
          <SimpleGrid className={classes.posterGrid} spacing="xl" verticalSpacing={30}>
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} height={280} radius="sm" />
            ))}
          </SimpleGrid>
        ) : (
          <Stack gap="xs">
            {Array.from({ length: 8 }, (_, i) => (
              <Skeleton key={i} height={36} radius="sm" />
            ))}
          </Stack>
        ))}

      {series && series.length === 0 && (
        <Center className={classes.empty} mih={400}>
          <Stack align="center" gap="sm">
            <div className={classes.emptyIcon}><IconBooks size={30} stroke={1.4} /></div>
            <Text fw={600} size="lg">Your library is empty</Text>
            <Text c="dimmed" size="sm" ta="center" maw={330}>Add a series to track releases and download missing volumes or chapters.</Text>
            <Button mt="sm" variant="default" leftSection={<IconPlus size={15} />} onClick={openAddSeries}>
              Add series
            </Button>
          </Stack>
        </Center>
      )}
      {series && series.length > 0 && shown.length === 0 && (
        <Center mih={220}><Stack align="center" gap="sm">
          <Text c="dimmed">{filter ? `No series match “${filter}”.` : 'No series in this view.'}</Text>
          <Button variant="subtle" color="gray" onClick={() => { setFilter(''); setScope('all') }}>Clear filters</Button>
        </Stack></Center>
      )}

      {shown.length > 0 && view === 'grid' && (
        <PosterGrid shown={shown} owned={owned} />
      )}

      {shown.length > 0 && view === 'table' && (
        <SeriesTable shown={shown} owned={owned} selected={selected} allSelected={allSelected} someSelected={someSelected} toggleAll={toggleAll} toggleOne={toggleOne} sortField={sortField} sortDir={sortDir} toggleSort={toggleSort} />
      )}

      <BulkBar view={view} visibleSelected={visibleSelected} bulkMonitor={bulkMonitor} setOrganizing={setOrganizing} setRemoving={setRemoving} setSelected={setSelected} />
      {organizing && <OrganizeLibrary seriesIds={organizing} onClose={() => setOrganizing(null)} />}
      {removing && (
        <RemoveSeries
          ids={removing}
          close={() => setRemoving(null)}
          removed={() => {
            setRemoving(null)
            setSelected(new Set())
            void queryClient.invalidateQueries({ queryKey: ['series'] })
          }}
        />
      )}
    </>
  )
}
