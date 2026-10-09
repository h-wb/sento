import { useEffect, useState, type ReactNode } from 'react'
import { useUnsavedChanges } from '../useUnsavedChanges'
import {
  Alert,
  Button,
  Checkbox,
  Group,
  Modal,
  MultiSelect,
  NumberInput,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import type {
  Edition,
  CollectedVersion,
  Book,
  CatalogLinkPreview,
  File as SeriesFile,
} from '../types/store'
import type { BookStatus } from '../types/service'
import { useConfirmAction } from './ConfirmAction'

const editionLabel = (c: Edition) =>
  [c.language, c.editionName, c.publisher, c.medium].filter(Boolean).join(' · ')
function useEditor() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  return {
    busy,
    error,
    run: async (fn: () => Promise<void>) => {
      setBusy(true)
      setError('')
      try {
        await fn()
      } catch (e) {
        setError(String(e))
      } finally {
        setBusy(false)
      }
    },
  }
}
export function VersionEditor({
  value,
  chapter,
  save,
  close,
}: {
  value: CollectedVersion
  chapter: boolean
  save: (v: CollectedVersion) => Promise<void>
  close: () => void
}) {
  const [v, set] = useState(value)
  const q = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const edit = useEditor()
  return (
    <EditorModal
      dirty={JSON.stringify(v) !== JSON.stringify(value)}
      busy={edit.busy}
      onClose={close}
      title={v.id ? 'Version settings' : 'Keep an additional version'}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void edit.run(() => save(v))
        }}
      >
        <Stack>
          <Text size="sm" c="dimmed">
            Keep another release of this edition, such as a second release group. A group preference
            alone can be set in a profile. Files are preserved when a version is removed.
          </Text>
          <fieldset className="formFields" disabled={edit.busy}>
            <Stack>
              <TextInput
                label="Version name"
                required
                value={v.name}
                onChange={(e) => set({ ...v, name: e.currentTarget.value })}
              />
              <Select
                label="Release profile"
                placeholder="Inherit edition profile"
                clearable
                data={
                  q.data?.map((p) => ({
                    value: String(p.id),
                    label: p.name,
                  })) ?? []
                }
                value={v.profileId ? String(v.profileId) : null}
                onChange={(s) => set({ ...v, profileId: Number(s) || 0 })}
              />
              <TextInput
                label="Required release group"
                description="Optional regular expression; other groups cannot satisfy this version"
                value={v.requiredGroup}
                onChange={(e) => set({ ...v, requiredGroup: e.currentTarget.value })}
              />
              {chapter && (
                <NumberInput
                  label="Skip through chapter"
                  description="Skip this chapter and earlier ones. Use 0 to skip through chapter 0."
                  value={v.cutoffNum / 100}
                  min={0}
                  decimalScale={2}
                  onChange={(n) => set({ ...v, cutoffNum: Math.round(Number(n) * 100) })}
                />
              )}
              <Checkbox
                label="Include extras and specials"
                description="Collect bonus and other books too. Individual choices override this setting."
                checked={v.includeExtras}
                onChange={(e) => set({ ...v, includeExtras: e.currentTarget.checked })}
              />
              <Checkbox
                label="Allow releases with unknown language"
                checked={v.allowUnknown}
                onChange={(e) => set({ ...v, allowUnknown: e.currentTarget.checked })}
              />
              <Checkbox
                label="Search books with unknown publication dates"
                checked={v.acquireUnknown}
                onChange={(e) => set({ ...v, acquireUnknown: e.currentTarget.checked })}
              />
              <Switch
                label="Collect this version"
                checked={v.monitored}
                onChange={(e) => set({ ...v, monitored: e.currentTarget.checked })}
              />
            </Stack>
          </fieldset>
          {edit.error && <Alert color="red">{edit.error}</Alert>}
          <Group justify="flex-end">
            <Button type="submit" loading={edit.busy}>
              Save version
            </Button>
          </Group>
        </Stack>
      </form>
    </EditorModal>
  )
}
export function BookEditor({
  value,
  save,
  remove,
  reset,
  close,
}: {
  value: Book
  save: (v: Book) => Promise<void>
  remove: () => Promise<void>
  reset: () => Promise<void>
  close: () => void
}) {
  const [v, set] = useState(value)
  const edit = useEditor()
  const { confirm, confirmation } = useConfirmAction()
  return (
    <>
      <EditorModal
        dirty={JSON.stringify(v) !== JSON.stringify(value)}
        busy={edit.busy}
        onClose={close}
        title={v.id ? 'Edit book' : 'Add book'}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void edit.run(() => save(v))
          }}
        >
          <Stack>
            <Text size="sm" c="dimmed">
              Numbers identify books within this edition. Specials can use a label without a number.
            </Text>
            <fieldset className="formFields" disabled={edit.busy}>
              <Stack>
                <TextInput
                  label="Display label"
                  required
                  value={v.sequenceStr}
                  onChange={(e) => set({ ...v, sequenceStr: e.currentTarget.value })}
                />
                <Checkbox
                  label="Has a volume or chapter number"
                  checked={v.numberKnown}
                  onChange={(e) => set({ ...v, numberKnown: e.currentTarget.checked })}
                />
                {v.numberKnown && (
                  <NumberInput
                    label="Number"
                    decimalScale={2}
                    min={0}
                    value={v.sequenceNum / 100}
                    onChange={(n) => set({ ...v, sequenceNum: Math.round(Number(n) * 100) })}
                  />
                )}
                <Select
                  label="Category"
                  description="Regular books are collected by default. Enable extras in edition settings to include bonuses."
                  data={[
                    { value: 'main', label: 'Regular' },
                    { value: 'extra', label: 'Bonus / special' },
                    { value: 'other', label: 'Other' },
                  ]}
                  value={v.countType}
                  onChange={(s) => set({ ...v, countType: s ?? 'main' })}
                />
                <TextInput
                  type="date"
                  label="Publication date"
                  value={v.releaseDate ?? ''}
                  onChange={(e) => set({ ...v, releaseDate: e.currentTarget.value })}
                />
                <TextInput
                  label="ISBN"
                  value={v.isbn ?? ''}
                  onChange={(e) => set({ ...v, isbn: e.currentTarget.value })}
                />
                <TextInput
                  label="Revision"
                  value={v.revision ?? ''}
                  onChange={(e) => set({ ...v, revision: e.currentTarget.value })}
                />
              </Stack>
            </fieldset>
            {edit.error && <Alert color="red">{edit.error}</Alert>}
            <Group justify="space-between">
              {v.providerId && (
                <Button variant="subtle" disabled={edit.busy} onClick={() => void edit.run(reset)}>
                  Use catalog values
                </Button>
              )}
              {v.id > 0 && (
                <Button
                  color="red"
                  variant="subtle"
                  onClick={() =>
                    confirm({
                      title: v.providerId ? 'Retire catalog book?' : 'Delete book?',
                      message: 'Owned books must be unlinked first. Files stay on disk.',
                      label: v.providerId ? 'Retire book' : 'Delete book',
                      onConfirm: remove,
                    })
                  }
                >
                  {v.providerId ? 'Retire book' : 'Delete book'}
                </Button>
              )}
              <Group>
                <Button type="submit" loading={edit.busy}>
                  Save book
                </Button>
              </Group>
            </Group>
          </Stack>
        </form>
      </EditorModal>
      {confirmation}
    </>
  )
}
export function EditionEditor({
  value,
  save,
  reset,
  close,
}: {
  value: Edition
  save: (p: {
    profileId?: number
    editionName: string
    language: string
    publisher: string
    medium: string
    editionTraits: string[]
    contentVariant: string
    numberingSource: string
    discoverChapters?: boolean
  }) => Promise<void>
  reset: () => Promise<void>
  close: () => void
}) {
  const [v, set] = useState(value)
  const edit = useEditor()
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
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
          <Text size="sm" c="dimmed">
            Edits are preserved during catalog refresh. To collect a different language or edition,
            add an edition.
          </Text>
          {(['editionName', 'language', 'publisher', 'medium'] as const).map((k) => (
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
              required={k === 'editionName' || k === 'language'}
              value={v[k]}
              disabled={edit.busy}
              onChange={(e) => set({ ...v, [k]: e.currentTarget.value })}
            />
          ))}
          <Select
            label="Edition release profile"
            description="Used by versions that inherit this edition’s profile."
            placeholder="Inherit series profile"
            clearable
            data={(profiles.data ?? []).map((p) => ({ value: String(p.id), label: p.name }))}
            value={v.profileId ? String(v.profileId) : null}
            onChange={(id) => set({ ...v, profileId: Number(id) || 0 })}
            disabled={edit.busy}
          />
          <MultiSelect
            label="Edition traits"
            description="Choose all known traits. Leave blank when unspecified."
            data={[
              { value: 'standard', label: 'Standard numbering' },
              { value: 'omnibus', label: 'Omnibus' },
              { value: 'deluxe', label: 'Deluxe' },
              { value: 'boxset', label: 'Box set' },
            ]}
            value={v.editionTraits ?? []}
            onChange={(editionTraits) => set({ ...v, editionTraits })}
            disabled={edit.busy}
          />
          <Select
            label="Content version"
            data={[
              { value: 'unknown', label: 'Unspecified' },
              { value: 'original', label: 'Original' },
              { value: 'colored', label: 'Colored' },
            ]}
            value={v.contentVariant || 'unknown'}
            onChange={(variant) => set({ ...v, contentVariant: variant ?? 'unknown' })}
            disabled={edit.busy}
          />
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
                Use catalog values
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
export function CatalogLink({
  seriesID,
  value,
  editions,
  catalogId,
  done,
  close,
}: {
  seriesID: number
  value: Edition
  editions: Edition[]
  // A suggested catalog edition: preselected, its matches shown at once.
  catalogId?: number
  done: () => Promise<void>
  close: () => void
}) {
  const [id, setID] = useState<string | null>(catalogId ? String(catalogId) : null)
  const [preview, setPreview] = useState<CatalogLinkPreview | null>(null)
  const edit = useEditor()
  const loadPreview = (catalog: number) =>
    edit.run(async () => setPreview(await api.catalogLinkPreview(seriesID, value.id, catalog)))
  useEffect(() => {
    if (catalogId) void loadPreview(catalogId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const added = preview
    ? preview.catalogBooks.filter((b) => !Object.values(preview.mapping).includes(b.id)).length
    : 0
  return (
    <Modal
      opened
      onClose={() => {
        if (!edit.busy) close()
      }}
      title="Link edition to MangaBaka"
    >
      <Stack>
        <Text size="sm">
          Your edition becomes this catalog edition and follows MangaBaka from now on. Its files,
          collected versions, and the fields you edited by hand are kept. Review the book matches
          before linking.
        </Text>
        <Select
          label="Catalog edition"
          data={editions
            .filter(
              (c) =>
                !c.custom &&
                c.id !== value.id &&
                c.type === value.type &&
                c.language === value.language,
            )
            .map((c) => ({ value: String(c.id), label: editionLabel(c) }))}
          value={id}
          onChange={(v) => {
            setID(v)
            setPreview(null)
          }}
        />
        <Button
          variant="default"
          disabled={!id}
          loading={edit.busy}
          onClick={() => void loadPreview(Number(id))}
        >
          Preview matches
        </Button>
        {preview && (
          <>
            <Text size="sm">
              {Object.keys(preview.mapping).length} of your books match the catalog
              {preview.unmatched.length > 0 && `; ${preview.unmatched.length} do not and stay as they are`}
              {added > 0 && `; ${added} more from the catalog will be added`}.
            </Text>
            <Stack gap={4}>
              {preview.localBooks.map((w) => (
                <Text key={w.id} size="xs">
                  {w.sequenceStr} · {w.countType} →{' '}
                  {preview.mapping[w.id]
                    ? `${preview.catalogBooks.find((v) => v.id === preview.mapping[w.id])?.sequenceStr} · ${preview.catalogBooks.find((v) => v.id === preview.mapping[w.id])?.countType}`
                    : 'No catalog match; kept locally'}
                </Text>
              ))}
            </Stack>
            <Text size="xs" c="dimmed">
              The separate catalog entry is folded into your edition, so it appears once.
            </Text>
          </>
        )}
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Group justify="flex-end">
          <Button variant="default" disabled={edit.busy} onClick={close}>
            Cancel
          </Button>
          <Button
            disabled={!preview}
            loading={edit.busy}
            onClick={() =>
              void edit.run(async () => {
                await api.linkCatalog(seriesID, value.id, Number(id))
                await done()
              })
            }
          >
            Link edition
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
export function MappingEditor({
  seriesID,
  value,
  versions,
  existing,
  books,
  editions,
  save,
  close,
}: {
  seriesID: number
  value: SeriesFile
  versions: CollectedVersion[]
  existing: import('../types/store').AssetCoverage[]
  books: BookStatus[]
  editions: Edition[]
  save: (books: import('../types/store').BookRef[]) => Promise<void>
  close: () => void
}) {
  const initial = existing.map((a) => ({
    versionId: a.versionId,
    bookId: a.bookId,
    editionId: versions.find((t) => t.id === a.versionId)?.editionId ?? 0,
    number: books.find((s) => s.book.id === a.bookId)?.book.sequenceNum ?? 0,
    ledger:
      editions.find((c) => c.id === versions.find((t) => t.id === a.versionId)?.editionId)
        ?.type ?? 'volume',
  }))
  const [draft, setDraft] = useState(initial)
  const [version, setVersion] = useState<string | null>(
    existing[0] ? String(existing[0].versionId) : null,
  )
  const edit = useEditor()
  const [language, setLanguage] = useState(value.facts?.language ?? '')
  const [group, setGroup] = useState(value.facts?.group ?? '')
  const t = versions.find((t) => String(t.id) === version)
  const chosen = draft.filter((s) => s.versionId === t?.id).map((s) => String(s.bookId))
  return (
    <EditorModal
      dirty={
        JSON.stringify(draft) !== JSON.stringify(initial) ||
        language !== (value.facts?.language ?? '') ||
        group !== (value.facts?.group ?? '')
      }
      busy={edit.busy}
      onClose={close}
      title="Match imported file"
    >
      <Stack>
        <Text size="xs" style={{ overflowWrap: 'anywhere' }}>
          {value.path}
        </Text>
        <Text size="sm">
          Confirm the edition and books this archive contains. This confirmation overrides parsed
          language for coverage. Group requirements still apply. A pack can cover several books.
          Existing mappings to other versions are kept.
        </Text>
        {value.facts && (
          <Text size="xs" c="dimmed">
            {[
              value.facts.language || 'Language unknown',
              value.facts.group || 'Group unknown',
              value.facts.format,
            ]
              .filter(Boolean)
              .join(' · ')}
          </Text>
        )}
        <Text size="xs" c="dimmed">
          Release advertised:{' '}
          {[value.claims?.language, value.claims?.group].filter(Boolean).join(' · ') ||
            'No claims recorded'}
        </Text>
        <Group grow>
          <TextInput
            label="Confirmed language"
            value={language}
            onChange={(e) => setLanguage(e.currentTarget.value)}
          />
          <TextInput
            label="Confirmed release group"
            value={group}
            onChange={(e) => setGroup(e.currentTarget.value)}
          />
        </Group>
        <Select
          label="Collected version"
          data={versions.map((t) => ({
            value: String(t.id),
            label: `${editionLabel(editions.find((c) => c.id === t.editionId)!)} · ${t.name}`,
          }))}
          value={version}
          onChange={setVersion}
        />
        <MultiSelect
          label="Books in this file"
          searchable
          disabled={!t}
          data={books
            .filter((s) => s.versionId === t?.id)
            .map((s) => ({ value: String(s.book.id), label: s.book.sequenceStr }))}
          value={chosen}
          onChange={(ids) => {
            if (!t) return
            setDraft([
              ...draft.filter((s) => s.versionId !== t.id),
              ...ids.map((id) => ({
                versionId: t.id,
                bookId: Number(id),
                editionId: t.editionId,
                number: books.find((s) => s.book.id === Number(id))?.book.sequenceNum ?? 0,
                ledger: editions.find((c) => c.id === t.editionId)?.type ?? 'volume',
              })),
            ])
          }}
        />
        <Stack gap={2}>
          {draft.map((s) => (
            <Text size="xs" key={`${s.versionId}-${s.bookId}`}>
              {versions.find((t) => t.id === s.versionId)?.name} ·{' '}
              {books.find((u) => u.book.id === s.bookId)?.book.sequenceStr}
            </Text>
          ))}
        </Stack>
        {draft.length === 0 && (
          <Text size="sm" c="dimmed">
            This file will stay in the library without claiming ownership of any book.
          </Text>
        )}
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Group justify="space-between">
          <Button
            variant="subtle"
            color="red"
            disabled={edit.busy || draft.length === 0}
            onClick={() => setDraft([])}
          >
            Clear mappings
          </Button>
          <Button
            loading={edit.busy}
            onClick={() =>
              void edit.run(async () => {
                if (
                  language !== (value.facts?.language ?? '') ||
                  group !== (value.facts?.group ?? '')
                )
                  await api.correctFileFacts(seriesID, value.id, { language, group })
                await save(draft)
              })
            }
          >
            Save coverage
          </Button>
        </Group>
      </Stack>
    </EditorModal>
  )
}

export function EditorModal({
  dirty,
  busy,
  onClose,
  title,
  children,
  size,
}: {
  dirty: boolean
  busy: boolean
  onClose: () => void
  title: string
  children: ReactNode
  size?: string
}) {
  useUnsavedChanges(dirty)
  const { confirm, confirmation } = useConfirmAction()
  const close = () => {
    if (busy) return
    if (!dirty) {
      onClose()
      return
    }
    confirm({
      title: 'Discard unsaved changes?',
      message: 'Your edits have not been saved.',
      label: 'Discard changes',
      onConfirm: async () => onClose(),
    })
  }
  return (
    <>
      <Modal
        opened
        title={title}
        size={size}
        onClose={close}
        closeOnEscape={!busy}
        closeOnClickOutside={!busy}
        withCloseButton={!busy}
      >
        {children}
      </Modal>
      {confirmation}
    </>
  )
}
