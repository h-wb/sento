import { Anchor, Badge, Group, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Decision } from '../types/store'
import { fmtWhen } from '../ui'
import { QueryFeedback } from './QueryFeedback'
import { EmptyState } from './EmptyState'

const eventColor: Record<string, string> = {
  grabbed: 'indigo',
  imported: 'green',
  imported_after_review: 'green',
  import_failed: 'red',
  download_failed: 'red',
  download_retry: 'yellow',
  download_needs_review: 'orange',
  blocklisted: 'orange',
  editions_grouped: 'blue',
  listing_switched_off: 'orange',
}

export function SeriesHistory({
  seriesID,
  events: supplied,
  titles = {},
}: {
  seriesID: number
  events?: Decision[]
  titles?: Record<number, string>
}) {
  const q = useQuery({
    queryKey: ['series', seriesID, 'history'],
    queryFn: () => api.history(seriesID),
    enabled: !supplied,
  })
  if (!supplied && (q.isPending || q.error))
    return <QueryFeedback pending={q.isPending} error={q.error} onRetry={() => void q.refetch()} />
  const events = supplied ?? q.data ?? []
  if (!events.length)
    return (
      <EmptyState
        title="No decision history yet"
        description="Downloads, imports, and automatic decisions appear here."
      />
    )
  return (
    <Stack gap={0} mt="md">
      {events.map((event) => (
        <Stack
          key={event.id}
          gap={4}
          py="sm"
          style={{ borderTop: '1px solid var(--mantine-color-default-border)', minWidth: 0 }}
        >
          <Group gap="xs">
            <Badge size="xs" color={eventColor[event.body.event] ?? 'gray'}>
              {event.body.event.replaceAll('_', ' ')}
            </Badge>
            <Text size="xs" c="dimmed">
              {fmtWhen(event.createdAt)}
              {event.body.by ? (event.body.by === 'you' ? ' · chosen by you' : ' · automatic') : ''}
            </Text>
            {!seriesID && event.seriesId > 0 && (
              <Anchor
                component={Link}
                to={`/series/${event.seriesId}?view=activity&activity=history`}
                size="xs"
              >
                {titles[event.seriesId] || `Series ${event.seriesId}`}
              </Anchor>
            )}
          </Group>
          {event.body.title && (
            <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
              {event.body.title}
            </Text>
          )}
          {event.body.note && (
            <Text size="xs" c="dimmed">
              {event.body.note}
            </Text>
          )}
          {event.body.error && (
            <details>
              <summary style={{ cursor: 'pointer', fontSize: 'var(--mantine-font-size-xs)' }}>
                Details
              </summary>
              <Text
                size="xs"
                component="pre"
                style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
              >
                {event.body.error}
              </Text>
            </details>
          )}
        </Stack>
      ))}
    </Stack>
  )
}
