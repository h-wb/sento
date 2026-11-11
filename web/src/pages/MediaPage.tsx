import { useState } from 'react'
import {
  Accordion,
  Alert,
  Button,
  Card,
  Code,
  Group,
  Select,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
  Title,
  UnstyledButton,
} from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type MediaSettings } from '../api'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { TemplateInput } from '../components/TemplateInput'
import OrganizeLibrary from '../components/OrganizeLibrary'
import { useUnsavedChanges } from '../useUnsavedChanges'

// Settings that decide where existing files live or what they say: saving
// a change to one of them leaves those files to an organize pass.
const fileSettings: (keyof MediaSettings)[] = [
  'seriesFolder',
  'volumeFile',
  'chapterFile',
  'volumesRoot',
  'chaptersRoot',
  'bookFolder',
  'hashInName',
  'renameImports',
  'writeComicInfo',
  'readerFields',
]
const changesFiles = (a: MediaSettings, b: MediaSettings) =>
  fileSettings.some((k) => JSON.stringify(a[k]) !== JSON.stringify(b[k]))

// Folder layout presets.
const seriesFolderDefault = '{Series Title} [mangabaka-{seriesId}]'
const seriesFolderByEdition = '{Series Title} [mangabaka-{seriesId}] [{edition}][{lang}] ({version})'
const seriesFolderByReader = '{Reader Series} [mangabaka-{seriesId}]'
const layouts: { value: string; label: string; template: string; seriesFolder?: string }[] = [
  { value: 'edition', label: 'By edition — Standard Edition [en]', template: '{edition} [{lang}] ({version})' },
  {
    value: 'editionFolders',
    label: 'One series folder per edition and profile — ONE PIECE [Standard Edition][en] (Colored)',
    template: '',
    seriesFolder: seriesFolderByEdition,
  },
  {
    value: 'readerFolders',
    label: 'One folder per name in readers — ONE PIECE [mangabaka-377] beside ONE PIECE Colored [mangabaka-377]',
    template: '',
    seriesFolder: seriesFolderByReader,
  },
  { value: 'type', label: 'By type, then edition — Volumes/Standard Edition [en]', template: '{type}/{edition} [{lang}] ({version})' },
  { value: 'flat', label: 'Flat — everything in the series folder', template: '' },
  { value: 'ids', label: 'By internal id — editions/42/versions/81', template: 'editions/{editionId}/versions/{versionId}' },
]

export default function MediaPage() {
  usePageTitle('Media')
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['media-settings'], queryFn: api.mediaSettings })

  if (settings.isPending || settings.error) return <><PageHeader title="Media" description="File naming and import settings." /><QueryFeedback pending={settings.isPending} error={settings.error} onRetry={() => void settings.refetch()} /></>
  return <MediaForm initial={settings.data} onSaved={() => queryClient.invalidateQueries({ queryKey: ['media-settings'] })} />
}

