import { useState } from 'react'
import { Alert, Button, Group, Modal, Stack, Text } from '@mantine/core'
import { api } from '../api'
import { notify } from '../ui'
import type { File as SeriesFile } from '../types/store'
import { FileFacts } from './FileFacts'

// Correct what files are without opening each: the language, group or
// flags their names got wrong. What is left alone stays as it was read.
export function BulkFactsDialog({
  seriesID,
  files,
  hint,
  done,
  close,
}: {
  seriesID: number
  files: SeriesFile[]
  // Why these files were picked, when a card sent the user here.
  hint?: string
  done: () => Promise<void>
  close: () => void
}) {
  const [language, setLanguage] = useState('')
  const [group, setGroup] = useState('')
  const [flags, setFlags] = useState<Record<string, boolean>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const one = files.length === 1
  const nothing = !language.trim() && !group.trim() && Object.keys(flags).length === 0
  const apply = async () => {
    setBusy(true)
    setError('')
    try {
      await api.correctFilesFacts(seriesID, {
        fileIds: files.map((f) => f.id),
        ...(language.trim() ? { language: language.trim() } : {}),
        ...(group.trim() ? { group: group.trim() } : {}),
        flags,
      })
      await done()
      notify.ok(one ? 'File corrected' : `Corrected ${files.length} files`)
      close()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      opened
      onClose={() => !busy && close()}
      title={one ? 'Correct what this file is' : `Correct what ${files.length} files are`}
    >
      <Stack gap="md">
        <Text size="sm" c="dimmed">
          Release profiles accept or refuse a file on what it is. Change what Sento read wrong
          from {one ? 'its name' : 'their names'}; the rest stays as read.
        </Text>
        {hint && (
          <Alert variant="light" p="xs">
            <Text size="sm">{hint}</Text>
          </Alert>
        )}
        <FileFacts
          files={files}
          language={language}
          group={group}
          flags={flags}
          disabled={busy}
          onLanguage={setLanguage}
          onGroup={setGroup}
          onFlags={setFlags}
        />
        {error && <Alert color="red">{error}</Alert>}
        <Group justify="flex-end" gap="sm">
          <Button variant="default" disabled={busy} onClick={close}>
            Cancel
          </Button>
          <Button loading={busy} disabled={nothing} onClick={() => void apply()}>
            {one ? 'Correct file' : `Correct ${files.length} files`}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
