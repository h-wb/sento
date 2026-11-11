import { useState } from 'react'
import { Alert, Button, Group, Modal, MultiSelect, Stack, Text, TextInput } from '@mantine/core'
import { api } from '../api'
import { notify } from '../ui'
import type { File as SeriesFile } from '../types/store'
import { factFlags } from './factFlags'

// Correct what several files are in one go: the language, group or flags
// their names got wrong. Fields left empty stay as they were read.
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
  const [are, setAre] = useState<string[]>([])
  const [areNot, setAreNot] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const options = (taken: string[]) =>
    factFlags.filter((f) => !taken.includes(f.key)).map((f) => ({ value: f.key, label: f.label }))
  const nothing = !language.trim() && !group.trim() && are.length + areNot.length === 0
  const apply = async () => {
    setBusy(true)
    setError('')
    try {
      await api.correctFilesFacts(seriesID, {
        fileIds: files.map((f) => f.id),
        ...(language.trim() ? { language: language.trim() } : {}),
        ...(group.trim() ? { group: group.trim() } : {}),
        flags: Object.fromEntries([
          ...are.map((k) => [k, true] as const),
          ...areNot.map((k) => [k, false] as const),
        ]),
      })
      await done()
      notify.ok(`Corrected ${files.length} ${files.length === 1 ? 'file' : 'files'}`)
      close()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal opened onClose={() => !busy && close()} title="Correct what these files are">
      <Stack gap="sm">
        <Text size="sm">
          {files.length} {files.length === 1 ? 'file' : 'files'}. Release profiles accept or
          refuse a file on its language, group and flags, which Sento reads from its name and
          release. What you set here is kept; anything left empty stays as it was read.
        </Text>
        {hint && (
          <Alert variant="light" p="xs">
            <Text size="sm">{hint}</Text>
          </Alert>
        )}
        <MultiSelect
          label="These files are not"
          placeholder={areNot.length ? undefined : 'e.g. Colored'}
          data={options(are)}
          value={areNot}
          disabled={busy}
          onChange={setAreNot}
        />
        <MultiSelect
          label="These files are"
          placeholder={are.length ? undefined : 'e.g. Digital'}
          data={options(areNot)}
          value={are}
          disabled={busy}
          onChange={setAre}
        />
        <Group grow>
          <TextInput
            label="Language"
            placeholder="Leave as read"
            value={language}
            disabled={busy}
            onChange={(e) => setLanguage(e.currentTarget.value)}
          />
          <TextInput
            label="Release group"
            placeholder="Leave as read"
            value={group}
            disabled={busy}
            onChange={(e) => setGroup(e.currentTarget.value)}
          />
        </Group>
        {error && <Alert color="red">{error}</Alert>}
        <Group justify="flex-end">
          <Button variant="default" disabled={busy} onClick={close}>
            Cancel
          </Button>
          <Button loading={busy} disabled={nothing} onClick={() => void apply()}>
            Correct {files.length} {files.length === 1 ? 'file' : 'files'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
