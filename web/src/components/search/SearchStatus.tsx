import { Badge, Button, Group, Loader, Text } from '@mantine/core'
import { IconRefresh } from '@tabler/icons-react'
import type { Dispatch, SetStateAction } from 'react'
import { plural } from '../../format'
import type { StreamResult } from './useReleaseSearch'

export function SearchStatus({
  result,
  searching,
  problems,
  showProblems,
  setProblemsOpen,
  stop,
  interactiveSearch,
  busy,
}: {
  result: StreamResult
  searching: boolean
  problems: string[]
  showProblems: boolean
  setProblemsOpen: Dispatch<SetStateAction<boolean | null>>
  stop: () => void
  interactiveSearch: () => Promise<void>
  busy: boolean
}) {
  return (
    <Group justify="space-between" gap="xs">
      <Group gap={6}>
        <Text size="sm" role="status">
          {searching
            ? result.pending.length > 0
              ? `Searching ${result.pending.length} of ${result.searched.length} ${plural(result.searched.length, 'source')}…`
              : 'Searching…'
            : result.stopped
              ? 'Search stopped. Results already received are still available.'
              : `Searched ${result.searched.length} ${plural(result.searched.length, 'source')}`}
        </Text>
        {result.queries.length > 0 && (
          <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
            for {result.queries.join(' · ')}
          </Text>
        )}
        {result.pending.map((p) => (
          <Badge key={p} size="sm" variant="light" color="gray" leftSection={<Loader size={10} />}>
            {p.split('/')[1] ?? p}
          </Badge>
        ))}
        {problems.length > 0 && (
          <Button
            size="compact-xs"
            variant="subtle"
            color="orange"
            aria-expanded={showProblems}
            onClick={() => setProblemsOpen(!showProblems)}
          >
            {problems.length} {plural(problems.length, 'source')} had problems
          </Button>
        )}
      </Group>
      {searching ? (
        <Button size="compact-xs" variant="default" onClick={stop}>
          Stop search
        </Button>
      ) : (
        <Button
          size="compact-xs"
          variant="default"
          leftSection={<IconRefresh size={12} />}
          onClick={() => void interactiveSearch()}
          disabled={busy}
        >
          Search again
        </Button>
      )}
    </Group>
  )
}
