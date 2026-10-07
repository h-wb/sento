import { useState } from 'react'
import {
  Button,
  Card,
  Code,
  Accordion,
  Group,
  Select,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type MediaSettings } from '../api'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { useUnsavedChanges } from '../useUnsavedChanges'

// Token reference shown next to the templates ({token:000} pads numbers).
const tokens: [string, string][] = [
  ['{Series Title}', 'ONE PIECE'],
  ['{Series TitleSlug}', 'one-piece'],
  ['{seriesId}', 'MangaBaka id — 377'],
  ['{volume:00}', '112 (padding via :00)'],
  ['{chapter:000}', '1102.5 (padding on the integer part)'],
  ['{edition}', 'Standard Edition'],
  ['{lang}', 'en'],
  ['{group}', 'release/scanlation group'],
  ['{isbn}', 'ISBN when known'],
  ['{year}', '1997'],
  ['{publisher}', 'edition publisher (folders)'],
  ['{medium}', 'paperback, digital… (folders)'],
  ['{type}', 'Volumes or Chapters (folders)'],
  ['{version}', 'name of an additional version; empty for the default one (folders)'],
  ['{collectionId} {versionId}', 'internal ids that never change (folders)'],
]

// Folder layout presets; the templates mirror the backend's constants.
const layouts = [
  { value: 'edition', label: 'By edition — Standard Edition [en]', template: '{edition} [{lang}] ({version})' },
  { value: 'type', label: 'By type, then edition — Volumes/Standard Edition [en]', template: '{type}/{edition} [{lang}] ({version})' },
  { value: 'flat', label: 'Flat — everything in the series folder', template: '' },
  { value: 'ids', label: 'By internal id — collections/42/versions/81', template: 'collections/{collectionId}/versions/{versionId}' },
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
  const set = (patch: Partial<MediaSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setSaved(false)
  }

  // Preview renders server-side (the Go renderer is the only truth);
  // debounced so typing doesn't spam the API.
  const [debounced] = useDebouncedValue(form, 300)
  const preview = useQuery({
    queryKey: ['media-preview', debounced],
    queryFn: () => api.previewNaming(debounced),
    placeholderData: (prev) => prev,
  })

  const save = useMutation({
    mutationFn: () => api.saveMediaSettings(form),
    onSuccess: () => {
      setSaved(true)
      notify.ok('Media settings saved — applies to future imports')
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
        description="File naming and import settings. Changes apply to future imports."
      />

      <div className="settingsLayout">
        <fieldset className="formFields" disabled={save.isPending}><Stack>
          <Title order={4}>File naming</Title>
          <QueryFeedback error={preview.error} onRetry={() => void preview.refetch()} />
          <TemplateInput
            label="Series folder"
            value={form.seriesFolder}
            preview={preview.data?.seriesFolder}
            onChange={(v) => set({ seriesFolder: v })}
          />
          <TemplateInput
            label="Volume file"
            value={form.volumeFile}
            preview={preview.data?.volumeFile}
            onChange={(v) => set({ volumeFile: v })}
          />
          <TemplateInput
            label="Chapter file"
            value={form.chapterFile}
            preview={preview.data?.chapterFile}
            onChange={(v) => set({ chapterFile: v })}
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
              onChange={(e) => set({ volumesRoot: e.currentTarget.value })}
            />
            <TextInput
              label="Chapters top folder"
              placeholder="None"
              description="Give it a different name, e.g. Chapters, to keep chapters in their own tree."
              value={form.chaptersRoot}
              onChange={(e) => set({ chaptersRoot: e.currentTarget.value })}
            />
          </Group>
          <Select
            label="Folders inside a series"
            description="How editions and additional versions are separated. Applies to new imports."
            allowDeselect={false}
            value={layouts.find((l) => l.template === form.bookFolder)?.value ?? 'custom'}
            onChange={(v) => {
              const layout = layouts.find((l) => l.value === v)
              if (layout) set({ bookFolder: layout.template, hashInName: layout.value === 'ids' })
            }}
            data={[...layouts.map(({ value, label }) => ({ value, label })), { value: 'custom', label: 'Custom' }]}
          />
          <TemplateInput
            label="Book folder"
            value={form.bookFolder}
            preview={
              preview.data ? preview.data.bookFolder || '(directly in the series folder)' : undefined
            }
            onChange={(v) => set({ bookFolder: v })}
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
              Existing files move when you run “Organize files” on a series.
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
          </Group>
        </Stack></fieldset>

        <Card padding="md"><Accordion variant="default"><Accordion.Item value="tokens"><Accordion.Control>Naming tokens</Accordion.Control><Accordion.Panel>

          <Table withRowBorders={false} verticalSpacing={2}>
            <Table.Tbody>
              {tokens.map(([tok, desc]) => (
                <Table.Tr key={tok}>
                  <Table.Td>
                    <Code>{tok}</Code>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {desc}
                    </Text>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
          <Text size="xs" c="dimmed" mt="xs">
            Brackets around an empty token disappear: <Code>[{'{group}'}]</Code> vanishes when
            there is no group.
          </Text>
        </Accordion.Panel></Accordion.Item></Accordion></Card>
      </div>
    </>
  )
}

function TemplateInput({
  label,
  value,
  preview,
  onChange,
}: {
  label: string
  value: string
  preview?: string
  onChange: (v: string) => void
}) {
  return (
    <div>
      <TextInput label={label} value={value} onChange={(e) => onChange(e.currentTarget.value)} />
      <Text size="xs" c="dimmed" mt={2} style={{ fontFamily: 'monospace', overflowWrap: 'anywhere' }}>
        Preview: {preview ?? 'Loading…'}
      </Text>
    </div>
  )
}
