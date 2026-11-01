import { Badge, Table, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
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

// The series' history: what was grabbed, imported or refused, newest first.
export function SeriesHistory({ seriesID }: { seriesID: number }) {
  const {
    data: events,
    error,
    isPending,
    refetch,
  } = useQuery({
    queryKey: ['series', seriesID, 'history'],
    queryFn: () => api.history(seriesID),
  })

  if (isPending || error)
    return <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
  if (events && events.length === 0)
    return (
      <EmptyState
        title="No history yet"
        description="Downloads, imports, and errors for this series will appear here."
      />
    )
  return (
    <Table.ScrollContainer minWidth={580}>
      <Table highlightOnHover verticalSpacing="sm">
        <Table.Thead>
          <Table.Tr>
            <Table.Th>When</Table.Th>
            <Table.Th>Event</Table.Th>
            <Table.Th>Release</Table.Th>
            <Table.Th>Site</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {events?.map((d) => (
            <Table.Tr key={d.id}>
              <Table.Td>
                <Text size="xs" c="dimmed" title={d.createdAt}>
                  {fmtWhen(d.createdAt)}
                </Text>
              </Table.Td>
              <Table.Td>
                <Badge size="xs" color={eventColor[d.body.event] ?? 'gray'}>
                  {d.body.event.replaceAll('_', ' ')}
                  {d.body.force ? ' (forced)' : ''}
                </Badge>
                {d.body.by && (
                  <Text size="xs" c="dimmed" mt={2}>
                    {d.body.by === 'you' ? 'by you' : 'automatically'}
                  </Text>
                )}
              </Table.Td>
              <Table.Td>
                <Text size="sm" lineClamp={1} title={d.body.title}>
                  {d.body.title}
                </Text>
                {d.body.error && (
                  <Text size="xs" c="red" lineClamp={1} title={d.body.error}>
                    {d.body.error}
                  </Text>
                )}
                {d.body.note && (
                  <Text size="xs" c="dimmed">
                    {d.body.note}
                  </Text>
                )}
              </Table.Td>
              <Table.Td>
                <Text size="xs" c="dimmed">
                  {d.body.provider}
                  {d.body.site ? `/${d.body.site}` : ''}
                </Text>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  )
}
