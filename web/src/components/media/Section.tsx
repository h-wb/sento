import { Stack, Text, Title } from '@mantine/core'
import type { ReactNode } from 'react'
import classes from '../../pages/MediaPage.module.css'

export function Section({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <section className={classes.section}>
      <Title order={3} className={classes.sectionTitle}>
        {title}
      </Title>
      <Text size="sm" c="dimmed" mb="md" maw={620}>
        {description}
      </Text>
      <Stack gap="md">{children}</Stack>
    </section>
  )
}
