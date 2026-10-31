import { useState } from 'react'
import { Alert, Button, Checkbox, Group, List, Modal, Stack, Text } from '@mantine/core'
import { api } from '../api'
import { notify } from '../ui'

// RemoveSeries asks before removing one or several series, and what goes
// with them: their files (on by default), and their torrents (opt in:
// imports are hardlinks, so the torrent keeps its own copy of the data and
// the space only comes back when it goes too).
export function RemoveSeries({
  ids,
  name,
  close,
  removed,
}: {
  ids: number[]
  // The series' title when there is one.
  name?: string
  close: () => void
  removed: () => void
}) {
  const [deleteFiles, setDeleteFiles] = useState(true)
  const [removeDownloads, setRemoveDownloads] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  // What was asked for but stayed; shown before the dialog closes.
  const [kept, setKept] = useState<string[] | null>(null)
  const them = ids.length === 1 ? 'its' : 'their'
  const submit = async () => {
    setPending(true)
    setError('')
    try {
      const results = await Promise.all(ids.map((id) => api.deleteSeries(id, { deleteFiles, removeDownloads })))
      const files = results.reduce((n, r) => n + r.filesDeleted, 0)
      const torrents = results.reduce((n, r) => n + r.downloadsRemoved, 0)
      notify.ok(
        [
          ids.length === 1 ? `Removed ${name ?? 'the series'}` : `Removed ${ids.length} series`,
          deleteFiles && `${files} file${files === 1 ? '' : 's'} deleted`,
          removeDownloads && `${torrents} torrent${torrents === 1 ? '' : 's'} removed`,
        ]
          .filter(Boolean)
          .join(' · '),
      )
      const left = results.flatMap((r) => r.kept ?? [])
      if (left.length) setKept(left)
      else removed()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setPending(false)
    }
  }
  if (kept)
    return (
      <Modal opened onClose={removed} title="Some things stayed" centered>
        <Stack gap="md">
          <Text size="sm" c="dimmed">
            The removal is done; these could not be deleted:
          </Text>
          <List size="sm" spacing={4}>
            {kept.map((k) => (
              <List.Item key={k} style={{ overflowWrap: 'anywhere' }}>
                {k}
              </List.Item>
            ))}
          </List>
          <Group justify="flex-end">
            <Button onClick={removed}>Done</Button>
          </Group>
        </Stack>
      </Modal>
    )
  return (
    <Modal
      opened
      onClose={() => !pending && close()}
      title={ids.length === 1 ? `Remove ${name ?? 'this series'}?` : `Remove ${ids.length} series?`}
      centered
      closeOnClickOutside={!pending}
      closeOnEscape={!pending}
      withCloseButton={!pending}
    >
      <Stack gap="md">
        <Text size="sm" c="dimmed">
          Sento forgets {ids.length === 1 ? 'it' : 'them'}: editions, monitoring and history.
        </Text>
        <Checkbox
          label={`Delete ${them} files from disk`}
          description="Every file Sento has on record, and the folders that leaves empty."
          checked={deleteFiles}
          disabled={pending}
          onChange={(e) => setDeleteFiles(e.currentTarget.checked)}
        />
        <Checkbox
          label={`Remove ${them} torrents from qBittorrent`}
          description="With their downloaded data. Left unticked they keep seeding, and keep using that space."
          checked={removeDownloads}
          disabled={pending}
          onChange={(e) => setRemoveDownloads(e.currentTarget.checked)}
        />
        {error && <Alert color="red">{error}</Alert>}
        <Group justify="flex-end">
          <Button variant="default" onClick={close} disabled={pending} data-autofocus>
            Cancel
          </Button>
          <Button color="red" loading={pending} onClick={() => void submit()}>
            Remove
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
