import { filterActivity } from './activityModel'
import {
  ActionIcon,
  Anchor,
  Group,
  Menu,
  Pagination,
  Stack,
  Tabs,
  Text,
  TextInput,
} from '@mantine/core'
import {
  IconCheck,
  IconDotsVertical,
  IconDownload,
} from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { plural } from '../format'
import { invalidateLibrary } from '../queries'
import type { ActivityDownload } from '../types/service'
import type { ReviewItem } from '../types/store'
import { notify } from '../ui'
import classes from './Activity.module.css'
import { AttachDownloadDialog } from './activity/AttachDownloadDialog'
import { AttentionProblem } from './activity/AttentionProblem'
import { BlockedReleases } from './activity/BlockedReleases'
import { ProgressSeries } from './activity/ProgressSeries'
import { useDownloadActions } from './activity/useDownloadActions'
import {
  activityCounts,
  type ActivityFilter,
} from './activityModel'
import BookDialog,{ type BookSelection } from './BookDialog'
import { useConfirmAction } from './ConfirmAction'
import { DownloadActivityRow } from './DownloadActivityRow'
import { EmptyState } from './EmptyState'
import { FileReviewDialog } from './FileReviewDialog'
import { QueryFeedback } from './QueryFeedback'
import { ReleaseMatchDialog } from './ReleaseMatchDialog'
import { ReviewDialogsContext, type ReleaseReview } from './reviewDialogs'
import { SeriesHistory } from './SeriesHistory'
import { useActivity } from './useActivity'
import { usePaged } from './usePaged'

export default function ActivityPanel({
  seriesID = 0,
  onSources,
  legacyFilter,
}: {
  seriesID?: number
  onSources?: () => void
  legacyFilter?: ActivityFilter
}) {
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
  const [blockedOpen, setBlockedOpen] = useState(false)
  const [recovery, setRecovery] = useState<{ seriesID: number; sel: BookSelection } | null>(null)
  const [attach, setAttach] = useState<ActivityDownload | null>(null)
  // An answered row can disappear on the next poll. Keep its editor and
  // saved-file receipt mounted until the person dismisses the dialog.
  const [fileReview, setFileReview] = useState<ReviewItem | null>(null)
  const [releaseReview, setReleaseReview] = useState<ReleaseReview | null>(null)
  const [remoteID, setRemoteID] = useState('')
  const { confirm, confirmation } = useConfirmAction()
  const refresh = async () => {
    await invalidateLibrary(qc)
  }
  const mutation = useMutation({
    mutationFn: async (work: () => Promise<unknown>) => work(),
    onError: notify.err,
    onSettled: refresh,
  })
  const actions = useDownloadActions({ seriesID, onSources, mutation, confirm, refresh, setAttach, setRemoteID, setRecovery })
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
  const { progress, progressGroups, attention, attentionSeries, history } = filterActivity(q.data, search)
  const { page, setPage, slice: historySlice } = usePaged(history, 30)
  const shown =
    filter === 'attention'
      ? attention.length
      : filter === 'history'
        ? history.length
        : progress.length
  const content = (
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
                        {groups.length} {plural(groups.length, 'issue')}
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
                    {historySlice.map((d) => (
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
                    value={page}
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
        <AttachDownloadDialog attach={attach} setAttach={setAttach} remoteID={remoteID} setRemoteID={setRemoteID} mutation={mutation} />
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
  return (
    <ReviewDialogsContext.Provider value={{ file: setFileReview, release: setReleaseReview }}>
      {content}
      {fileReview && <FileReviewDialog item={fileReview} close={() => setFileReview(null)} />}
      {releaseReview && <ReleaseMatchDialog
        seriesID={releaseReview.seriesID} title={releaseReview.title} files={releaseReview.files}
        close={() => setReleaseReview(null)}
      />}
    </ReviewDialogsContext.Provider>
  )
}
