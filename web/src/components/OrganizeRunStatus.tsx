import { Alert, Progress, Stack, Text } from '@mantine/core'
import type { LibraryOrganizeStatus } from '../types/service'

// Progress of the background organize run while it goes, then what it
// did and which series failed.
export default function OrganizeRunStatus({
  status: s,
  busy,
  finished,
}: {
  status?: LibraryOrganizeStatus
  busy: boolean
  finished: boolean
}) {
  if (!s) return null
  if (busy) {
    return (
      <Stack gap={6}>
        <Text size="sm">
          {s.queued ? 'Starting…' : `Organizing ${s.current || '…'} (${s.done} of ${s.total})`}
        </Text>
        <Progress value={s.total ? (100 * s.done) / s.total : 0} animated />
        <Text size="xs" c="dimmed">
          {s.renamed} moved · {s.retagged} updated so far. This keeps running if you close the
          dialog.
        </Text>
      </Stack>
    )
  }
  if (!finished) return null
  const failed = s.series.filter((r) => r.errors.length > 0)
  return (
    <>
      <Alert color={failed.length > 0 || s.error ? 'orange' : 'green'}>
        {s.renamed} moved · {s.retagged} updated across {s.done} series
        {s.done < s.total && ` (stopped after ${s.done} of ${s.total})`}.
        {s.error && failed.length === 0 && <Text size="xs">{s.error}</Text>}
      </Alert>
      {failed.map((r) => (
        <Alert key={r.seriesId} color="orange" title={r.title}>
          {r.errors.map((e) => (
            <Text key={e} size="xs" style={{ overflowWrap: 'anywhere' }}>
              {e}
            </Text>
          ))}
        </Alert>
      ))}
    </>
  )
}
