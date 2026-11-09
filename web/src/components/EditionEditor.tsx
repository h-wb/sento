import { useState } from 'react'
import {
  Alert,
  Button,
  Divider,
  Group,
  Radio,
  Select,
  Stack,
  Text,
  TextInput,
} from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import type { Edition } from '../types/store'
import { EditionOrigin } from './Origin'
import { EditorModal } from './EditorModal'
import { useEditor } from './useEditor'
import { edited } from './editionText'

// An edition's series name in readers: the library's, its own name after
// the title, or a template of its own.
const editionSeriesName = '{Series Title} {edition} {version}'
type NameChoice = 'library' | 'edition' | 'custom'
const choiceOf = (template = ''): NameChoice =>
  !template.trim() ? 'library' : template === editionSeriesName ? 'edition' : 'custom'

export function EditionEditor({
  seriesID,
  value,
  save,
  saveReaderFields,
  reset,
  close,
}: {
  seriesID: number
  value: Edition
  save: (p: {
    editionName: string
    language: string
    publisher: string
    medium: string
    numberingSource: string
  }) => Promise<void>
  saveReaderFields: (fields: Record<string, string>) => Promise<void>
  reset: () => Promise<void>
  close: () => void
}) {
  const [v, set] = useState(value)
  // Publisher and medium rarely need a hand; they open when one was edited.
  const [more, setMore] = useState(
    value.overrides.includes('publisher') || value.overrides.includes('medium'),
  )
  const edit = useEditor()
  const savedName = value.readerFields.series ?? ''
  const [choice, setChoice] = useState(choiceOf(savedName))
  const [custom, setCustom] = useState(choiceOf(savedName) === 'custom' ? savedName : editionSeriesName)
  const seriesName = { library: '', edition: editionSeriesName, custom }[choice]
  const readerFields = { ...value.readerFields, series: seriesName }
  const nameChanged = seriesName !== savedName
  // What readers will show for each card of the edition, rendered by the
  // server as files are.
  const [previewFields] = useDebouncedValue(readerFields, 300)
  const names = useQuery({
    queryKey: ['series', seriesID, 'reader-names', value.id, previewFields],
    queryFn: () => api.readerNames(seriesID, { editionId: value.id, readerFields: previewFields }),
    placeholderData: (prev) => prev,
    retry: false,
  })
  const nameError = names.error instanceof Error ? names.error.message : undefined
  const shown = [
    ...new Set(
      (names.data ?? []).filter((n) => n.editionId === value.id).map((n) => n.fields.series),
    ),
  ]
  return (
    <EditorModal
      dirty={JSON.stringify(v) !== JSON.stringify(value) || nameChanged}
      busy={edit.busy}
      onClose={close}
      title={'Edit edition'}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void edit.run(async () => {
            if (nameChanged) await saveReaderFields(readerFields)
            await save(v)
          })
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
          <Divider label="In readers" labelPosition="left" mt="xs" />
          <Radio.Group
            label="Series name"
            description="The series Kavita, Komga and other readers file this edition under. Give it its own to show it apart from the series’ other editions."
            value={choice}
            onChange={(c) => setChoice(c as NameChoice)}
          >
            <Stack gap={6} mt={6}>
              <Radio value="library" label="Same as the library (Settings → Media)" disabled={edit.busy} />
              <Radio value="edition" label="The series and edition name" disabled={edit.busy} />
              <Radio value="custom" label="Custom" disabled={edit.busy} />
            </Stack>
          </Radio.Group>
          {choice === 'custom' && (
            <TextInput
              aria-label="Custom series name"
              description="Plain text, or tokens such as {Series Title}, {edition}, {lang} and {version}."
              value={custom}
              error={nameError}
              disabled={edit.busy}
              onChange={(e) => setCustom(e.currentTarget.value)}
            />
          )}
          {!nameError && shown.length > 0 && (
            <Text size="xs" c="dimmed">
              Readers show {shown.map((n) => `“${n}”`).join(' and ')}. Existing files change when
              you organize them.
            </Text>
          )}
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
