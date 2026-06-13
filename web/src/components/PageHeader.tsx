import { Group, Stack, Text, Title } from '@mantine/core'
import type { ReactNode } from 'react'

// Consistent page heading: bold title (with optional count) + one muted line of
// context, and an optional actions slot on the right.
export function PageHeader({
  title,
  description,
  count,
  action,
}: {
  title: string
  description?: string
  count?: number
  action?: ReactNode
}) {
  return (
    <Group justify="space-between" align="flex-start" mb="lg" wrap="wrap" gap="sm">
      <Stack gap={2}>
        <Title order={2}>
          {title}
          {count != null && count > 0 && (
            <Text span c="dimmed" size="lg" fw={400} ml={8}>
              {count}
            </Text>
          )}
        </Title>
        {description && (
          <Text c="dimmed" size="sm">
            {description}
          </Text>
        )}
      </Stack>
      {action}
    </Group>
  )
}
