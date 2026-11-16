import { useMemo, useState } from 'react'
import { Button, FileButton, Group, Modal, Stack, Text, Textarea } from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { useMutation } from '@tanstack/react-query'
import { api, type ProfileDoc } from '../../api'
import { notify } from '../../ui'

// --- import ---

export function ImportProfileModal({
  onClose,
  onSaved,
  fail,
}: {
  onClose: () => void
  onSaved: () => void
  fail: (e: Error) => void
}) {
  const [text, setText] = useState('')
  // Match the theme's mobile input rule: under 16px iOS zooms on focus.
  const coarseInput = useMediaQuery('(pointer: coarse), (max-width: 48em)')

  const doc = useMemo(() => {
    try {
      return JSON.parse(text) as ProfileDoc
    } catch {
      return null
    }
  }, [text])

  const importMut = useMutation({
    mutationFn: (d: ProfileDoc) => api.importProfile(d),
    onSuccess: (p) => {
      notify.ok(`Imported profile “${p.name}”`)
      onSaved()
      onClose()
    },
    onError: fail,
  })

  return (
    <Modal opened onClose={onClose} title="Import profile" size="lg">
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          Paste an exported profile document, or pick a file. Formats are matched by name and
          updated; the profile is created or replaced by name.
        </Text>
        <FileButton
          accept="application/json"
          onChange={(f) => {
            if (f) void f.text().then(setText)
          }}
        >
          {(props) => (
            <Button {...props} variant="default" size="xs" style={{ alignSelf: 'flex-start' }}>
              Choose file…
            </Button>
          )}
        </FileButton>
        <Textarea
          autosize
          minRows={8}
          maxRows={20}
          placeholder='{"name": "…", "minFormatScore": -10000, "formats": [...]}'
          value={text}
          onChange={(e) => setText(e.currentTarget.value)}
          styles={{ input: { fontFamily: 'monospace', fontSize: coarseInput ? 16 : 12 } }}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button
            disabled={!doc}
            loading={importMut.isPending}
            onClick={() => doc && importMut.mutate(doc)}
          >
            Import
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
