import { useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Code,
  Group,
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
]

export default function MediaPage() {
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['media-settings'], queryFn: api.mediaSettings })

  if (settings.isPending) return null
  if (settings.error) return <Alert color="red">{String(settings.error)}</Alert>
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
      onSaved()
    },
  })

  const dirty = JSON.stringify(form) !== JSON.stringify(initial)

  return (
    <>
      <Title order={2} mb="xs">
        Media Management
      </Title>
      <Text size="sm" c="dimmed" mb="md">
        How finished downloads are post-processed into the library. Changes apply to future
        imports — existing files are never renamed.
      </Text>

      <Group align="flex-start" gap="xl" wrap="wrap">
        <Stack style={{ flex: 1, minWidth: 380, maxWidth: 560 }}>
          <Title order={5}>Naming</Title>
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
          <TextInput
            label="Chapter subfolder"
            description="Chapters land in this folder inside the series folder; empty = alongside volumes"
            value={form.chapterDir}
            onChange={(e) => set({ chapterDir: e.currentTarget.value })}
          />

          <Title order={5} mt="sm">
            Post-processing
          </Title>
          <Switch
            label="Rename imported files"
            description="Off: keep the release's original filename, only the folder structure applies"
            checked={form.renameImports}
            onChange={(e) => set({ renameImports: e.currentTarget.checked })}
          />
          <Switch
            label="Copy instead of hardlink"
            description="Torrent imports hardlink so seeding continues without using twice the space; force a real copy if your filesystem misbehaves with hardlinks"
            checked={form.alwaysCopy}
            onChange={(e) => set({ alwaysCopy: e.currentTarget.checked })}
          />
          <Switch
            label="Write ComicInfo.xml"
            description="Embed series metadata into imported archives (source downloads only; hardlinked torrents are never rewritten)"
            checked={form.writeComicInfo}
            onChange={(e) => set({ writeComicInfo: e.currentTarget.checked })}
          />

          <Group mt="sm">
            <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!dirty}>
              Save
            </Button>
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
          {save.error && <Alert color="red">{String(save.error)}</Alert>}
        </Stack>

        <Card withBorder padding="sm" style={{ minWidth: 320 }}>
          <Title order={6} mb="xs">
            Tokens
          </Title>
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
        </Card>
      </Group>
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
      <Text size="xs" c="dimmed" mt={2} style={{ fontFamily: 'monospace' }}>
        → {preview ?? '…'}
      </Text>
    </div>
  )
}
