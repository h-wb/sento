import { useState } from 'react'
import {
  ActionIcon,
  Affix,
  Alert,
  Badge,
  Button,
  Card,
  Center,
  Checkbox,
  Divider,
  Group,
  Image,
  Menu,
  Paper,
  Popover,
  SimpleGrid,
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
  IconFilter,
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
  const { data: series, error, isPending } = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const [filter, setFilter] = useState('')
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
  const bulkDelete = useMutation({
    mutationFn: (ids: number[]) => Promise.all(ids.map((id) => api.deleteSeries(id))),
    onSuccess: (_d, ids) => notify.ok(`Deleted ${ids.length} series`),
    onError: notify.err,
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['series'] })
      setSelected(new Set())
    },
  })

  const sortTh = (label: string, field: SortField, align: 'left' | 'right' = 'left') => (
    <Table.Th
      onClick={() => toggleSort(field)}
      style={{ cursor: 'pointer', userSelect: 'none' }}
      ta={align}
    >
      <Group gap={4} wrap="nowrap" justify={align === 'right' ? 'flex-end' : 'flex-start'}>
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
      </Group>
    </Table.Th>
  )

  return (
    <>
      <PageHeader
        title="Library"
        count={series?.length}
        description="Your manga collection."
        action={
          <Group gap="xs">
            {series && series.length > 0 && (
              <>
                <Popover position="bottom-end" withArrow trapFocus width={260}>
                  <Popover.Target>
                    <Tooltip label="Filter" openDelay={400}>
                      <ActionIcon
                        size="lg"
                        variant={filter ? 'light' : 'default'}
                        aria-label="Filter"
                      >
                        <IconFilter size={16} />
                      </ActionIcon>
                    </Tooltip>
                  </Popover.Target>
                  <Popover.Dropdown>
                    <TextInput
                      size="xs"
                      placeholder="Filter by title…"
                      leftSection={<IconSearch size={14} />}
                      value={filter}
                      onChange={(e) => setFilter(e.currentTarget.value)}
                      data-autofocus
                      rightSection={
                        filter ? (
                          <ActionIcon
                            size="sm"
                            variant="subtle"
                            color="gray"
                            onClick={() => setFilter('')}
                            aria-label="Clear filter"
                          >
                            ✕
                          </ActionIcon>
                        ) : undefined
                      }
                    />
                  </Popover.Dropdown>
                </Popover>

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
                      onClick={() => setView('grid')}
                      aria-label="Poster grid"
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
                    >
                      <IconLayoutList size={16} />
                    </ActionIcon>
                  </Tooltip>
                </ActionIcon.Group>

                <Divider orientation="vertical" />
              </>
            )}
            <Tooltip label="Add series" openDelay={400}>
              <ActionIcon size="lg" variant="filled" onClick={openAddSeries} aria-label="Add series">
                <IconPlus size={16} />
              </ActionIcon>
            </Tooltip>
          </Group>
        }
      />
      {error && <Alert color="red">{String(error)}</Alert>}

      {isPending &&
        (view === 'grid' ? (
          <SimpleGrid cols={{ base: 2, sm: 4, lg: 6 }}>
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
        <Center mih={300}>
          <Stack align="center" gap="xs">
            <IconBooks size={42} opacity={0.4} />
            <Text c="dimmed">Your library is empty.</Text>
            <Button variant="light" onClick={openAddSeries}>
              Add your first series
            </Button>
          </Stack>
        </Center>
      )}
      {series && series.length > 0 && shown.length === 0 && (
        <Text c="dimmed">No series match “{filter}”.</Text>
      )}

      {shown.length > 0 && view === 'grid' && (
        <SimpleGrid cols={{ base: 2, sm: 4, lg: 6 }}>
          {shown.map((s) => (
            <Card
              key={s.id}
              className="hoverLift"
              component={Link}
              to={`/series/${s.id}`}
              padding="xs"
              withBorder
              style={{ textDecoration: 'none' }}
              opacity={s.monitored ? 1 : 0.55}
            >
              <Card.Section>
                <Image
                  src={s.coverUrl}
                  height={220}
                  alt={s.title}
                  fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E"
                />
              </Card.Section>
              <Text fw={600} size="sm" mt="xs" lineClamp={2}>
                {s.title}
              </Text>
              <Group gap={4} mt={4}>
                <Badge size="xs" variant="light">
                  {s.status}
                </Badge>
                {s.finalVolume > 0 && (
                  <Badge size="xs" variant="outline">
                    {s.finalVolume} vol
                  </Badge>
                )}
                {!s.monitored && (
                  <Badge size="xs" color="gray" variant="outline">
                    unmonitored
                  </Badge>
                )}
              </Group>
            </Card>
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
                      <Text fw={600} size="sm" lineClamp={1}>
                        {s.title}
                      </Text>
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
        <Transition transition="slide-up" mounted={view === 'table' && selected.size > 0}>
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
                  {selected.size} selected
                </Text>
                <Divider orientation="vertical" />
                <Button
                  size="xs"
                  radius="xl"
                  variant="light"
                  leftSection={<IconEye size={14} />}
                  loading={bulkMonitor.isPending}
                  onClick={() => bulkMonitor.mutate({ ids: [...selected], monitored: true })}
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
                  onClick={() => bulkMonitor.mutate({ ids: [...selected], monitored: false })}
                >
                  Unmonitor
                </Button>
                <Button
                  size="xs"
                  radius="xl"
                  variant="light"
                  color="red"
                  leftSection={<IconTrash size={14} />}
                  loading={bulkDelete.isPending}
                  onClick={() => {
                    if (window.confirm(`Delete ${selected.size} series? This cannot be undone.`))
                      bulkDelete.mutate([...selected])
                  }}
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
    </>
  )
}
