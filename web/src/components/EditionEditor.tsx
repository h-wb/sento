import { useState } from 'react'
import { Alert, Button, Checkbox, Group, Select, Stack, Text, TextInput } from '@mantine/core'
import type { Edition } from '../types/store'
import { EditionOrigin } from './Origin'
import { EditorModal } from './EditorModal'
import { useEditor } from './useEditor'
import { edited } from './editionText'

export function EditionEditor({
  value,
  save,
  reset,
  close,
}: {
  value: Edition
  save: (p: {
    editionName: string
    language: string
    publisher: string
    medium: string
    numberingSource: string
    discoverChapters?: boolean
  }) => Promise<void>
  reset: () => Promise<void>
  close: () => void
}) {
  const [v, set] = useState(value)
  // Publisher and medium rarely need a hand; they open when one was edited.
  const [more, setMore] = useState(
    value.overrides.includes('publisher') || value.overrides.includes('medium'),
  )
  const edit = useEditor()
  return (
    <EditorModal
      dirty={JSON.stringify(v) !== JSON.stringify(value)}
      busy={edit.busy}
      onClose={close}
      title={'Edit edition'}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void edit.run(() => save(v))
        }}
      >
        <Stack>
          <Group gap="xs">
            <EditionOrigin edition={value} />
          </Group>
          <Text size="sm" c="dimmed">
            {value.custom
              ? 'A local edition: everything here is yours, and MangaBaka does not update it.'
              : `MangaBaka lists this as “${value.providerEditionName || value.editionName}” in ${
                  value.collections
                    .map((c) => [c.medium, c.publisher].filter(Boolean).join(' from '))
                    .join(' and ') || 'no medium'
                }. What you change here is kept when it is refreshed.`}{' '}
            To collect a different language or edition, add an edition.
          </Text>
          {(['editionName', 'language', 'publisher', 'medium'] as const).map((k) =>
            k === 'publisher' && !more ? (
              <Button
                key="more"
                variant="subtle"
                size="compact-sm"
                w="fit-content"
                onClick={() => setMore(true)}
              >
                Publisher and medium
                {v.publisher || v.medium
                  ? `: ${[v.publisher, v.medium].filter(Boolean).join(' · ')}`
                  : ''}
              </Button>
            ) : k === 'medium' && !more ? null : (
              <TextInput
                key={k}
                label={
                  {
                    editionName: 'Edition name',
                    language: 'Language',
                    publisher: 'Publisher',
                    medium: 'Publication medium',
                  }[k]
                }
                description={
                  edited(
                    value,
                    {
                      editionName: 'edition_name',
                      language: 'language',
                      publisher: 'publisher',
                      medium: 'medium',
                    }[k],
                  ) ??
                  (k === 'editionName'
                    ? 'Releases are matched by what this says: add “Deluxe”, “Omnibus” or “Colored” when the books are.'
                    : undefined)
                }
                required={k === 'editionName' || k === 'language'}
                value={v[k]}
                disabled={edit.busy}
                onChange={(e) => set({ ...v, [k]: e.currentTarget.value })}
              />
            ),
          )}
          {v.type === 'chapter' && v.custom && (
            <Select
              label="Chapter numbering"
              description="Use explicit books for gaps or edition-specific numbering"
              data={[
                { value: 'manual', label: 'Explicit books only' },
                { value: 'series_count', label: 'Infer 1 to the series chapter count' },
                { value: 'catalog', label: 'Catalog books only' },
              ]}
              value={v.numberingSource}
              disabled={edit.busy}
              onChange={(source) => set({ ...v, numberingSource: source ?? 'manual' })}
            />
          )}
          {v.type === 'chapter' && (
            <Checkbox
              label="Collect source observations for review"
              description="Only verified source edition mappings can submit chapter observations"
              checked={v.discoverChapters}
              onChange={(e) => set({ ...v, discoverChapters: e.currentTarget.checked })}
            />
          )}{' '}
          {edit.error && <Alert color="red">{edit.error}</Alert>}
          <Group justify="space-between">
            {(v.syncState === 'linked' || v.syncState === 'stale') && (
              <Button variant="subtle" disabled={edit.busy} onClick={() => void edit.run(reset)}>
                Use MangaBaka’s values
              </Button>
            )}
            <Button type="submit" loading={edit.busy}>
              Save edition
            </Button>
          </Group>
        </Stack>
      </form>
    </EditorModal>
  )
}
