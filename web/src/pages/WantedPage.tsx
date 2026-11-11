import { useMemo, useState } from 'react'
import { ActionIcon, Anchor, Button, Card, Group, Image, SegmentedControl, Stack, Text, Tooltip } from '@mantine/core'
import { IconCircleCheck, IconRefresh, IconSearch } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { WantedRow } from '../types/service'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { bookRanges } from '../components/bookRanges'
import { notify, usePageTitle } from '../ui'
import classes from './WantedPage.module.css'

type Filter = 'missing' | 'upgrades' | 'all'
type Kind = 'missing' | 'upcoming' | 'upgrade'

const kindOf = (r: WantedRow): Kind =>
  r.state === 'upgrade_eligible' ? 'upgrade' : r.availability === 'upcoming' ? 'upcoming' : 'missing'
const kindLabel = (kind: Kind, n: number) =>
  kind === 'upgrade' ? `${n} upgrade${n === 1 ? '' : 's'} due` : kind === 'upcoming' ? `${n} not out yet` : `${n} missing`

// "Vol. 4–12, 14 · Extra 1": main books as ranges, anything else by name.
function booksText(rows: WantedRow[]) {
  const prefix = rows[0].ledger === 'chapter' ? 'Ch.' : 'Vol.'
  const main = rows.filter((r) => r.book.countType === 'main')
  const other = rows.filter((r) => r.book.countType !== 'main')
  return [
    main.length ? `${prefix} ${bookRanges(main, (r) => r.book.sequenceNum, (r) => r.book.sequenceStr)}` : '',
    ...other.map((r) => `${r.book.countType} ${r.book.sequenceStr}`.trim()),
  ]
    .filter(Boolean)
    .join(' · ')
}

// A series' wanted books, one line per card and kind, with the automatic
// search for each ledger it is missing.
function SeriesWanted({ rows, coverUrl }: { rows: WantedRow[]; coverUrl?: string }) {
  const first = rows[0]
  const client = useQueryClient()
  const search = useMutation({
    mutationFn: (ledger: 'volume' | 'chapter') => api.search(first.seriesId, { ledger, force: false }),
    onSuccess: (res) => {
      const queued = res.queued?.length ?? 0
      notify.ok(
        queued > 0
          ? `Queued ${queued} release${queued > 1 ? 's' : ''} for ${first.seriesTitle}`
          : `No usable release found for ${first.seriesTitle}`,
      )
      void client.invalidateQueries({ queryKey: ['queue'] })
      void client.invalidateQueries({ queryKey: ['activity'] })
      void client.invalidateQueries({ queryKey: ['wanted'] })
    },
    onError: notify.err,
  })
  const ledgers = [...new Set(rows.map((r) => (r.ledger === 'chapter' ? 'chapter' : 'volume')))]

  const lines = new Map<string, WantedRow[]>()
  for (const r of rows) {
    const key = `${r.versionId}/${kindOf(r)}`
    lines.set(key, [...(lines.get(key) ?? []), r])
  }

  return (
    <Card padding="md" radius="md">
      <Group justify="space-between" wrap="nowrap" align="flex-start" gap="md">
        <Group gap="sm" wrap="nowrap" miw={0} align="center">
          <Image src={coverUrl} w={32} h={46} radius="sm" alt="" fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E" />
          <Anchor component={Link} to={`/series/${first.seriesId}`} fw={600} size="sm" c="inherit" lineClamp={1}>
            {first.seriesTitle}
          </Anchor>
        </Group>
        <Group gap="xs" wrap="nowrap">
          {ledgers.map((ledger) => (
            <Tooltip key={ledger} label={`Search every indexer and source, and download the best release for each missing ${ledger}`} multiline maw={260} openDelay={300}>
              <Button
                size="xs"
                variant="default"
                leftSection={<IconSearch size={14} />}
                aria-label={`Search ${ledger}s for ${first.seriesTitle}`}
                loading={search.isPending && search.variables === ledger}
                disabled={search.isPending}
                onClick={() => search.mutate(ledger)}
              >
                {ledgers.length > 1 ? `Search ${ledger}s` : 'Search'}
              </Button>
            </Tooltip>
          ))}
        </Group>
      </Group>
      <div className={classes.lines}>
        {[...lines.values()].map((line) => {
          const r = line[0]
          const kind = kindOf(r)
          const reasons = [...new Set(line.map((x) => x.reason).filter(Boolean))]
          return (
            <div key={`${r.versionId}/${kind}`} className={classes.line}>
              <Text size="sm" c="dimmed" className={classes.card}>
                {[r.editionName, r.card, r.language].filter(Boolean).join(' · ')}
              </Text>
              <Text size="sm" className={classes.books}>{booksText(line)}</Text>
              <Tooltip label={reasons.join(' · ')} disabled={kind === 'missing' || reasons.length === 0} multiline maw={320}>
                <Text size="xs" className={classes.kind} c={kind === 'upgrade' ? 'yellow' : 'dimmed'}>
                  {kindLabel(kind, line.length)}
                </Text>
              </Tooltip>
            </div>
          )
        })}
      </div>
    </Card>
  )
}

export default function WantedPage() {
  usePageTitle('Wanted')
  const [filter, setFilter] = useState<Filter>('missing')
  const { data, isPending, error, refetch, isRefetching } = useQuery({
    queryKey: ['wanted'],
    queryFn: api.wanted,
    refetchInterval: 60_000,
  })
  // Already loaded by the shell for search; here only for the covers.
  const { data: series } = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const covers = new Map((series ?? []).map((s) => [s.id, s.coverUrl]))

  const rows = useMemo(() => {
    if (!data) return []
    if (filter === 'missing') return data.filter((r) => r.state !== 'upgrade_eligible')
    if (filter === 'upgrades') return data.filter((r) => r.state === 'upgrade_eligible')
    return data
  }, [data, filter])

  // Rows arrive sorted by series; group consecutive ones per series.
  const groups: { sid: number; rows: WantedRow[] }[] = []
  for (const r of rows) {
    const last = groups[groups.length - 1]
    if (last && last.sid === r.seriesId) last.rows.push(r)
    else groups.push({ sid: r.seriesId, rows: [r] })
  }

  const upgrades = data?.filter((r) => r.state === 'upgrade_eligible').length ?? 0
  const missing = (data?.length ?? 0) - upgrades

  return (
    <Stack gap="md">
      <PageHeader
        title="Wanted"
        description="Books your monitored editions are still missing, across the library."
        count={data?.length}
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
            <Tooltip label="Refresh">
              <ActionIcon variant="subtle" color="gray" aria-label="Refresh wanted" loading={isRefetching} onClick={() => void refetch()}>
                <IconRefresh size={18} />
              </ActionIcon>
            </Tooltip>
          </Group>
        }
      />
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {!isPending && !error && groups.length === 0 && (
        <EmptyState
          icon={<IconCircleCheck size={28} />}
          title={filter === 'upgrades' ? 'No upgrades due' : 'Nothing is wanted'}
          description={
            filter === 'upgrades'
              ? 'Every file on disk meets its profile’s cutoff.'
              : 'Every book of every monitored edition is on disk.'
          }
        />
      )}
      {groups.map((g) => (
        <SeriesWanted key={g.sid} rows={g.rows} coverUrl={covers.get(g.sid)} />
      ))}
    </Stack>
  )
}
