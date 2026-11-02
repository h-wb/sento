import { useState } from 'react'
import {
  ActionIcon,
  Anchor,
  Badge,
  Button,
  Group,
  Menu,
  Modal,
  Pagination,
  Progress,
  Stack,
  Tabs,
  Text,
  TextInput,
} from '@mantine/core'
import {
  IconChevronDown,
  IconChevronRight,
  IconDotsVertical,
  IconDownload,
  IconCheck,
} from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import type { ActivityDownload } from '../types/service'
import BookDialog, { type BookSelection } from './BookDialog'
import { QueryFeedback } from './QueryFeedback'
import { EmptyState } from './EmptyState'
import { useConfirmAction } from './ConfirmAction'
import { notify } from '../ui'
import { useActivity } from './useActivity'
import {
  activityTime,
  activityCounts,
  attentionGroups,
  downloadLabel,
  inProgress,
  type ActivityFilter,
  type AttentionGroup,
} from './activityModel'
import { DownloadActivityRow, type DownloadActions } from './DownloadActivityRow'
import ReviewQuestions from './ReviewQuestions'
import { SeriesHistory } from './SeriesHistory'
import classes from './Activity.module.css'

function ProgressSeries({
  downloads,
  title,
  scoped,
  actions,
}: {
  downloads: ActivityDownload[]
  title: string
  scoped: boolean
  actions: DownloadActions
}) {
  const [opened, setOpened] = useState(false)
  const [waitingOpen, setWaitingOpen] = useState(false)
  const waiting = downloads.filter((d) => ['queued', 'retry_wait'].includes(d.job.status) || ['waiting for source cooldown', 'waiting for download slot', 'queuedDL'].includes(d.job.downloadState))
  const waitingIDs = new Set(waiting.map((d) => d.job.id))
  const visible = waiting.length > 5 ? downloads.filter((d) => !waitingIDs.has(d.job.id)) : downloads
  const current = downloads.find(
    (d) =>
      d.job.status === 'downloading' &&
      d.job.progress != null &&
      !d.job.downloadState?.startsWith('waiting'),
  )
  const counts = new Map<string, number>()
  for (const d of downloads) {
    const label = downloadLabel(d).toLowerCase()
    counts.set(label, (counts.get(label) ?? 0) + 1)
  }
  return (
    <section className={classes.series} data-testid="activity-series">
      <div className={classes.header}>
        <ActionIcon
          variant="subtle"
          color="gray"
          aria-label={`${opened ? 'Hide' : 'Show'} downloads for ${title}`}
          aria-expanded={opened}
          onClick={() => setOpened(!opened)}
        >
          {opened ? <IconChevronDown size={18} /> : <IconChevronRight size={18} />}
        </ActionIcon>
        <div className={classes.headerText}>
          {scoped ? (
            <Text size="sm" fw={600}>
              Downloads
            </Text>
          ) : (
            <Anchor
              component={Link}
              to={`/series/${downloads[0].job.seriesId}?view=activity`}
              c="inherit"
              fw={600}
              className={classes.title}
            >
              {title}
            </Anchor>
          )}
          <Text size="xs" c="dimmed">
            {[...counts].map(([label, count]) => `${count} ${label}`).join(' · ')}
          </Text>
          {current && (
            <Progress
              mt={8}
              maw={260}
              size={3}
              value={Math.max(0, Math.min(100, current.job.progress! * 100))}
              aria-label={`Active download progress for ${title}`}
            />
          )}
        </div>
        <Badge variant="light" color="blue" size="sm">
          {downloads.length}
        </Badge>
      </div>
      {opened && (
        <div className={classes.rows}>
          {visible.map((d) => (
            <DownloadActivityRow key={d.job.id} download={d} actions={actions} />
          ))}
          {waiting.length > 5 && <>
            <Button variant="subtle" size="compact-xs" my="sm" aria-expanded={waitingOpen} onClick={() => setWaitingOpen(!waitingOpen)}>{waitingOpen ? 'Hide' : 'Show'} {waiting.length} waiting downloads</Button>
            {waitingOpen && waiting.map((d) => <DownloadActivityRow key={d.job.id} download={d} actions={actions} />)}
          </>}
        </div>
      )}
    </section>
  )
}

