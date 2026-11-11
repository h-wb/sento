import { useState } from 'react'
import {
  Anchor,
  ActionIcon,
  Affix,
  Badge,
  Button,
  Center,
  Checkbox,
  Divider,
  Group,
  Image,
  Menu,
  Paper,
  SimpleGrid,
  UnstyledButton,
  Skeleton,
  Stack,
  Table,
  Text,
  TextInput,
  Tooltip,
  Transition,
} from '@mantine/core'
import { useLocalStorage } from '@mantine/hooks'
import {
  IconArrowsSort,
  IconBooks,
  IconChevronDown,
  IconChevronUp,
  IconEye,
  IconEyeOff,
  IconFolders,
  IconLayoutGrid,
  IconLayoutList,
  IconPlus,
  IconSearch,
  IconSelector,
  IconTrash,
} from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api'
import type { Series } from '../types/store'
import { fmtWhen, notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { useAddSeries } from '../addSeries'
import { RemoveSeries } from '../components/RemoveSeries'
import OrganizeLibrary from '../components/OrganizeLibrary'
import { QueryFeedback } from '../components/QueryFeedback'
import classes from './LibraryPage.module.css'

type SortField = 'sortTitle' | 'type' | 'status' | 'year' | 'finalVolume' | 'totalChapters' | 'addedAt'

const SORTS: { value: SortField; label: string }[] = [
  { value: 'sortTitle', label: 'Title' },
  { value: 'addedAt', label: 'Recently added' },
  { value: 'year', label: 'Year' },
  { value: 'status', label: 'Status' },
]

function compare(a: Series, b: Series, field: SortField): number {
  const av = a[field]
  const bv = b[field]
  if (typeof av === 'number' || typeof bv === 'number') return (Number(av) || 0) - (Number(bv) || 0)
  return String(av ?? '').localeCompare(String(bv ?? ''))
}

export default function LibraryPage() {
  usePageTitle('Library')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const openAddSeries = useAddSeries()
  const { data: series, error, isPending, refetch } = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
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

  const sortTh = (label: string, field: SortField, align: 'left' | 'right' = 'left') => (
    <Table.Th
      onClick={() => toggleSort(field)}
      style={{ cursor: 'pointer', userSelect: 'none' }}
      ta={align}
      aria-sort={sortField === field ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
    >
      <UnstyledButton onClick={(e) => { e.stopPropagation(); toggleSort(field) }} aria-label={`Sort by ${label}`} w="100%"><Group gap={4} wrap="nowrap" justify={align === 'right' ? 'flex-end' : 'flex-start'}>
        {label}
        {sortField === field ? (
          sortDir === 'asc' ? (
            <IconChevronUp size={13} />
          ) : (
            <IconChevronDown size={13} />
          )
        ) : (
          <IconSelector size={13} style={{ opacity: 0.35 }} />
        )}
      </Group></UnstyledButton>
    </Table.Th>
  )

  return (
    <>
      <PageHeader
        title="Library"
        count={series?.length}
        description="Manage your series and keep track of new releases."
        action={<Button size="sm" leftSection={<IconPlus size={16} />} onClick={openAddSeries}>Add series</Button>}
      />
      {series && series.length > 0 && (
        <div className={classes.toolbar}>
          <div className={classes.scopes} role="group" aria-label="Monitoring filter">
            {(['all', 'monitored', 'unmonitored'] as const).map((value) => (
              <UnstyledButton key={value} className={classes.scope} data-active={scope === value}
                aria-pressed={scope === value} onClick={() => { setScope(value); setSelected(new Set()) }}>
                {value === 'all' ? 'All series' : value === 'monitored' ? 'Monitored' : 'Unmonitored'}
                <span>{value === 'all' ? series.length : series.filter((s) => value === 'monitored' ? s.monitored : !s.monitored).length}</span>
              </UnstyledButton>
            ))}
          </div>
          <Group gap="xs" wrap="nowrap" className={classes.tools}>
            <TextInput size="sm" placeholder="Filter by title…" aria-label="Filter by title"
              leftSection={<IconSearch size={15} />} value={filter}
              onChange={(e) => { setFilter(e.currentTarget.value); setSelected(new Set()) }} className={classes.search} />
            <Menu position="bottom-end" withArrow>
              <Menu.Target>
                <Tooltip label="Sort" openDelay={400}>
                  <ActionIcon size="lg" variant="default" aria-label="Sort">
                    <IconArrowsSort size={16} />
                  </ActionIcon>
                </Tooltip>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Label>Sort by</Menu.Label>
                {SORTS.map((s) => (
                  <Menu.Item
                    key={s.value}
                    onClick={() => toggleSort(s.value)}
                    rightSection={
                      sortField === s.value ? (
                        sortDir === 'asc' ? (
                          <IconChevronUp size={14} />
                        ) : (
                          <IconChevronDown size={14} />
                        )
                      ) : undefined
                    }
                  >
                    {s.label}
                  </Menu.Item>
                ))}
              </Menu.Dropdown>
            </Menu>

            <ActionIcon.Group>
              <Tooltip label="Poster grid" openDelay={400}>
                <ActionIcon
                  size="lg"
                  variant={view === 'grid' ? 'filled' : 'default'}
                  onClick={() => { setView('grid'); setSelected(new Set()) }}
                  aria-label="Poster grid"
                  aria-pressed={view === 'grid'}
                >
                  <IconLayoutGrid size={16} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Table" openDelay={400}>
                <ActionIcon
                  size="lg"
                  variant={view === 'table' ? 'filled' : 'default'}
                  onClick={() => setView('table')}
                  aria-label="Table"
                  aria-pressed={view === 'table'}
                >
                  <IconLayoutList size={16} />
                </ActionIcon>
              </Tooltip>
            </ActionIcon.Group>

          </Group>
        </div>
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
        <SimpleGrid className={classes.posterGrid} spacing="xl" verticalSpacing={30}>
          {shown.map((s) => (
            <UnstyledButton
              key={s.id}
              className={classes.poster}
              component={Link}
              to={`/series/${s.id}`}
              data-monitored={s.monitored}
            >
              <div className={classes.cover}>
                {s.coverUrl ? <Image src={s.coverUrl} alt={s.title} className={classes.coverImage}
                  fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='240' height='360'%3E%3Crect width='240' height='360' fill='%2352524e'/%3E%3C/svg%3E" /> :
                  <div className={classes.coverFallback}><IconBooks size={32} stroke={1} /><Text size="sm" ta="center">{s.title}</Text></div>}
                {!s.monitored && <Tooltip label="Unmonitored"><span className={classes.monitorFlag}><IconEyeOff size={14} /></span></Tooltip>}
                {owned(s.id) && (
                  <span className={classes.progress} data-complete={owned(s.id)!.complete}>
                    <span style={{ width: `${owned(s.id)!.share}%` }} />
                  </span>
                )}
              </div>
              <Text className={classes.posterTitle} fw={500} size="sm" mt={11} lineClamp={2}>{s.title}</Text>
              <Group gap={6} mt={4} className={classes.metadata}>
                {s.status && <><span className={classes.releaseDot} data-ongoing={s.status === 'ongoing'} /><Text size="xs" tt="capitalize">{s.status.replaceAll('_', ' ')}</Text></>}
                {owned(s.id) ? (
                  <Text size="xs" title={owned(s.id)!.title}>· {owned(s.id)!.label}</Text>
                ) : (
                  s.finalVolume > 0 && <Text size="xs">· {s.finalVolume} vol</Text>
                )}
                {!s.status && (s.year ?? 0) > 0 && <Text size="xs">{s.year}</Text>}
              </Group>
            </UnstyledButton>
          ))}
        </SimpleGrid>
      )}

      {shown.length > 0 && view === 'table' && (
        <Table.ScrollContainer minWidth={720}>
          <Table highlightOnHover verticalSpacing="xs" stickyHeader>
            <Table.Thead>
              <Table.Tr>
                <Table.Th w={36}>
                  <Checkbox
                    aria-label="Select all"
                    checked={allSelected}
                    indeterminate={someSelected && !allSelected}
                    onChange={toggleAll}
                  />
                </Table.Th>
                {sortTh('Title', 'sortTitle')}
                {sortTh('Type', 'type')}
                {sortTh('Status', 'status')}
                {sortTh('Year', 'year')}
                <Table.Th ta="right">Owned</Table.Th>
                {sortTh('Vol', 'finalVolume', 'right')}
                {sortTh('Chapters', 'totalChapters', 'right')}
                {sortTh('Added', 'addedAt')}
                <Table.Th w={40} ta="center">
                  <Tooltip label="Monitored">
                    <IconEye size={15} style={{ verticalAlign: 'middle' }} />
                  </Tooltip>
                </Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {shown.map((s) => (
                <Table.Tr
                  key={s.id}
                  onClick={() => navigate(`/series/${s.id}`)}
                  style={{ cursor: 'pointer' }}
                  bg={selected.has(s.id) ? 'var(--mantine-color-brand-light)' : undefined}
                  opacity={s.monitored ? 1 : 0.55}
                >
                  <Table.Td onClick={(e) => e.stopPropagation()}>
                    <Checkbox
                      aria-label={`Select ${s.title}`}
                      checked={selected.has(s.id)}
                      onChange={() => toggleOne(s.id)}
                    />
                  </Table.Td>
                  <Table.Td>
                    <Group gap="sm" wrap="nowrap">
                      <Image
                        src={s.coverUrl}
                        w={28}
                        h={40}
                        radius="sm"
                        alt=""
                        fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E"
                      />
                      <Anchor component={Link} to={`/series/${s.id}`} c="inherit" fw={500} size="sm" lineClamp={1} underline="hover">
                        {s.title}
                      </Anchor>
                    </Group>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed" tt="capitalize">
                      {s.type || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" variant="light">
                      {s.status}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed">
                      {s.year || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="right">
                    <Text size="sm" c={owned(s.id)?.complete === false ? undefined : 'dimmed'} title={owned(s.id)?.title}>
                      {owned(s.id)?.label ?? '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="right">
                    <Text size="sm" c="dimmed">
                      {s.finalVolume || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="right">
                    <Text size="sm" c="dimmed">
                      {s.totalChapters || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed" title={s.addedAt}>
                      {fmtWhen(s.addedAt)}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="center">
                    {s.monitored ? (
                      <IconEye size={15} style={{ verticalAlign: 'middle' }} />
                    ) : (
                      <IconEyeOff size={15} style={{ verticalAlign: 'middle', opacity: 0.5 }} />
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}

      {/* Floating bulk action bar — slides up from the bottom on selection. */}
      <Affix position={{ bottom: 24, left: '50%' }} style={{ transform: 'translateX(-50%)' }}>
        <Transition transition="slide-up" mounted={view === 'table' && visibleSelected.length > 0}>
          {(styles) => (
            <Paper
              shadow="md"
              radius="xl"
              withBorder
              py="xs"
              pl="lg"
              pr="xs"
              style={styles}
            >
              <Group gap="sm" wrap="nowrap">
                <Text size="sm" fw={600}>
                  {visibleSelected.length} selected
                </Text>
                <Divider orientation="vertical" />
                <Button
                  size="xs"
                  radius="xl"
                  variant="light"
                  leftSection={<IconEye size={14} />}
                  loading={bulkMonitor.isPending}
                  onClick={() => bulkMonitor.mutate({ ids: visibleSelected, monitored: true })}
                >
                  Monitor
                </Button>
                <Button
                  size="xs"
                  radius="xl"
                  variant="light"
                  color="gray"
                  leftSection={<IconEyeOff size={14} />}
                  loading={bulkMonitor.isPending}
                  onClick={() => bulkMonitor.mutate({ ids: visibleSelected, monitored: false })}
                >
                  Unmonitor
                </Button>
                <Button
                  size="xs"
                  radius="xl"
                  variant="light"
                  color="gray"
                  leftSection={<IconFolders size={14} />}
                  disabled={bulkMonitor.isPending}
                  onClick={() => setOrganizing(visibleSelected)}
                >
                  Organize files
                </Button>
                <Button
                  size="xs"
                  radius="xl"
                  variant="light"
                  color="red"
                  leftSection={<IconTrash size={14} />}
                  disabled={bulkMonitor.isPending}
                  onClick={() => setRemoving(visibleSelected)}
                >
                  Delete
                </Button>
                <Button size="xs" radius="xl" variant="subtle" onClick={() => setSelected(new Set())}>
                  Clear
                </Button>
              </Group>
            </Paper>
          )}
        </Transition>
      </Affix>
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
