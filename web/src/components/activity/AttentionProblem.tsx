import {
  Anchor,
  Button,
  Group,
  Stack,
  Text,
} from '@mantine/core'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import classes from '../Activity.module.css'
import {
  type AttentionGroup,
} from '../activityModel'
import { DownloadActivityRow, type DownloadActions } from '../DownloadActivityRow'
import ReviewQuestions from '../ReviewQuestions'

export function AttentionProblem({
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
