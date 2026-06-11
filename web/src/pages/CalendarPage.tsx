import { Alert, Badge, Card, Group, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, fmtNum } from '../api'
import { fmtDay, usePageTitle } from '../ui'

// Upcoming (and recent) dated volume releases of monitored collections,
// grouped by date — Sonarr's calendar, list-shaped.
export default function CalendarPage() {
  usePageTitle('Calendar')
  const { data: entries, error } = useQuery({
    queryKey: ['calendar'],
    queryFn: () => api.calendar(),
  })

  const today = new Date().toISOString().slice(0, 10)
  const byDate = new Map<string, NonNullable<typeof entries>>()
  for (const e of entries ?? []) {
    const list = byDate.get(e.releaseDate) ?? []
    list.push(e)
    byDate.set(e.releaseDate, list)
  }

  return (
    <>
      <Title order={2} mb="md">
        Calendar
      </Title>
      {error && <Alert color="red">{String(error)}</Alert>}
      {entries && entries.length === 0 && (
        <Text c="dimmed">
          No dated releases in range — the calendar fills in as monitored editions get release
          dates from MangaBaka.
        </Text>
      )}
      <Stack gap="md">
        {[...byDate.entries()].map(([date, list]) => (
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
                  key={`${e.collectionId}-${e.sequenceNum}`}
                  withBorder
                  padding="xs"
                  component={Link}
                  to={`/series/${e.seriesId}`}
                  style={{ textDecoration: 'none' }}
                >
                  <Group justify="space-between" wrap="nowrap">
                    <Text size="sm">
                      {e.seriesTitle} — Volume {e.sequenceStr || fmtNum(e.sequenceNum)}
                    </Text>
                    <Group gap={6} wrap="nowrap">
                      <Badge size="xs" variant="light">
                        {e.editionName}
                      </Badge>
                      <Badge size="xs" color={e.owned ? 'green' : date <= today ? 'yellow' : 'gray'}>
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