function MediaForm({ initial, onSaved }: { initial: MediaSettings; onSaved: () => void }) {
  const [form, setForm] = useState<MediaSettings>(initial)
  const [saved, setSaved] = useState(false)
  const [organizing, setOrganizing] = useState(false)
  // Set by a save that changed how existing files are named or tagged.
  const [offerOrganize, setOfferOrganize] = useState(false)
  const [moreReaderFields, setMoreReaderFields] = useState(false)
  const set = (patch: Partial<MediaSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setSaved(false)
  }
  const setReaderField = (key: string, template: string) => {
    setForm((f) => ({ ...f, readerFields: { ...f.readerFields, [key]: template } }))
    setSaved(false)
  }

  // The tokens and reader fields come from the server's own lists.
  const schema = useQuery({
    queryKey: ['media-schema'],
    queryFn: api.mediaSchema,
    staleTime: Infinity,
  })
  const readerFields = schema.data?.readerFields ?? []

  // Preview renders server-side (the Go renderer is the only truth);
  // debounced so typing doesn't spam the API.
  const [debounced] = useDebouncedValue(form, 300)
  const preview = useQuery({
    queryKey: ['media-preview', debounced],
    queryFn: () => api.previewNaming(debounced),
    placeholderData: (prev) => prev,
  })
  const errors = preview.data?.errors ?? {}

  // The template last focused: a token picked from the table goes into it,
  // at the caret. Reader fields are keyed "reader:<field>".
  const [active, setActive] = useState<{ field: string; input: HTMLInputElement } | null>(null)
  const focused = (field: string) => (input: HTMLInputElement) => setActive({ field, input })
  const insertToken = (syntax: string) => {
    if (!active?.input.isConnected) return
    const { field, input } = active
    const start = input.selectionStart ?? input.value.length
    const end = input.selectionEnd ?? start
    const value = input.value.slice(0, start) + syntax + input.value.slice(end)
    if (field.startsWith('reader:')) setReaderField(field.slice('reader:'.length), value)
    else set({ [field]: value })
    requestAnimationFrame(() => {
      input.focus()
      input.setSelectionRange(start + syntax.length, start + syntax.length)
    })
  }

  // The sample edition, and a second card of it collected as "Colored".
  const readerPreview = (key: string) => {
    const sample = preview.data?.reader[key]
    if (!sample) return undefined
    return sample.colored !== sample.first
      ? `${sample.first} · collected again as “Colored”: ${sample.colored}`
      : sample.first
  }
  const readerInput = (f: (typeof readerFields)[number]) => (
    <TemplateInput
      key={f.key}
      label={f.title}
      description={f.help}
      value={form.readerFields[f.key] ?? ''}
      preview={readerPreview(f.key)}
      error={preview.data?.reader[f.key]?.error}
      onChange={(v) => setReaderField(f.key, v)}
      onFocus={focused(`reader:${f.key}`)}
    />
  )

  const save = useMutation({
    mutationFn: async () => {
      const organize = changesFiles(initial, form)
      await api.saveMediaSettings(form)
      return organize
    },
    onSuccess: (organize) => {
      setSaved(true)
      setOfferOrganize(organize)
      notify.ok('Media settings saved')
      onSaved()
    },
    onError: notify.err,
  })

  const dirty = JSON.stringify(form) !== JSON.stringify(initial)
  useUnsavedChanges(dirty)

  return (
    <>
      <PageHeader
        title="Media"
        description="File naming and import settings. New imports follow them at once; existing files change when you organize them."
      />

      <div className="settingsLayout">
        <fieldset className="formFields" disabled={save.isPending}><Stack>
          <Title order={4}>Naming</Title>
          <QueryFeedback
            error={schema.error ?? preview.error}
            onRetry={() => void (schema.error ? schema.refetch() : preview.refetch())}
          />
          {readerFields.slice(0, 1).map(readerInput)}
          {readerFields.length < 2 ? null : moreReaderFields ? (
            readerFields.slice(1).map(readerInput)
          ) : (
            <Button
              variant="subtle"
              size="compact-sm"
              w="fit-content"
              onClick={() => setMoreReaderFields(true)}
            >
              Other reader fields: {readerFields.slice(1).map((f) => f.title).join(', ')}
            </Button>
          )}
          <TemplateInput
            label="Series folder"
            value={form.seriesFolder}
            preview={preview.data?.seriesFolder}
            error={errors.seriesFolder}
            onChange={(v) => set({ seriesFolder: v })}
            onFocus={focused('seriesFolder')}
          />
          <TemplateInput
            label="Volume file"
            value={form.volumeFile}
            preview={preview.data?.volumeFile}
            error={errors.volumeFile}
            onChange={(v) => set({ volumeFile: v })}
            onFocus={focused('volumeFile')}
          />
          <TemplateInput
            label="Chapter file"
            value={form.chapterFile}
            preview={preview.data?.chapterFile}
            error={errors.chapterFile}
            onChange={(v) => set({ chapterFile: v })}
            onFocus={focused('chapterFile')}
          />

          <Title order={5} mt="sm">
            Folder layout
          </Title>
          <Group grow align="flex-start">
            <TextInput
              label="Volumes top folder"
              placeholder="None"
              description="Optional folder above the series, e.g. Volumes."
              value={form.volumesRoot}
              error={errors.volumesRoot}
              onChange={(e) => set({ volumesRoot: e.currentTarget.value })}
            />
            <TextInput
              label="Chapters top folder"
              placeholder="None"
              description="Give it a different name, e.g. Chapters, to keep chapters in their own tree."
              value={form.chaptersRoot}
              error={errors.chaptersRoot}
              onChange={(e) => set({ chaptersRoot: e.currentTarget.value })}
            />
          </Group>
          <Select
            label="Folders inside a series"
            description="How editions, and an edition collected again with another profile, are separated. A series folder template naming the edition gives each edition its own top folder, which folder-based readers show as its own series."
            allowDeselect={false}
            value={
              layouts.find(
                (l) => l.template === form.bookFolder && (!l.seriesFolder || l.seriesFolder === form.seriesFolder),
              )?.value ?? 'custom'
            }
            onChange={(v) => {
              const layout = layouts.find((l) => l.value === v)
              if (!layout) return
              const seriesFolder =
                layout.seriesFolder ?? (form.seriesFolder === seriesFolderByEdition ? seriesFolderDefault : form.seriesFolder)
              set({ bookFolder: layout.template, seriesFolder, hashInName: layout.value === 'ids' })
            }}
            data={[...layouts.map(({ value, label }) => ({ value, label })), { value: 'custom', label: 'Custom' }]}
          />
          <TemplateInput
            label="Book folder"
            value={form.bookFolder}
            preview={
              preview.data ? preview.data.bookFolder || '(directly in the series folder)' : undefined
            }
            error={errors.bookFolder}
            onChange={(v) => set({ bookFolder: v })}
            onFocus={focused('bookFolder')}
          />
          <div>
            <Text size="xs" c="dimmed">
              A volume lands at
            </Text>
            <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
              {preview.data?.volumePath ?? 'Loading…'}
            </Text>
            <Text size="xs" c="dimmed" mt={4}>
              A chapter lands at
            </Text>
            <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
              {preview.data?.chapterPath ?? 'Loading…'}
            </Text>
            <Text size="xs" c="dimmed" mt={6}>
              Existing files move only when you organize them: on a series, on several from the
              Library table, or with “Organize existing files” below.
            </Text>
          </div>
          <Switch
            label="Add the content fingerprint to file names"
            description="Off, it is only added when two different books would get the same path."
            checked={form.hashInName}
            onChange={(e) => set({ hashInName: e.currentTarget.checked })}
          />

          <Title order={5} mt="sm">
            Import options
          </Title>
          <Switch
            label="Rename imported files"
            description="When off, keep the original filename"
            checked={form.renameImports}
            onChange={(e) => set({ renameImports: e.currentTarget.checked })}
          />
          <Switch
            label="Embed metadata in books"
            description="Every import becomes a clean CBZ with a full ComicInfo.xml (series, number, credits, summary, dates, language). Torrent downloads are copied so the original keeps seeding, which uses extra disk space while it seeds."
            checked={form.writeComicInfo}
            onChange={(e) => set({ writeComicInfo: e.currentTarget.checked })}
          />
          <Switch
            label="Copy instead of hardlink"
            description="Only when metadata is not embedded: use a full copy if your filesystem does not support hardlinks."
            checked={form.alwaysCopy}
            disabled={form.writeComicInfo}
            onChange={(e) => set({ alwaysCopy: e.currentTarget.checked })}
          />

          <Group mt="sm">
            <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!dirty}>
              Save changes
            </Button>
            {dirty && <Text size="xs" c="dimmed">Unsaved changes</Text>}
            {saved && !dirty && (
              <Text size="sm" c="green">
                Saved.
              </Text>
            )}
            {dirty && (
              <Button variant="subtle" onClick={() => setForm(initial)}>
                Reset
              </Button>
            )}
            <Button variant="default" ml="auto" disabled={dirty} onClick={() => setOrganizing(true)}>
              Organize existing files
            </Button>
          </Group>
          {offerOrganize && !dirty && (
            <Alert
              color="blue"
              title="Existing files are unchanged"
              withCloseButton
              closeButtonLabel="Dismiss"
              onClose={() => setOfferOrganize(false)}
            >
              <Text size="sm">
                New imports follow these settings. Files already in the library keep their names
                and metadata until you organize them.
              </Text>
              <Button size="xs" mt="xs" onClick={() => setOrganizing(true)}>
                Preview changes
              </Button>
            </Alert>
          )}
          {organizing && (
            <OrganizeLibrary
              onClose={() => {
                setOrganizing(false)
                setOfferOrganize(false)
              }}
            />
          )}
        </Stack></fieldset>

        <Card padding="md">
          <Accordion variant="default" defaultValue="tokens">
            <Accordion.Item value="tokens">
              <Accordion.Control>Naming tokens</Accordion.Control>
              <Accordion.Panel>
                <Text size="xs" c="dimmed" mb="xs">
                  Click a token to put it into the template you are editing. <Code>:00</Code> pads
                  a number.
                </Text>
                <Table withRowBorders={false} verticalSpacing={2}>
                  <Table.Tbody>
                    {(schema.data?.tokens ?? []).map((t) => (
                      <Table.Tr key={t.syntax}>
                        <Table.Td style={{ whiteSpace: 'nowrap' }}>
                          <UnstyledButton
                            aria-label={`Insert ${t.syntax}`}
                            // Keep the caret in the template being edited.
                            onMouseDown={(e) => e.preventDefault()}
                            onClick={() => insertToken(t.syntax)}
                          >
                            <Code style={{ whiteSpace: 'nowrap' }}>{t.syntax}</Code>
                          </UnstyledButton>
                          {t.perBook && (
                            <Text span size="xs" c="dimmed" aria-hidden>
                              {' *'}
                            </Text>
                          )}
                        </Table.Td>
                        <Table.Td>
                          <Text size="xs" c="dimmed">
                            {t.help}
                          </Text>
                        </Table.Td>
                      </Table.Tr>
                    ))}
                  </Table.Tbody>
                </Table>
                <Text size="xs" c="dimmed" mt="xs">
                  Brackets around an empty token disappear: <Code>[{'{group}'}]</Code> vanishes
                  when there is no group.
                </Text>
                <Text size="xs" c="dimmed" mt={4}>
                  * names one book, so it cannot be part of a name in readers.
                </Text>
              </Accordion.Panel>
            </Accordion.Item>
          </Accordion>
        </Card>
      </div>
    </>
  )
}


