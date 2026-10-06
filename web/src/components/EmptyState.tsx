import { Center, Stack, Text } from '@mantine/core'
import type { ReactNode } from 'react'

export function EmptyState({ icon, title, description, action }: {
  icon?: ReactNode
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <Center py={48}>
      <Stack align="center" gap="sm" maw={420} ta="center">
        {icon && <div className="emptyStateIcon">{icon}</div>}
        <Text fw={600} size="md">{title}</Text>
        <Text c="dimmed" size="sm" lh={1.6}>{description}</Text>
        {action}
      </Stack>
    </Center>
  )
}
