import { useState } from 'react'
import { Badge, Button, Group, Menu, Progress, Stack, Text } from '@mantine/core'
import { IconDotsVertical } from '@tabler/icons-react'
import type { ActivityDownload } from '../types/service'
import { fmtWhen } from '../ui'
import { activityTime, downloadLabel } from './activityModel'
import ReviewQuestions from './ReviewQuestions'
import classes from './Activity.module.css'

export type DownloadActions = {
  another: (ids: number[]) => void
  retry: (ids: number | number[]) => void
  cancel: (download: ActivityDownload) => void
  replace: (download: ActivityDownload) => void
  attach: (download: ActivityDownload) => void
  recheck: (seriesID: number) => void
  sources: (seriesID: number) => void
  busy: boolean
}

function retryWhen(value: string) {
  const seconds = Math.max(0, Math.ceil((activityTime(value) - Date.now()) / 1000))
  if (!Number.isFinite(seconds) || seconds < 60) return 'Retrying shortly'
  if (seconds < 3600) return `Retrying in ${Math.ceil(seconds / 60)} min`
  return `Retrying in ${Math.ceil(seconds / 3600)} hr`
}

// Every context uses this row, including its file questions and recovery actions.
export function DownloadActivityRow({
  download: d,
  actions,
  showSeries = false,
}: {
  download: ActivityDownload
  actions: DownloadActions
  showSeries?: boolean
}) {
  const [filesOpen, setFilesOpen] = useState(false)
  const job = d.job
  const color = d.history
    ? job.status === 'imported'
      ? 'green'
      : 'gray'
    : d.attention
      ? 'orange'
      : job.status === 'importing'
        ? 'cyan'
        : 'blue'
  const progress = Math.max(0, Math.min(100, (job.progress ?? job.downloadProgress ?? 0) * 100))
  const hasProgress = job.status === 'downloading' && (job.progress != null || !!job.observedAt)
  return (
    <div className={classes.row} data-testid="download-row">
      <Stack gap={5} className={classes.content}>
        <Text size="sm" fw={500} className={classes.title}>
          {job.title || 'Untitled release'}
        </Text>
        <Group gap={8}>
          <Badge size="xs" variant="light" color={color}>
            {downloadLabel(d)}
          </Badge>
          <Text size="xs" c="dimmed">
            {[
              showSeries && job.seriesTitle,
              d.sourceName || (job.protocol === 'source' ? 'Source' : 'Torrent'),
              fmtWhen(job.createdAt),
            ]
              .filter(Boolean)
              .join(' · ')}
          </Text>
        </Group>
        {d.history && d.resolution && (
          <Text size="xs" c="dimmed">
            {d.resolution}
          </Text>
        )}
        {!d.history && job.status === 'retry_wait' && (
          <Text size="xs" c="dimmed">
            {retryWhen(job.retryAt)}. Sento will try this download again automatically.
          </Text>
        )}
        {!d.history && d.attention && d.explanation && (
          <Text size="sm" c="orange">
            {d.explanation}
          </Text>
        )}
        {!d.history && !!d.unresolvedBooks?.length && (
          <Text size="xs" c="dimmed">
            Still unmatched: {d.unresolvedBooks.slice(0, 8).map((book) => `${book.ledger} ${book.number / 100}`).join(', ')}{d.unresolvedBooks.length > 8 ? ` and ${d.unresolvedBooks.length - 8} more` : ''}.
          </Text>
        )}
        {hasProgress && (
          <Group gap={8} wrap="nowrap" maw={240}>
            <Progress
              value={progress}
              size={4}
              style={{ flex: 1 }}
              aria-label={`Download progress for ${job.title}`}
            />
            <Text size="xs" c="dimmed" className="tabnums">
              {Math.round(progress)}%
            </Text>
          </Group>
        )}
      </Stack>
      <div className={classes.actions}>
        {!d.history && (
          <>
            {d.files.length > 0 ? (
              <Button
                size="compact-xs"
                variant="default"
                aria-expanded={filesOpen}
                onClick={() => setFilesOpen(!filesOpen)}
              >
                {filesOpen
                  ? 'Hide files'
                  : `Review ${d.files.length} ${d.files.length === 1 ? 'file' : 'files'}`}
              </Button>
            ) : job.status === 'dispatch_unknown' ? (
              <Button size="compact-xs" variant="default" onClick={() => actions.attach(d)}>
                Confirm download
              </Button>
            ) : job.protocol === 'source' && d.issue === 'access' ? (
              <Button
                size="compact-xs"
                variant="default"
                onClick={() => actions.sources(job.seriesId)}
              >
                Check source settings
              </Button>
            ) : job.canTryAnother &&
              (job.status === 'failed' || ['gone', 'damaged'].includes(d.issue)) ? (
              <Button
                size="compact-xs"
                variant="default"
                disabled={actions.busy}
                onClick={() => actions.another([job.id])}
              >
                Try another release
              </Button>
            ) : job.stalled && job.status === 'downloading' && job.books?.[0]?.versionId ? (
              <Button size="compact-xs" variant="default" onClick={() => actions.replace(d)}>
                Choose a replacement
              </Button>
            ) : ['retry_wait', 'needs_review'].includes(job.status) ? (
              <Button
                size="compact-xs"
                variant="default"
                disabled={actions.busy}
                onClick={() => actions.retry(job.id)}
              >
                Retry now
              </Button>
            ) : null}
            {job.status !== 'importing' && (
              <Menu position="bottom-end">
                <Menu.Target>
                  <Button
                    variant="subtle"
                    size="compact-xs"
                    aria-label={`More actions for ${job.title}`}
                  >
                    <IconDotsVertical size={15} />
                  </Button>
                </Menu.Target>
                <Menu.Dropdown>
                  {job.canTryAnother &&
                    !['failed'].includes(job.status) &&
                    !['gone', 'damaged'].includes(d.issue) && (
                      <Menu.Item onClick={() => actions.another([job.id])}>
                        Try another release
                      </Menu.Item>
                    )}
                  {!!d.importedBooks && !!d.unresolvedBooks?.length && (
                    <Menu.Item onClick={() => actions.replace(d)}>
                      Find releases for unmatched books
                    </Menu.Item>
                  )}
                  {d.files.length > 0 && (
                    <Menu.Item onClick={() => actions.recheck(job.seriesId)}>
                      Recheck files
                    </Menu.Item>
                  )}
                  <Menu.Item color="red" disabled={actions.busy} onClick={() => actions.cancel(d)}>
                    Stop tracking download
                  </Menu.Item>
                </Menu.Dropdown>
              </Menu>
            )}
          </>
        )}
      </div>
      {filesOpen && d.files.length > 0 && (
        <div className={`${classes.expanded} ${classes.files}`}>
          <Text size="sm" mb="xs">
            Resolve these files to finish this import. Files already confirmed stay in your library.
          </Text>
          <ReviewQuestions items={d.files} seriesID={job.seriesId} />
        </div>
      )}
      {(job.error || d.listingName || d.explanation) && (
        <details className={classes.expanded}>
          <summary
            style={{
              cursor: 'pointer',
              fontSize: 'var(--mantine-font-size-xs)',
              color: 'var(--mantine-color-dimmed)',
            }}
          >
            Details
          </summary>
          <Stack gap={6} mt="xs">
            {d.listingName && (
              <Text size="xs" c="dimmed">
                Source folder: {d.listingName}
              </Text>
            )}
            {job.error && (
              <Text component="pre" size="xs" className={classes.error}>
                {job.error}
              </Text>
            )}
            {job.updatedAt && (
              <Text size="xs" c="dimmed">
                Last updated {fmtWhen(job.updatedAt)}
              </Text>
            )}
          </Stack>
        </details>
      )}
    </div>
  )
}
