import { Alert, Button, Group, Text } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'
import { bookRanges } from './bookRanges'
import type { Refused } from './refusal'

// Says which books are on disk but not used by the card, why, and the
// three ways out: the files were read wrong, they belong on a card with
// another profile, or other releases should replace them.
export function RefusedFilesNotice({
  refused,
  chapters,
  onCorrect,
  onCollectAgain,
  onSearch,
}: {
  refused: Refused
  chapters: boolean
  onCorrect: () => void
  onCollectAgain: () => void
  onSearch: () => void
}) {
  const n = refused.books.length
  if (n === 0) return null
  const which = `${chapters ? 'Ch.' : 'Vol.'} ${bookRanges(refused.books, (b) => b.book.sequenceNum, (b) => b.book.sequenceStr)}`
  return (
    <Alert color="orange" variant="default" p="sm" icon={<IconAlertTriangle size={18} color="var(--mantine-color-orange-filled)" />}>
      <Text size="sm" fw={500}>
        {which} {n === 1 ? 'is' : 'are'} on disk, but this card doesn’t use {n === 1 ? 'its file' : 'their files'}
      </Text>
      {refused.reason && (
        <Text size="xs" c="dimmed" mt={2}>
          {refused.reason}.
        </Text>
      )}
      <Group gap="xs" mt="sm">
        <Button size="xs" variant="default" onClick={onCorrect}>
          Correct what they are…
        </Button>
        <Button size="xs" variant="default" onClick={onCollectAgain}>
          Collect them with another profile…
        </Button>
        <Button size="xs" variant="subtle" color="gray" onClick={onSearch}>
          Find other releases
        </Button>
      </Group>
    </Alert>
  )
}
