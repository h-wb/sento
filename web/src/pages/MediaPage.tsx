import { useState, type ReactNode } from 'react'
import {
  Accordion,
  Alert,
  Badge,
  Button,
  Card,
  Code,
  Collapse,
  Group,
  Select,
  SimpleGrid,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
  Title,
  UnstyledButton,
} from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { IconAlertTriangle, IconCheck, IconCircleCheck } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type MediaSettings } from '../api'
import type { NamingSetup } from '../types/service'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { TemplateInput } from '../components/TemplateInput'
import { PathTree } from '../components/PathTree'
import OrganizeLibrary from '../components/OrganizeLibrary'
import { useUnsavedChanges } from '../useUnsavedChanges'
import classes from './MediaPage.module.css'

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

// What sits inside a series folder. A layout naming a series folder sets
// that too; the others keep it, unless it was the per-edition one.
const seriesFolderDefault = '{Series Title} [mangabaka-{seriesId}]'
const seriesFolderByEdition = '{Series Title} [mangabaka-{seriesId}] [{edition}][{lang}] ({version})'
const layouts: { value: string; label: string; template: string; seriesFolder?: string }[] = [
  { value: 'edition', label: 'A folder per edition — Standard Edition [en]', template: '{edition} [{lang}] ({version})' },
  { value: 'type', label: 'By type, then edition — Volumes/Standard Edition [en]', template: '{type}/{edition} [{lang}] ({version})' },
  { value: 'flat', label: 'None — books directly in the series folder', template: '' },
  {
    value: 'editionFolders',
    label: 'A series folder per edition instead — ONE PIECE [Standard Edition][en] (Colored)',
    template: '',
    seriesFolder: seriesFolderByEdition,
  },
  { value: 'ids', label: 'By internal id — editions/42/versions/81', template: 'editions/{editionId}/versions/{versionId}' },
]

// A setup is the settings it fills; it is recognised by them, not stored.
const matches = (s: NamingSetup, m: MediaSettings) =>
  s.seriesFolder === m.seriesFolder &&
  s.bookFolder === m.bookFolder &&
  Object.entries(s.readerFields).every(([k, v]) => m.readerFields[k] === v)

function Section({ title, description, children }: { title: string; description: string; children: ReactNode }) {
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

export default function MediaPage() {
  usePageTitle('Media')
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['media-settings'], queryFn: api.mediaSettings })

  if (settings.isPending || settings.error) return <><PageHeader title="Media" description="Where Sento puts your books, and what it writes inside them." /><QueryFeedback pending={settings.isPending} error={settings.error} onRetry={() => void settings.refetch()} /></>
  return <MediaForm initial={settings.data} onSaved={() => queryClient.invalidateQueries({ queryKey: ['media-settings'] })} />
}