function AttentionProblem({
  group,
  actions,
  onSources,
}: {
  group: AttentionGroup
  actions: DownloadActions
  onSources?: () => void
}) {
  const [opened, setOpened] = useState(false)
  const first = group.downloads[0]
  const multiple = group.downloads.length > 1
  return (
    <section className={classes.series} data-testid="activity-problem">
      {multiple ? (
        <div className={classes.problem}>
          <Group justify="space-between" align="flex-start">
            <Stack gap={4} style={{ minWidth: 0, flex: 1 }}>
              <Text size="sm" fw={600}>
                {first.explanation}
              </Text>
              <Text size="xs" c="dimmed" className={classes.title}>
                {[
                  first.sourceName || 'Source',
                  first.listingName,
                  `${group.downloads.length} affected downloads`,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
            </Stack>
            {first.issue === 'access' ? (
              <Button
                size="compact-xs"
                variant="default"
                onClick={() => actions.sources(group.seriesID)}
              >
                Check source settings
              </Button>
            ) : group.downloads.every(
                (d) => d.job.status === 'failed' || ['gone', 'damaged'].includes(d.issue),
              ) ? (
              <Button
                size="compact-xs"
                variant="default"
                disabled={actions.busy}
                onClick={() =>
                  actions.another(
                    group.downloads.filter((d) => d.job.canTryAnother).map((d) => d.job.id),
                  )
                }
              >
                Try other releases
              </Button>
            ) : (
              <Button
                size="compact-xs"
                variant="default"
                disabled={actions.busy}
                onClick={() => actions.retry(group.downloads.map((d) => d.job.id))}
              >
                Retry downloads
              </Button>
            )}
          </Group>
          <Group gap="sm" mt="sm">
            <Button
              variant="subtle"
              size="compact-xs"
              aria-expanded={opened}
              onClick={() => setOpened(!opened)}
            >
              {opened ? 'Hide' : 'Show'} affected downloads
            </Button>
            {onSources ? (
              <Button variant="subtle" size="compact-xs" onClick={onSources}>
                Open sources
              </Button>
            ) : (
              <Anchor component={Link} to={`/series/${group.seriesID}?sources=1`} size="xs">
                Open sources
              </Anchor>
            )}
          </Group>
          {first.job.error && (
            <details style={{ marginTop: 8 }}>
              <summary style={{ cursor: 'pointer', fontSize: 'var(--mantine-font-size-xs)' }}>
                Details
              </summary>
              <Text component="pre" size="xs" className={classes.error}>
                {first.job.error}
              </Text>
            </details>
          )}
          {opened && (
            <div className={classes.problemRows}>
              {group.downloads.map((d) => (
                <DownloadActivityRow key={d.job.id} download={d} actions={actions} />
              ))}
            </div>
          )}
        </div>
      ) : first ? (
        <div className={classes.rows}>
          <DownloadActivityRow download={first} actions={actions} />
        </div>
      ) : (
        <div className={classes.problem}>
          <ReviewQuestions items={group.reviews} seriesID={group.seriesID} onSources={onSources} />
        </div>
      )}
    </section>
  )
}

function BlockedReleases() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['blocklist'], queryFn: api.blocklist })
  const unblock = useMutation({
    mutationFn: api.unblock,
    onError: notify.err,
    onSuccess: () => {
      notify.ok('Release unblocked')
      void qc.invalidateQueries({ queryKey: ['blocklist'] })
    },
  })
  return (
    <Stack gap="xs" mt="sm">
      <QueryFeedback pending={q.isPending} error={q.error} onRetry={() => void q.refetch()} />
      {q.data?.length === 0 && (
        <Text size="sm" c="dimmed">
          No blocked releases.
        </Text>
      )}
      {q.data?.map((b) => (
        <Group key={b.id} justify="space-between" align="flex-start" wrap="nowrap">
          <div style={{ minWidth: 0, flex: 1 }}>
            <Text size="xs" className={classes.title}>
              {b.key}
            </Text>
            <Text size="xs" c="dimmed">
              {b.reason}
            </Text>
          </div>
          <Button
            size="compact-xs"
            variant="default"
            loading={unblock.isPending && unblock.variables === b.id}
            onClick={() => unblock.mutate(b.id)}
          >
            Unblock
          </Button>
        </Group>
      ))}
    </Stack>
  )
}

