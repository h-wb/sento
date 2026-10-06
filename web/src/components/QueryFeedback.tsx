import { Alert, Button, Skeleton, Stack, Text } from '@mantine/core'

// Loading and failure are distinct from a successfully loaded empty list.
export function QueryFeedback({ pending, error, onRetry }: {
  pending?: boolean
  error?: unknown
  onRetry?: () => void
}) {
  if (error) return (
    <Alert color="red" title="Couldn't load this data" mb="md">
      <Text size="sm">{error instanceof Error ? error.message : String(error)}</Text>
      {onRetry && <Button variant="white" color="red" size="xs" mt="sm" onClick={onRetry}>Try again</Button>}
    </Alert>
  )
  if (pending) return (
    <Stack gap="sm" py="md" role="status" aria-label="Loading">
      <Skeleton h={18} w="35%" />
      <Skeleton h={42} />
      <Skeleton h={42} />
    </Stack>
  )
  return null
}