function MediaForm({ initial, onSaved }: { initial: MediaSettings; onSaved: () => void }) {
  const [form, setForm] = useState<MediaSettings>(initial)
  const [saved, setSaved] = useState(false)
  const [organizing, setOrganizing] = useState(false)
  // Set by a save that changed how existing files are named or tagged.
  const [offerOrganize, setOfferOrganize] = useState(false)
  const [allFields, setAllFields] = useState(false)
  const set = (patch: Partial<MediaSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setSaved(false)
  }
  const setReaderField = (key: string, template: string) => {
    setForm((f) => ({ ...f, readerFields: { ...f.readerFields, [key]: template } }))
    setSaved(false)
  }

  // The tokens, reader fields, ComicInfo account and setups come from the
  // server's own lists.
  const schema = useQuery({
    queryKey: ['media-schema'],
    queryFn: api.mediaSchema,
    staleTime: Infinity,
  })
  const readerFields = schema.data?.readerFields ?? []
  const setups = schema.data?.setups ?? []
  const setup = setups.find((s) => matches(s, form))

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
  // A level of the path tree leads to the setting that makes it.
  const pick = (setting: string) => {
    const input = document.getElementById(`media-${setting}`)
    input?.scrollIntoView({ block: 'center', behavior: 'smooth' })
    input?.focus({ preventScroll: true })
  }

  // The sample edition, and the same edition collected again as "Colored".
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
      id={`media-reader-${f.key}`}
      label={f.title}
      description={f.help}
      value={form.readerFields[f.key] ?? ''}
      preview={readerPreview(f.key)}
      error={preview.data?.reader[f.key]?.error}
      disabled={!form.writeComicInfo}
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
        description="Where Sento puts your books, and what it writes inside them. New imports follow these settings at once; files already in the library change when you organize them."
        action={
          <Button variant="default" disabled={dirty} onClick={() => setOrganizing(true)}>
            Organize existing files
          </Button>
        }
      />

      <div className="settingsLayout">
        <fieldset className="formFields" disabled={save.isPending}>
          <QueryFeedback
            error={schema.error ?? preview.error}
            onRetry={() => void (schema.error ? schema.refetch() : preview.refetch())}
          />

          <Section
            title="Your reader"
            description="Readers decide what a series is in their own way. A setup fills the series folder, the book folder and the names in readers below so yours shows each edition as you mean; nothing changes until you save."
          >
            <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
              {setups.map((s, i) => {
                const on = setup?.key === s.key
                return (
                  <UnstyledButton
                    key={s.key}
                    className={classes.setup}
                    data-active={on}
                    aria-pressed={on}
                    onClick={() =>
                      set({ seriesFolder: s.seriesFolder, bookFolder: s.bookFolder, readerFields: { ...form.readerFields, ...s.readerFields } })
                    }
                  >
                    <Group justify="space-between" wrap="nowrap" gap="xs">
                      <Text fw={600} size="sm">
                        {s.title}
                      </Text>
                      {on ? (
                        <IconCheck size={16} aria-hidden />
                      ) : (
                        i === 0 && <Badge size="xs" variant="light" color="gray">Recommended</Badge>
                      )}
                    </Group>
                    <Text size="xs" c="dimmed" mt={4} lh={1.5}>
                      {s.summary}
                    </Text>
                  </UnstyledButton>
                )
              })}
            </SimpleGrid>
            {setups.length > 0 && !setup && (
              <Text size="xs" c="dimmed">
                Your own arrangement: it matches neither setup.
              </Text>
            )}
            <Stack gap={8} aria-label="How readers will show it" role="group">
              {(preview.data?.readers ?? []).map((r) => (
                <Group key={r.reader} gap="xs" wrap="nowrap" align="flex-start">
                  {r.fine ? (
                    <IconCircleCheck size={18} color="var(--mantine-color-green-filled)" style={{ flexShrink: 0 }} />
                  ) : (
                    <IconAlertTriangle size={18} color="var(--mantine-color-yellow-filled)" style={{ flexShrink: 0 }} />
                  )}
                  <Text size="sm">
                    <Text span fw={600}>
                      {r.reader}
                    </Text>{' '}
                    {r.says.charAt(0).toLowerCase() + r.says.slice(1)}
                  </Text>
                </Group>
              ))}
            </Stack>
            {(setup?.tips?.length ?? 0) > 0 && (
              <div className={classes.tips}>
                <Text size="xs" fw={600} mb={4}>
                  In your reader
                </Text>
                {[...new Set(setup!.tips.map((t) => t.reader))].map((reader) => (
                  <div key={reader}>
                    <Text size="xs" fw={500} mt={6}>
                      {reader}
                    </Text>
                    <ul className={classes.tipList}>
                      {setup!.tips
                        .filter((t) => t.reader === reader)
                        .map((t) => (
                          <li key={t.tip}>
                            <Text size="xs" c="dimmed">
                              {t.tip}
                            </Text>
                          </li>
                        ))}
                    </ul>
                  </div>
                ))}
              </div>
            )}
          </Section>

          <Section
            title="On disk"
            description="Where each book's file goes, one level at a time. The tree shows three sample books; pick a level to change the setting that makes it."
          >
            {preview.data?.paths ? <PathTree examples={preview.data.paths} onPick={pick} /> : <Text size="sm" c="dimmed">Loading…</Text>}
            <Group grow align="flex-start">
              <TextInput
                id="media-volumesRoot"
                label="Volumes top folder"
                placeholder="None"
                description="Optional folder above the series, e.g. Volumes."
                value={form.volumesRoot}
                error={errors.volumesRoot}
                onChange={(e) => set({ volumesRoot: e.currentTarget.value })}
              />
              <TextInput
                id="media-chaptersRoot"
                label="Chapters top folder"
                placeholder="None"
                description="A different name, e.g. Chapters, keeps chapters in their own tree."
                value={form.chaptersRoot}
                error={errors.chaptersRoot}
                onChange={(e) => set({ chaptersRoot: e.currentTarget.value })}
              />
            </Group>
            <TemplateInput
              id="media-seriesFolder"
              label="Series folder"
              description="One folder per series. When it names something a copy has, such as {Reader Series} or {edition}, each copy that renders differently gets a folder of its own beside it."
              value={form.seriesFolder}
              preview={preview.data?.seriesFolder}
              error={errors.seriesFolder}
              onChange={(v) => set({ seriesFolder: v })}
              onFocus={focused('seriesFolder')}
            />
            <Select
              label="Folders inside a series"
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
              id="media-bookFolder"
              label="Book folder"
              value={form.bookFolder}
              preview={preview.data ? preview.data.bookFolder || '(directly in the series folder)' : undefined}
              error={errors.bookFolder}
              onChange={(v) => set({ bookFolder: v })}
              onFocus={focused('bookFolder')}
            />
            <TemplateInput
              id="media-volumeFile"
              label="Volume file"
              value={form.volumeFile}
              preview={preview.data?.volumeFile}
              error={errors.volumeFile}
              onChange={(v) => set({ volumeFile: v })}
              onFocus={focused('volumeFile')}
            />
            <TemplateInput
              id="media-chapterFile"
              label="Chapter file"
              value={form.chapterFile}
              preview={preview.data?.chapterFile}
              error={errors.chapterFile}
              onChange={(v) => set({ chapterFile: v })}
              onFocus={focused('chapterFile')}
            />
            <Switch
              label="Rename imported files"
              description="Off, a file keeps the name it arrived with."
              checked={form.renameImports}
              onChange={(e) => set({ renameImports: e.currentTarget.checked })}
            />
            <Switch
              label="Add the content fingerprint to file names"
              description="Off, it is only added when two different books would get the same path."
              checked={form.hashInName}
              onChange={(e) => set({ hashInName: e.currentTarget.checked })}
            />
          </Section>

          <Section
            title="Inside each book"
            description="The ComicInfo.xml Sento writes into every book. Readers file, name and describe books by it; the two names below are yours to choose, the rest comes from MangaBaka and the release."
          >
            <Switch
              label="Embed metadata in books"
              description="Every import becomes a clean CBZ with a full ComicInfo.xml. Torrent downloads are copied so the original keeps seeding, which uses extra disk space while it seeds."
              checked={form.writeComicInfo}
              onChange={(e) => set({ writeComicInfo: e.currentTarget.checked })}
            />
            {!form.writeComicInfo && (
              <Switch
                label="Copy instead of hardlink"
                description="Books keep whatever metadata they came with. Use a full copy if your filesystem does not support hardlinks."
                checked={form.alwaysCopy}
                onChange={(e) => set({ alwaysCopy: e.currentTarget.checked })}
              />
            )}
            {readerFields.map(readerInput)}
            <div>
              <Button
                variant="subtle"
                size="compact-sm"
                px={0}
                aria-expanded={allFields}
                onClick={() => setAllFields((v) => !v)}
              >
                {allFields ? 'Hide' : 'Show'} every field Sento writes ({schema.data?.comicInfo.length ?? 0})
              </Button>
              <Collapse expanded={allFields}>
                <Table verticalSpacing={4} mt="xs" withRowBorders={false}>
                  <Table.Tbody>
                    {(schema.data?.comicInfo ?? []).map((w) => (
                      <Table.Tr key={w.fields.join()}>
                        <Table.Td style={{ verticalAlign: 'top', whiteSpace: 'nowrap' }}>
                          <Group gap={4}>
                            {w.fields.map((f) => (
                              <Code key={f}>{f}</Code>
                            ))}
                          </Group>
                        </Table.Td>
                        <Table.Td>
                          <Text size="xs" c="dimmed">
                            {w.from}
                          </Text>
                        </Table.Td>
                      </Table.Tr>
                    ))}
                  </Table.Tbody>
                </Table>
              </Collapse>
            </div>
          </Section>

          {offerOrganize && !dirty && (
            <Alert
              color="blue"
              title="Existing files are unchanged"
              withCloseButton
              closeButtonLabel="Dismiss"
              onClose={() => setOfferOrganize(false)}
              mb="md"
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
          <div className={classes.saveBar}>
            <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!dirty}>
              Save changes
            </Button>
            {dirty && (
              <Button variant="subtle" color="gray" onClick={() => setForm(initial)}>
                Reset
              </Button>
            )}
            <Text size="xs" c={saved && !dirty ? 'green' : 'dimmed'}>
              {dirty ? 'Unsaved changes' : saved ? 'Saved.' : ''}
            </Text>
          </div>
          {organizing && (
            <OrganizeLibrary
              onClose={() => {
                setOrganizing(false)
                setOfferOrganize(false)
              }}
            />
          )}
        </fieldset>

        <Card padding="md" className={classes.tokens}>
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