export default function ActivityPanel({
  seriesID = 0,
  onSources,
  legacyFilter,
}: {
  seriesID?: number
  onSources?: () => void
  legacyFilter?: ActivityFilter
}) {
  const navigate = useNavigate()
  const q = useActivity(seriesID)
  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const counts = activityCounts(q.data)
  const value = params.get('activity')
  const filter: ActivityFilter =
    value === 'progress' || value === 'attention' || value === 'history'
      ? value
      : (legacyFilter ?? (counts.attention > 0 ? 'attention' : 'progress'))
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [blockedOpen, setBlockedOpen] = useState(false)
  const [recovery, setRecovery] = useState<{ seriesID: number; sel: BookSelection } | null>(null)
  const [attach, setAttach] = useState<ActivityDownload | null>(null)
  const [remoteID, setRemoteID] = useState('')
  const { confirm, confirmation } = useConfirmAction()
  const refresh = async () => {
    await Promise.all(
      ['activity', 'queue', 'series', 'review', 'blocklist'].map((key) =>
        qc.invalidateQueries({ queryKey: [key] }),
      ),
    )
  }
  const mutation = useMutation({
    mutationFn: async (work: () => Promise<unknown>) => work(),
    onError: notify.err,
    onSettled: refresh,
  })
  const another = (ids: number[]) =>
    mutation.mutate(async () => {
      for (let i = 0; i < ids.length; i += 500) await api.tryAnotherRelease(ids.slice(i, i + 500))
      notify.ok('Looking for other releases')
    })
  const actions: DownloadActions = {
    busy: mutation.isPending,
    sources: (sid) => {
      if (sid === seriesID && onSources) onSources()
      else navigate(`/series/${sid}?sources=1`)
    },
    another,
    retry: (ids) =>
      mutation.mutate(async () => {
        for (const id of Array.isArray(ids) ? ids : [ids]) await api.retryQueue(id)
        notify.ok('Downloads will be checked again')
      }),
    recheck: (id) => mutation.mutate(() => api.checkFiles(id)),
    attach: (d) => {
      setAttach(d)
      setRemoteID('')
    },
    cancel: (d) =>
      confirm({
        title: 'Stop tracking this download?',
        message:
          'Release its book reservations. Files in your library stay. Torrents and their files remain in the download client.',
        label: 'Stop tracking',
        onConfirm: async () => {
          await api.cancelQueue(d.job.id)
          await refresh()
        },
      }),
    replace: (d) => {
      const book = d.unresolvedBooks?.[0] ?? d.job.books?.[0]
      if (!book?.versionId) return
      setRecovery({
        seriesID: d.job.seriesId,
        sel: {
          versionId: book.versionId,
          editionId: book.editionId,
          ledger: book.ledger as 'volume' | 'chapter',
          all: true,
          number: 0,
          monitored: true,
          wanted: true,
          fileIds: [],
          autoSearch: true,
          editionLabel: 'Replacement',
          replaceQueueId: d.job.id,
          replaceTitle: d.job.title,
        },
      })
    },
  }
  const setFilter = (next: string | null) => {
    if (!next) return
    setPage(1)
    setParams(
      (previous) => {
        const p = new URLSearchParams(previous)
        p.set('activity', next)
        if (seriesID) p.set('view', 'activity')
        return p
      },
      { replace: true },
    )
  }
  const matches = (title: string) => title.toLowerCase().includes(search.toLowerCase())
  const progress = (q.data?.downloads ?? [])
    .filter(inProgress)
    .filter((d) => matches(`${d.job.seriesTitle} ${d.job.title}`))
  const progressGroups = new Map<number, ActivityDownload[]>()
  for (const d of progress) {
    const list = progressGroups.get(d.job.seriesId) ?? []
    list.push(d)
    progressGroups.set(d.job.seriesId, list)
  }
  const attention = attentionGroups(q.data).filter((g) =>
    matches(
      `${g.title} ${g.downloads.map((d) => `${d.job.title} ${d.sourceName} ${d.explanation}`).join(' ')} ${g.reviews.map((r) => r.title).join(' ')}`,
    ),
  )
  const attentionSeries = new Map<number, AttentionGroup[]>()
  for (const g of attention) {
    const list = attentionSeries.get(g.seriesID) ?? []
    list.push(g)
    attentionSeries.set(g.seriesID, list)
  }
  const history = (q.data?.downloads ?? [])
    .filter((d) => d.history)
    .filter((d) => matches(`${d.job.seriesTitle} ${d.job.title}`))
    .sort(
      (a, b) =>
        activityTime(b.job.updatedAt || b.job.createdAt) -
          activityTime(a.job.updatedAt || a.job.createdAt) || b.job.id - a.job.id,
    )
  const historyPage = Math.min(page, Math.max(1, Math.ceil(history.length / 30)))
  const shown =
    filter === 'attention'
      ? attention.length
      : filter === 'history'
        ? history.length
        : progress.length
  return (
    <Tabs value={filter} onChange={setFilter} keepMounted={false}>
      <Stack gap="md">
        <Tabs.List className={classes.tabsList}>
          <Tabs.Tab value="progress">
            In progress <span className="tabCount">{counts.progress}</span>
          </Tabs.Tab>
          <Tabs.Tab value="attention">
            Needs attention <span className="tabCount">{counts.attention}</span>
          </Tabs.Tab>
          <Tabs.Tab value="history">History</Tabs.Tab>
        </Tabs.List>
        <div className={classes.toolbar}>
          <TextInput
            placeholder="Filter activity"
            aria-label="Filter activity"
            value={search}
            onChange={(event) => {
              setSearch(event.currentTarget.value)
              setPage(1)
            }}
            maw={360}
          />
          <Menu position="bottom-end">
            <Menu.Target>
              <ActionIcon variant="default" size="lg" aria-label="Activity actions">
                <IconDotsVertical size={17} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item onClick={() => void q.refetch()}>Refresh activity</Menu.Item>
              {!seriesID && (
                <Menu.Item
                  onClick={() =>
                    mutation.mutate(async () => {
                      const r = await api.processQueue()
                      notify.ok(`${r.processed} imported`)
                    })
                  }
                >
                  Check finished downloads
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        </div>
        <QueryFeedback pending={q.isPending} error={q.error} onRetry={() => void q.refetch()} />
        {!q.isPending && !q.error && shown === 0 && (
          <EmptyState
            icon={filter === 'attention' ? <IconCheck size={26} /> : <IconDownload size={26} />}
            title={
              search
                ? 'No matching activity'
                : filter === 'attention'
                  ? 'Nothing needs attention'
                  : filter === 'history'
                    ? 'No download history yet'
                    : 'No downloads in progress'
            }
            description={
              search
                ? 'Try another title or source.'
                : filter === 'attention'
                  ? 'Downloads that can recover automatically stay in progress.'
                  : filter === 'history'
                    ? 'Finished downloads and previous attempts appear here.'
                    : 'Downloads and imports will appear here as they happen.'
            }
          />
        )}
        <Tabs.Panel value="progress">
          <Stack gap="md">
            {' '}
            {!q.error &&
              [...progressGroups].map(([sid, downloads]) => (
                <ProgressSeries
                  key={sid}
                  downloads={downloads}
                  title={
                    q.data?.seriesTitles[sid] || downloads[0].job.seriesTitle || `Series ${sid}`
                  }
                  scoped={!!seriesID}
                  actions={actions}
                />
              ))}
          </Stack>
        </Tabs.Panel>
        <Tabs.Panel value="attention">
          <Stack gap="md">
            {' '}
            {!q.error &&
              [...attentionSeries].map(([sid, groups]) => (
                <Stack key={sid} gap="sm">
                  {!seriesID && (
                    <Group gap="sm">
                      <Anchor
                        component={Link}
                        to={`/series/${sid}?view=activity&activity=attention`}
                        c="inherit"
                        fw={600}
                      >
                        {groups[0].title}
                      </Anchor>
                      <Text size="xs" c="dimmed">
                        {groups.length} {groups.length === 1 ? 'issue' : 'issues'}
                      </Text>
                    </Group>
                  )}
                  {groups.map((group) => (
                    <AttentionProblem
                      key={group.key}
                      group={group}
                      actions={actions}
                      onSources={onSources}
                    />
                  ))}
                </Stack>
              ))}
          </Stack>
        </Tabs.Panel>
        <Tabs.Panel value="history">
          <Stack gap="md">
            {' '}
            {!q.error && (
              <>
                {history.length > 0 && (
                  <div className={classes.rows}>
                    {history.slice((historyPage - 1) * 30, historyPage * 30).map((d) => (
                      <DownloadActivityRow
                        key={d.job.id}
                        download={d}
                        actions={actions}
                        showSeries={!seriesID}
                      />
                    ))}
                  </div>
                )}
                {history.length > 30 && (
                  <Pagination
                    total={Math.ceil(history.length / 30)}
                    value={historyPage}
                    onChange={setPage}
                    size="sm"
                  />
                )}
                <details>
                  <summary style={{ cursor: 'pointer' }}>Decision history</summary>
                  <SeriesHistory
                    seriesID={seriesID}
                    events={q.data?.events}
                    titles={q.data?.seriesTitles}
                  />
                </details>
                {!seriesID && (
                  <details onToggle={(event) => setBlockedOpen(event.currentTarget.open)}>
                    <summary style={{ cursor: 'pointer' }}>Blocked releases</summary>
                    {blockedOpen && <BlockedReleases />}
                  </details>
                )}
              </>
            )}
          </Stack>
        </Tabs.Panel>
        {confirmation}
        <Modal
          opened={!!attach}
          onClose={() => {
            if (!mutation.isPending) setAttach(null)
          }}
          title="Confirm existing download"
        >
          <Stack>
            <Text size="sm">
              Find this download in your client and enter its existing ID. Sento will check it
              without sending another download.
            </Text>
            <TextInput
              label="Client download ID"
              value={remoteID}
              onChange={(event) => setRemoteID(event.currentTarget.value)}
            />
            <Button
              disabled={!remoteID.trim()}
              loading={mutation.isPending}
              onClick={() =>
                mutation.mutate(async () => {
                  await api.retryQueue(attach!.job.id, remoteID.trim())
                  setAttach(null)
                })
              }
            >
              Attach download
            </Button>
          </Stack>
        </Modal>
        {recovery && (
          <BookDialog
            seriesID={recovery.seriesID}
            sel={recovery.sel}
            files={[]}
            onClose={() => setRecovery(null)}
            onChanged={() => void refresh()}
          />
        )}
      </Stack>
    </Tabs>
  )
}
