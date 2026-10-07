import { Badge, Button, Card, Group, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, fmtNum } from '../api'
import { fmtDay, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'

// Upcoming (and recent) dated volume releases of monitored collections,
// grouped by date — Sonarr's calendar, list-shaped.
export default function CalendarPage() {
  usePageTitle('Calendar')
  const {
    data: entries,
    error,
    isPending,
    refetch,
  } = useQuery({
    queryKey: ['calendar'],
    queryFn: () => api.calendar(),
  })

  const now = new Date()
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  const byDate = new Map<string, NonNullable<typeof entries>>()
  for (const e of entries ?? []) {
    const list = byDate.get(e.releaseDate) ?? []
    list.push(e)
    byDate.set(e.releaseDate, list)
  }

  return (
    <>
      <PageHeader title="Calendar" description="Release dates for the editions you monitor." />
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {entries?.length === 0 && !error && (
        <EmptyState
          title="No scheduled releases"
          description="Release dates appear here when they're available for your monitored editions."
          action={
            <Button component={Link} to="/" variant="default" size="sm">
              Browse library
            </Button>
          }
        />
      )}
      <Stack gap="md">
        {[...byDate.entries()]
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([date, list]) => (
            <div key={date}>
              <Group gap={8} mb={6}>
                <Text fw={600} size="sm" title={date}>
                  {fmtDay(date)}
                </Text>
                {date === today && (
                  <Badge size="xs" color="indigo">
                    today
                  </Badge>
                )}
                {date < today && (
                  <Badge size="xs" color="gray" variant="light">
                    released
                  </Badge>
                )}
              </Group>
              <Stack gap={6}>
                {list.map((e) => (
                  <Card
                    key={`${e.versionId}-${e.bookId}`}
                    withBorder
                    padding="xs"
                    component={Link}
                    to={`/series/${e.seriesId}`}
                    style={{ textDecoration: 'none' }}
                  >
                    <Group justify="space-between">
                      <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
                        {e.seriesTitle} — Volume {e.sequenceStr || fmtNum(e.sequenceNum)}
                      </Text>
                      <Group gap={6} wrap="nowrap">
                        <Badge size="xs" variant="light">
                          {e.editionName}
                          {e.versionName !== 'Default' ? ` · ${e.versionName}` : ''}
                        </Badge>
                        <Badge
                          size="xs"
                          color={e.owned ? 'green' : date <= today ? 'yellow' : 'gray'}
                        >
                          {e.owned ? 'owned' : date <= today ? 'wanted' : 'upcoming'}
                        </Badge>
                      </Group>
                    </Group>
                  </Card>
                ))}
              </Stack>
            </div>
          ))}
      </Stack>
    </>
  )
}
