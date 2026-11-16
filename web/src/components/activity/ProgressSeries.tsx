import { ActionIcon, Anchor, Badge, Button, Progress, Text } from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { ActivityDownload } from '../../types/service'
import classes from '../Activity.module.css'
import { downloadLabel } from '../activityModel'
import { DownloadActivityRow, type DownloadActions } from '../DownloadActivityRow'

export function ProgressSeries({
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
  const waiting = downloads.filter(
    (d) =>
      ['queued', 'retry_wait'].includes(d.job.status) ||
      ['waiting for source cooldown', 'waiting for download slot', 'queuedDL'].includes(
        d.job.downloadState,
      ),
  )
  const waitingIDs = new Set(waiting.map((d) => d.job.id))
  const visible =
    waiting.length > 5 ? downloads.filter((d) => !waitingIDs.has(d.job.id)) : downloads
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
          {waiting.length > 5 && (
            <>
              <Button
                variant="subtle"
                size="compact-xs"
                my="sm"
                aria-expanded={waitingOpen}
                onClick={() => setWaitingOpen(!waitingOpen)}
              >
                {waitingOpen ? 'Hide' : 'Show'} {waiting.length} waiting downloads
              </Button>
              {waitingOpen &&
                waiting.map((d) => (
                  <DownloadActivityRow key={d.job.id} download={d} actions={actions} />
                ))}
            </>
          )}
        </div>
      )}
    </section>
  )
}
