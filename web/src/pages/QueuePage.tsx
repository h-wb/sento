import ReviewInbox from '../components/ReviewInbox'
import BookDialog, { type BookSelection } from '../components/BookDialog'
import type { QueueItem } from '../types/store'
import { useState, type ReactNode } from 'react'
import {
  Anchor,
  Badge,
  Button,
  Card,
  Collapse,
  Group,
  Progress,
  Stack,
  Table,
  Tabs,
  Text,
  Tooltip,
} from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { ErrorDetails } from '../components/ErrorDetails'
import { IconBan, IconDownload, IconRefresh } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { fmtWhen, notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'

const statusColor: Record<string, string> = {
  queued: 'gray',
  downloading: 'blue',
  importing: 'cyan',
  imported: 'green',
  failed: 'red',
  needs_review: 'orange',
  retry_wait: 'yellow',
  dispatch_unknown: 'orange',
  cancelled: 'gray',
}
const statusLabel: Record<string, string> = {
  queued: 'Queued',
  downloading: 'Downloading',
  importing: 'Importing',
  imported: 'Imported',
  failed: 'Failed',
  needs_review: 'Needs review',
  retry_wait: 'Retry scheduled',
  dispatch_unknown: 'Confirm download',
  cancelled: 'Cancelled',
}

function downloadLabel(item: QueueItem) {
  if (item.status !== 'downloading') return statusLabel[item.status] ?? item.status
  if (item.stalled) return 'Stalled'
  const states: Record<string, string> = {
    pausedDL: 'Paused in client', stoppedDL: 'Paused in client', queuedDL: 'Queued in client',
    checkingDL: 'Checking files', checkingResumeData: 'Checking files', moving: 'Moving files',
    missingFiles: 'Files missing', error: 'Client error',
    'waiting for source cooldown': 'Waiting for source',
    'waiting for download slot': 'Waiting for download slot',
  }
  return states[item.downloadState] ?? 'Downloading'
}

function QueueSeriesGroup({ items, renderJobs, onAnother, pending }: {
  items: QueueItem[]
  renderJobs: (items: QueueItem[]) => ReactNode
  onAnother: (ids: number[]) => void
  pending: boolean
}) {
  const [opened, setOpened] = useState(items.length <= 5)
  const first = items[0]
  const counts = new Map<string, number>()
  for (const item of items) {
    const label = downloadLabel(item).toLowerCase()
    counts.set(label, (counts.get(label) ?? 0) + 1)
  }
  const retry = items.filter((i) => i.canTryAnother)
  const title = first.seriesTitle || `Series ${first.seriesId}`
  const panelID = `queue-series-${first.seriesId}`
  return (
    <Card padding="md" data-testid="queue-series">
      <Group justify="space-between" align="flex-start" gap="sm">
        <Stack gap={4} style={{ minWidth: 0, flex: 1 }}>
          <Anchor component={Link} to={`/series/${first.seriesId}`} fw={600} c="inherit" style={{ overflowWrap: 'anywhere' }}>{title}</Anchor>
          <Text size="sm" c="dimmed">{[...counts].map(([label, count]) => `${count} ${label}`).join(' · ')}</Text>
        </Stack>
        {retry.length > 0 && <Button size="compact-xs" variant="default" loading={pending} onClick={() => onAnother(retry.map((i) => i.id))}>
          Try another for {retry.length} {retry.length === 1 ? 'download' : 'downloads'}
        </Button>}
      </Group>
      <Button mt="sm" size="compact-xs" variant="subtle" style={{ alignSelf: 'flex-start' }} aria-expanded={opened} aria-controls={panelID} onClick={() => setOpened(!opened)}>
        {opened ? 'Hide' : 'Show'} {items.length} {items.length === 1 ? 'download' : 'downloads'}
      </Button>
      <Collapse expanded={opened} id={panelID}>{opened && <div style={{ marginTop: 12 }}>{renderJobs(items)}</div>}</Collapse>
    </Card>
  )
}

export default function QueuePage() {
  usePageTitle('Activity')
  const small = useMediaQuery('(max-width: 48em)')
  const [tab, setTab] = useState<string | null>('queue')
  const [recovery, setRecovery] = useState<{ seriesID: number; sel: BookSelection } | null>(null)
  const queryClient = useQueryClient()
  const queue = useQuery({
    queryKey: ['queue', true],
    queryFn: () => api.queue(true),
    refetchInterval: 5000,
  })
  const blocked = useQuery({ queryKey: ['blocklist'], queryFn: api.blocklist })
  // Which client holds a download only matters when there are several.
  const connections = useQuery({ queryKey: ['connections'], queryFn: api.connections })
  const clients = connections.data?.downloadClients ?? []
  const via = (i: { protocol: string; downloadClient?: string }) => {
    if (i.protocol === 'source') return 'Source'
    const client = clients.find((c) => c.id === i.downloadClient)
    return clients.length > 1 && client ? `Torrent · ${client.name}` : 'Torrent'
  }
  const items = queue.data ?? []
  const active = items.filter((i) => i.status !== 'imported' && i.status !== 'cancelled')
  const completed = items.filter((i) => i.status === 'imported' || i.status === 'cancelled')
  const shown = tab === 'history' ? completed : active
  const failed = active.filter((i) => i.status === 'failed').length
  const process = useMutation({
    mutationFn: api.processQueue,
    onSuccess: (r) => notify.ok(`${r.processed} ${r.processed === 1 ? 'item' : 'items'} imported`),
    onError: notify.err,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      void queryClient.invalidateQueries({ queryKey: ['series'] })
    },
  })
  const another = useMutation({
    mutationFn: async (ids: number[]) => {
      for (let i = 0; i < ids.length; i += 500) await api.tryAnotherRelease(ids.slice(i, i + 500))
    },
    onSuccess: () => notify.ok('Looking for other releases'),
    onError: notify.err,
    onSettled: () => {
      for (const key of ['queue', 'blocklist', 'series', 'review']) void queryClient.invalidateQueries({ queryKey: [key] })
    },
  })
  const unblock = useMutation({
    mutationFn: (id: number) => api.unblock(id),
    onSuccess: () => notify.ok('Release unblocked'),
    onError: notify.err,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['blocklist'] }),
  })
  // The stalled job keeps running and keeps its books until a replacement
  // is picked in the search; nothing is given up by looking.
  const recover = (item: QueueItem) => {
    const book = item.books?.[0]
    if (!book?.versionId) return
    setRecovery({ seriesID: item.seriesId, sel: {
      versionId: book.versionId, editionId: book.editionId, ledger: book.ledger as 'volume' | 'chapter',
      all: true, number: 0, monitored: true, wanted: true, fileIds: [], autoSearch: true,
      editionLabel: 'Replacement', replaceQueueId: item.id, replaceTitle: item.title,
    } })
  }
  const recoveryAction = (item: QueueItem) => (
    <Stack gap={4} mt="xs">
      {item.stalled && item.status === 'downloading' && <>
        <Text size="xs" c="orange">No progress for at least 20 minutes.</Text>
        <Button variant="default" size="compact-xs" style={{ alignSelf: 'flex-start' }} onClick={() => recover(item)}>Find another release</Button>
      </>}
      {item.canTryAnother && <Button variant="default" size="compact-xs" style={{ alignSelf: 'flex-start' }} loading={another.isPending} onClick={() => another.mutate([item.id])}>Try another release</Button>}
      {item.status === 'needs_review' && !item.canTryAnother && <Button variant="subtle" size="compact-xs" style={{ alignSelf: 'flex-start' }} onClick={() => setTab('review')}>Review files</Button>}
    </Stack>
  )
  const groups = new Map<number, QueueItem[]>()
  for (const item of active) {
    const jobs = groups.get(item.seriesId) ?? []
    jobs.push(item)
    groups.set(item.seriesId, jobs)
  }
  const renderJobs = (jobs: QueueItem[]) => (
    small ? (
      <Stack gap="sm">
        {jobs.map((i) => (
          <Card key={i.id} padding="md">
            <Group justify="space-between" mb="xs">
              <Badge size="sm" variant="dot" color={statusColor[i.status] ?? 'gray'}>
                {downloadLabel(i)}
              </Badge>
              <Text size="xs" c="dimmed">
                {via(i)} ·{' '}
                {fmtWhen(i.createdAt)}
              </Text>
            </Group>
            <Anchor
              component={Link}
              to={`/series/${i.seriesId}`}
              size="sm"
              fw={500}
              c="inherit"
              underline="hover"
              lineClamp={3}
              title={i.title}
              style={{ overflowWrap: 'anywhere' }}
            >
              {i.title}
            </Anchor>
            {i.status === 'downloading' && i.progress != null && (
              <Group gap="sm" wrap="nowrap" mt="md">
                <Progress
                  value={Math.max(0, Math.min(100, i.progress * 100))}
                  size={4}
                  style={{ flex: 1 }}
                  aria-label={`Download progress for ${i.title}`}
                />
                <Text size="xs" c="dimmed">
                  {Math.round(Math.max(0, Math.min(1, i.progress)) * 100)}%
                </Text>
              </Group>
            )}
            {i.error && (
              <div style={{ marginTop: 12 }}>
                <ErrorDetails error={i.error} title="Download error" />
              </div>
            )}
            {recoveryAction(i)}
          </Card>
        ))}
      </Stack>
    ) : (
      <Table
        highlightOnHover
        verticalSpacing="md"
        className="dataTable"
        style={{ tableLayout: 'fixed' }}
      >
        <Table.Thead>
          <Table.Tr>
            <Table.Th w="50%">Release</Table.Th>
            <Table.Th w="12%">Type</Table.Th>
            <Table.Th w="25%">Status</Table.Th>
            <Table.Th w="13%">Added</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {jobs.map((i) => (
            <Table.Tr key={i.id}>
              <Table.Td>
                <Stack gap={5} style={{ minWidth: 0 }}>
                  <Anchor
                    component={Link}
                    to={`/series/${i.seriesId}`}
                    size="sm"
                    c="inherit"
                    underline="hover"
                    lineClamp={2}
                    title={i.title}
                    style={{ overflowWrap: 'anywhere' }}
                  >
                    {i.title}
                  </Anchor>
                  {i.error && <ErrorDetails error={i.error} title="Download error" />}
                </Stack>
              </Table.Td>
              <Table.Td>
                <Text size="xs" c="dimmed">
                  {via(i)}
                </Text>
              </Table.Td>
              <Table.Td>
                <Stack gap={8}>
                  <Badge
                    size="sm"
                    variant="dot"
                    color={statusColor[i.status] ?? 'gray'}
                  >
                    {downloadLabel(i)}
                  </Badge>
                  {recoveryAction(i)}
                  {i.status === 'downloading' && i.progress != null && (
                    <Group gap={8} wrap="nowrap" maw={160}>
                      <Progress
                        value={Math.max(0, Math.min(100, i.progress * 100))}
                        size={4}
                        style={{ flex: 1 }}
                        aria-label={`Download progress for ${i.title}`}
                      />
                      <Text size="xs" c="dimmed" className="tabnums">
                        {Math.round(Math.max(0, Math.min(1, i.progress)) * 100)}%
                      </Text>
                    </Group>
                  )}
                </Stack>
              </Table.Td>
              <Table.Td>
                <Text size="xs" c="dimmed" title={i.createdAt}>
                  {fmtWhen(i.createdAt)}
                </Text>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    )
  )
  return (
    <>
      <PageHeader
        title="Activity"
        description="Downloads and imports."
        action={
          <Tooltip label="Check finished downloads and import available files. Downloads are also checked automatically.">
            <Button
              size="sm"
              variant="default"
              leftSection={<IconRefresh size={15} />}
              onClick={() => process.mutate()}
              loading={process.isPending}
            >
              Check downloads
            </Button>
          </Tooltip>
        }
      />
      <Tabs value={tab} onChange={setTab}>
        <Tabs.List mb="lg">
          <Tabs.Tab value="queue">
            Queue <span className="tabCount">{active.length}</span>
          </Tabs.Tab>
          <Tabs.Tab value="review">Review</Tabs.Tab>
          <Tabs.Tab value="history">
            Completed <span className="tabCount">{completed.length}</span>
          </Tabs.Tab>
          <Tabs.Tab value="blocked">
            Blocked releases <span className="tabCount">{blocked.data?.length ?? 0}</span>
          </Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel value="queue">
          {failed > 0 && (
            <Text size="sm" c="red" mb="md">
              {failed} {failed === 1 ? 'download needs' : 'downloads need'} attention. Review the
              error or try another release.
            </Text>
          )}
        </Tabs.Panel>
        {tab !== 'blocked' && tab !== 'review' && (
          <>
            <QueryFeedback
              pending={queue.isPending}
              error={queue.error}
              onRetry={() => void queue.refetch()}
            />
            {!queue.isPending && !queue.error && shown.length === 0 && (
              <EmptyState
                icon={<IconDownload size={26} stroke={1.5} />}
                title={tab === 'history' ? 'No completed imports' : 'No downloads in progress'}
                description={
                  tab === 'history'
                    ? 'Finished imports will appear here.'
                    : 'Releases you download will appear here. Sento imports them when they finish.'
                }
                action={
                  tab === 'queue' ? (
                    <Button component={Link} to="/" variant="default" size="sm">
                      Browse library
                    </Button>
                  ) : undefined
                }
              />
            )}
            {shown.length > 0 && (
              <>
                {tab === 'queue' ? <Stack gap="md">{[...groups].map(([seriesID, jobs]) => (
                  <QueueSeriesGroup key={seriesID} items={jobs} renderJobs={renderJobs} onAnother={(ids) => another.mutate(ids)} pending={another.isPending} />
                ))}</Stack> : renderJobs(shown)}
                {tab === 'history' && (
                  <Text size="xs" c="dimmed" mt="sm">
                    The 200 most recent finished downloads.
                  </Text>
                )}
              </>
            )}
          </>
        )}
        <Tabs.Panel value="review">
          <ReviewInbox />
        </Tabs.Panel>
        <Tabs.Panel value="blocked">
          <QueryFeedback
            pending={blocked.isPending}
            error={blocked.error}
            onRetry={() => void blocked.refetch()}
          />
          {blocked.data?.length === 0 && !blocked.error && (
            <EmptyState
              icon={<IconBan size={26} stroke={1.5} />}
              title="No blocked releases"
              description="Block a failed release to prevent Sento from downloading it again."
            />
          )}
          {(blocked.data?.length ?? 0) > 0 && (
            <Stack gap="sm">
              {blocked.data?.map((b) => (
                <Card key={b.id} padding="md">
                  <Group justify="space-between" align="flex-start" wrap="nowrap">
                    <Stack gap={4} style={{ minWidth: 0, flex: 1 }}>
                      <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
                        {b.key}
                      </Text>
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        {b.reason}
                      </Text>
                      <Text size="xs" c="dimmed">
                        {b.protocol === 'source' ? 'Source' : 'Torrent'}
                      </Text>
                    </Stack>
                    <Button
                      size="xs"
                      variant="default"
                      loading={unblock.isPending && unblock.variables === b.id}
                      disabled={unblock.isPending}
                      onClick={() => unblock.mutate(b.id)}
                    >
                      Unblock
                    </Button>
                  </Group>
                </Card>
              ))}
            </Stack>
          )}
        </Tabs.Panel>
      </Tabs>
      {recovery && <BookDialog seriesID={recovery.seriesID} sel={recovery.sel} files={[]} onClose={() => setRecovery(null)} onChanged={() => { void queryClient.invalidateQueries({ queryKey: ['queue'] }); void queryClient.invalidateQueries({ queryKey: ['series'] }) }} />}
    </>
  )
}
