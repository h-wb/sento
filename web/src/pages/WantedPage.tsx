import { useMemo, useState } from 'react'
import {
  ActionIcon,
  Anchor,
  Badge,
  Card,
  Group,
  SegmentedControl,
  Stack,
  Table,
  Text,
  Tooltip,
} from '@mantine/core'
import { IconAlertCircle, IconRefresh, IconZoomCheck } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { WantedRow } from '../types/service'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { notify, usePageTitle } from '../ui'

type Filter = 'missing' | 'upgrades' | 'all'

// One series' wanted books: a compact table with the automatic search at
// the top, Sonarr's Wanted → Missing shape.
function SeriesWanted({ rows }: { rows: WantedRow[] }) {
  const first = rows[0]
  const client = useQueryClient()
  const search = useMutation({
    mutationFn: (ledger: 'volume' | 'chapter') =>
      api.search(first.seriesId, { ledger, force: false }),
    onSuccess: (res) => {
      notify.ok(
        res.queued.length > 0
          ? `Queued ${res.queued.length} release${res.queued.length > 1 ? 's' : ''} for ${first.seriesTitle}`
          : `No usable release found for ${first.seriesTitle}`,
      )
      void client.invalidateQueries({ queryKey: ['queue'] })
      void client.invalidateQueries({ queryKey: ['wanted'] })
    },
    onError: (e) => notify.err(String(e)),
  })
  const searching = (ledger: string) => search.isPending && search.variables === ledger
  const ledgers = [...new Set(rows.map((r) => r.ledger))]

  return (
    <Card padding={0} withBorder radius="md" mb="md">
      <Group justify="space-between" px="md" py="sm" wrap="wrap">
        <Group gap="xs">
          <Anchor component={Link} to={`/series/${first.seriesId}`} fw={600} size="sm">
            {first.seriesTitle}
          </Anchor>
          <Badge variant="light" color="gray" size="sm">
            {rows.length} wanted
          </Badge>
        </Group>
        <Group gap="xs">
          {ledgers.map((ledger) => (
            <Tooltip key={ledger} label={`Search ${ledger === 'volume' ? 'volumes' : 'chapters'} and grab the best release`}>
              <ActionIcon
                variant="subtle"
                aria-label={`Search ${ledger} for ${first.seriesTitle}`}
                loading={searching(ledger)}
                onClick={() => search.mutate(ledger === 'volume' ? 'volume' : 'chapter')}
              >
                <IconZoomCheck size={18} />
              </ActionIcon>
            </Tooltip>
          ))}
        </Group>
      </Group>
      <Table.ScrollContainer minWidth={520}>
        <Table verticalSpacing="xs" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Book</Table.Th>
              <Table.Th>Edition</Table.Th>
              <Table.Th>Status</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {rows.map((r) => (
              <Table.Tr key={`${r.versionId}-${r.book.id}`}>
                <Table.Td>
                  <Text fw={500}>{r.book.sequenceStr}</Text>
                  {r.book.countType !== 'main' && (
                    <Badge ml="xs" size="xs" variant="light" color="violet">
                      {r.book.countType}
                    </Badge>
                  )}
                </Table.Td>
                <Table.Td>
                  <Text size="sm" c="dimmed">
                    {r.editionName}
                    {r.language ? ` · ${r.language}` : ''}
                  </Text>
                </Table.Td>
                <Table.Td>
                  <Status row={r} />
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Card>
  )
}

function Status({ row }: { row: WantedRow }) {
  if (row.state === 'upgrade_eligible')
    return (
      <Tooltip label={row.reason || 'The file on disk is below the upgrade cutoff'}>
        <Badge variant="light" color="yellow" size="sm">
          Upgrade due
        </Badge>
      </Tooltip>
    )
  if (row.availability === 'upcoming')
    return (
      <Badge variant="light" color="grape" size="sm">
        Not released
      </Badge>
    )
  return (
    <Tooltip label={row.reason || 'Missing from the library'}>
      <Badge variant="light" color="red" size="sm">
        Missing
      </Badge>
    </Tooltip>
  )
}

export default function WantedPage() {
  usePageTitle('Wanted')
  const [filter, setFilter] = useState<Filter>('missing')
  const { data, isPending, error, refetch, dataUpdatedAt } = useQuery({
    queryKey: ['wanted'],
    queryFn: api.wanted,
    refetchInterval: 60_000,
  })

  const rows = useMemo(() => {
    if (!data) return []
    if (filter === 'missing') return data.filter((r) => r.state === 'missing')
    if (filter === 'upgrades') return data.filter((r) => r.state === 'upgrade_eligible')
    return data
  }, [data, filter])

  // Rows are sorted by series; group consecutive ones per series.
  const groups: { sid: number; rows: WantedRow[] }[] = []
  for (const r of rows) {
    const last = groups[groups.length - 1]
    if (last && last.sid === r.seriesId) last.rows.push(r)
    else groups.push({ sid: r.seriesId, rows: [r] })
  }

  const upgrades = data?.filter((r) => r.state === 'upgrade_eligible').length ?? 0
  const missing = (data?.length ?? 0) - upgrades

  return (
    <Stack>
      <PageHeader
        title="Wanted"
        description="Books your monitored editions are missing, library-wide. The Scan Library Folders task keeps this honest: a file that vanishes from disk reopens its book here."
        count={missing + upgrades}
        action={
          <Group gap="sm">
            <SegmentedControl
              size="xs"
              value={filter}
              onChange={(v) => setFilter(v as Filter)}
              data={[
                { value: 'missing', label: `Missing · ${missing}` },
                { value: 'upgrades', label: `Upgrades · ${upgrades}` },
                { value: 'all', label: 'All' },
              ]}
            />
            <ActionIcon variant="subtle" aria-label="Refresh wanted" onClick={() => void refetch()}>
              <IconRefresh size={18} />
            </ActionIcon>
          </Group>
        }
      />
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {data && (
        <Text size="xs" c="dimmed" mb={4}>
          {missing} missing · {upgrades} upgrade{upgrades === 1 ? '' : 's'} due
          {dataUpdatedAt ? ` · updated ${new Date(dataUpdatedAt).toLocaleTimeString()}` : ''}
        </Text>
      )}
      {!isPending && !error && groups.length === 0 && (
        <EmptyState
          icon={<IconAlertCircle size={28} />}
          title="Nothing is wanted"
          description="Every book of every monitored edition is on disk and up to your profiles' cutoffs."
        />
      )}
      {groups.map((g) => (
        <SeriesWanted key={g.sid} rows={g.rows} />
      ))}
    </Stack>
  )
}
