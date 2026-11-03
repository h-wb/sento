import { Badge, Group, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import { QueryFeedback } from './QueryFeedback'

export function FileScoreDetails({ seriesID, versionID, fileID }: {
  seriesID: number
  versionID: number
  fileID: number
}) {
  const score = useQuery({
    queryKey: ['series', seriesID, 'versions', versionID, 'files', fileID, 'score'],
    queryFn: () => api.fileScore(seriesID, versionID, fileID),
  })
  if (!score.data) return (
    <QueryFeedback pending={score.isPending} error={score.error} onRetry={() => void score.refetch()} />
  )
  const s = score.data
  if (!s.profileId) return <Text size="xs" c="dimmed">No release profile configured.</Text>
  return (
    <Stack gap={4} mt="xs" role="group" aria-label="Current file score">
      <Group gap={6}>
        <Text size="sm" fw={500}>Current score: {s.score}</Text>
        <Text size="xs" c="dimmed">Profile: {s.profileName}</Text>
      </Group>
      <Group gap={4}>
        {s.formats.map((f) => (
          <Badge
            key={f.name}
            size="sm"
            variant="light"
            color={f.score < 0 ? 'orange' : 'gray'}
            styles={{
              root: { maxWidth: '100%', height: 'auto' },
              label: { whiteSpace: 'normal', overflowWrap: 'anywhere' },
            }}
          >
            {f.name}: {f.score > 0 ? '+' : ''}{f.score}
          </Badge>
        ))}
        {!s.formats.length && <Text size="xs" c="dimmed">No custom formats matched.</Text>}
      </Group>
      {!s.factsKnown && <Text size="xs" c="dimmed">File facts are unknown; this score may be incomplete.</Text>}
      {s.reason && <Text size="xs" c="orange">{s.reason}</Text>}
    </Stack>
  )
}
