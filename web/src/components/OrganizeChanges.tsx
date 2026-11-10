import { Stack, Text } from '@mantine/core'
import type { FileChange } from '../types/service'

// What an organize run would do to each file, for the changes the user
// kept ticked: where it moves and whether its metadata is rewritten.
export function OrganizeChanges({
  changes,
  rename,
  retag,
  limit = 200,
}: {
  changes: FileChange[]
  rename: boolean
  retag: boolean
  limit?: number
}) {
  const selected = changes.filter((c) => (rename && c.rename) || (retag && c.retag))
  return (
    <Stack gap={6}>
      {selected.slice(0, limit).map((c) => (
        <div key={c.fileId}>
          <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
            {c.from}
          </Text>
          <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
            {rename && c.rename ? `→ ${c.to}` : '→ same place'}
            {retag && c.retag && (
              <Text span size="xs" c="dimmed">
                {' '}
                · metadata updated
              </Text>
            )}
          </Text>
        </div>
      ))}
      {selected.length > limit && (
        <Text size="xs" c="dimmed">
          and {selected.length - limit} more
        </Text>
      )}
    </Stack>
  )
}

// The files a run leaves alone, with the server's reason for each kind.
export function SkippedFiles({ changes }: { changes: FileChange[] }) {
  const reasons = new Map<string, number>()
  for (const c of changes) {
    if (c.skip) reasons.set(c.skip, (reasons.get(c.skip) ?? 0) + 1)
  }
  const skipped = [...reasons.values()].reduce((a, b) => a + b, 0)
  if (skipped === 0) return null
  return (
    <Text size="xs" c="dimmed">
      {skipped} {skipped === 1 ? 'file is' : 'files are'} left alone:{' '}
      {[...reasons]
        .map(([reason, n]) => `${reason.replace(/\.$/, '')}${reasons.size > 1 ? ` (${n})` : ''}`)
        .join(' · ')}
      .
    </Text>
  )
}
