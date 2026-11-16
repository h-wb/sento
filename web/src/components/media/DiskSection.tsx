import { Group, Select, Switch, Text, TextInput } from '@mantine/core'
import { api, type MediaSettings } from '../../api'
import { PathTree } from '../PathTree'
import { TemplateInput } from '../TemplateInput'
import { Section } from './Section'

// What sits inside a series folder. A layout naming a series folder sets
// that too; the others keep it, unless it was the per-edition one.
const seriesFolderDefault = '{Series Title} [mangabaka-{seriesId}]'
const seriesFolderByEdition =
  '{Series Title} [mangabaka-{seriesId}] [{edition}][{lang}] ({version})'
const layouts: { value: string; label: string; template: string; seriesFolder?: string }[] = [
  {
    value: 'edition',
    label: 'A folder per edition — Standard Edition [en]',
    template: '{edition} [{lang}] ({version})',
  },
  {
    value: 'type',
    label: 'By type, then edition — Volumes/Standard Edition [en]',
    template: '{type}/{edition} [{lang}] ({version})',
  },
  { value: 'flat', label: 'None — books directly in the series folder', template: '' },
  {
    value: 'editionFolders',
    label: 'A series folder per edition instead — ONE PIECE [Standard Edition][en] (Colored)',
    template: '',
    seriesFolder: seriesFolderByEdition,
  },
  {
    value: 'ids',
    label: 'By internal id — editions/42/versions/81',
    template: 'editions/{editionId}/versions/{versionId}',
  },
]

export function DiskSection({
  form,
  set,
  preview,
  errors,
  focused,
  pick,
}: {
  form: MediaSettings
  set: (patch: Partial<MediaSettings>) => void
  preview: { data: Awaited<ReturnType<typeof api.previewNaming>> | undefined }
  errors: Record<string, string>
  focused: (field: string) => (input: HTMLInputElement) => void
  pick: (setting: string) => void
}) {
  return (
    <Section
      title="On disk"
      description="Where each book's file goes, one level at a time. The tree shows three sample books; pick a level to change the setting that makes it."
    >
      {preview.data?.paths ? (
        <PathTree examples={preview.data.paths} onPick={pick} />
      ) : (
        <Text size="sm" c="dimmed">
          Loading…
        </Text>
      )}
      <div>
        <Group grow align="flex-start">
          <TextInput
            id="media-volumesRoot"
            label="Volumes top folder"
            placeholder="None"
            value={form.volumesRoot}
            error={errors.volumesRoot}
            onChange={(e) => set({ volumesRoot: e.currentTarget.value })}
          />
          <TextInput
            id="media-chaptersRoot"
            label="Chapters top folder"
            placeholder="None"
            value={form.chaptersRoot}
            error={errors.chaptersRoot}
            onChange={(e) => set({ chaptersRoot: e.currentTarget.value })}
          />
        </Group>
        <Text size="xs" c="dimmed" mt={6}>
          Optional folders above every series, such as Volumes and Chapters. Two different names
          keep chapters in a tree of their own.
        </Text>
      </div>
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
            (l) =>
              l.template === form.bookFolder &&
              (!l.seriesFolder || l.seriesFolder === form.seriesFolder),
          )?.value ?? 'custom'
        }
        onChange={(v) => {
          const layout = layouts.find((l) => l.value === v)
          if (!layout) return
          const seriesFolder =
            layout.seriesFolder ??
            (form.seriesFolder === seriesFolderByEdition ? seriesFolderDefault : form.seriesFolder)
          set({ bookFolder: layout.template, seriesFolder, hashInName: layout.value === 'ids' })
        }}
        data={[
          ...layouts.map(({ value, label }) => ({ value, label })),
          { value: 'custom', label: 'Custom' },
        ]}
      />
      <TemplateInput
        id="media-bookFolder"
        label="Book folder"
        value={form.bookFolder}
        preview={
          preview.data ? preview.data.bookFolder || '(directly in the series folder)' : undefined
        }
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
  )
}
