import {
Alert,
Button,
Text
} from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useMutation,useQuery,useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api,type MediaSettings } from '../api'
import { ComicInfoSection } from '../components/media/ComicInfoSection'
import { DiskSection } from '../components/media/DiskSection'
import { ReaderSetupSection } from '../components/media/ReaderSetupSection'
import { TokenTable } from '../components/media/TokenTable'
import { useTemplateCaret } from '../components/media/useTemplateCaret'
import OrganizeLibrary from '../components/OrganizeLibrary'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { TemplateInput } from '../components/TemplateInput'
import type { NamingSetup } from '../types/service'
import { notify,usePageTitle } from '../ui'
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

// A setup is the settings it fills; it is recognised by them, not stored.
const matches = (s: NamingSetup, m: MediaSettings) =>
  s.seriesFolder === m.seriesFolder &&
  s.bookFolder === m.bookFolder &&
  Object.entries(s.readerFields).every(([k, v]) => m.readerFields[k] === v)

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

  const { focused, insertToken } = useTemplateCaret(set, setReaderField)
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
    <div className={classes.page}>
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

          <ReaderSetupSection setups={setups} setup={setup} form={form} set={set} preview={preview} />

          <DiskSection form={form} set={set} preview={preview} errors={errors} focused={focused} pick={pick} />

          <ComicInfoSection form={form} set={set} readerFields={readerFields} readerInput={readerInput} allFields={allFields} setAllFields={setAllFields} schema={schema} />

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

          <TokenTable schema={schema} insertToken={insertToken} />

      </div>
    </div>
  )
}
