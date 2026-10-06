import { useState } from 'react'
import { Button, CopyButton, Group, Modal, ScrollArea, Stack, Text } from '@mantine/core'

export function ErrorDetails({ error, title = 'Error details' }: { error: string; title?: string }) {
  const [opened, setOpened] = useState(false)
  return (
    <>
      <Stack gap={3} style={{ minWidth: 0 }}>
        <Text size="xs" c="red" lineClamp={2} style={{ overflowWrap: 'anywhere' }}>{error}</Text>
        <Button variant="subtle" color="gray" size="compact-xs" style={{ alignSelf: 'flex-start' }} onClick={() => setOpened(true)}>View error</Button>
      </Stack>
      <Modal opened={opened} onClose={() => setOpened(false)} title={title} size="lg" centered>
        <ScrollArea.Autosize mah="60vh" offsetScrollbars>
          <Text component="pre" size="sm" m={0} style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontFamily: 'var(--mantine-font-family-monospace)' }}>{error}</Text>
        </ScrollArea.Autosize>
        <Group mt="md" justify="flex-end"><CopyButton value={error}>{({ copied, copy }) => <Button size="sm" variant="default" onClick={copy}>{copied ? 'Copied' : 'Copy error'}</Button>}</CopyButton><Button size="sm" onClick={() => setOpened(false)}>Done</Button></Group>
      </Modal>
    </>
  )
}
