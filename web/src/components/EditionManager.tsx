import EditionActions from './EditionActions'
import { useState, type ReactNode } from 'react'
import { useMediaQuery } from '@mantine/hooks'
import { useUnsavedChanges } from '../useUnsavedChanges'
import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  Group,
  Modal,
  MultiSelect,
  Pagination,
  NumberInput,
  Select,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
} from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type {
  Collection,
  DownloadTarget,
  Work,
  CatalogLinkPreview,
  File as SeriesFile,
} from '../types/store'
import type { UnitSlot } from '../types/service'
import type { SlotSelection } from './SlotModal'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'

const editionLabel = (c: Collection) =>
  [c.language, c.editionName, c.publisher, c.medium].filter(Boolean).join(' · ')
const stateLabel: Record<string, string> = {
  owned: 'Owned',
  pending: 'Pending',
  missing: 'Missing',
  not_released: 'Not released',
  unmonitored: 'Paused',
  unknown: 'Date unknown',
  upgrade_eligible: 'Upgrade wanted',
}

type Props = {
  seriesID: number
  collections: Collection[]
  files: SeriesFile[]
  onSlot: (s: SlotSelection) => void
  onAddEdition: () => void
}
export default function EditionManager({
  seriesID,
  collections,
  files,
  onSlot,
  onAddEdition,
}: Props) {
  const qc = useQueryClient()
  const small = useMediaQuery('(max-width: 48em)')
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const targets = useQuery({
    queryKey: ['series', seriesID, 'targets'],
    queryFn: () => api.targets(seriesID),
  })
  const slots = useQuery({
    queryKey: ['series', seriesID, 'target-slots'],
    queryFn: () => api.targetSlots(seriesID),
  })
  const bindings = useQuery({
    queryKey: ['series', seriesID, 'bindings'],
    queryFn: () => api.bindings(seriesID),
  })
  const [pages, setPages] = useState<Record<number, number>>({})
  const coverage = useQuery({
    queryKey: ['series', seriesID, 'coverage'],
    queryFn: () => api.coverage(seriesID),
  })
  const observations = useQuery({
    queryKey: ['series', seriesID, 'observations'],
    queryFn: () => api.observations(seriesID),
  })
  const [chosen, setChosen] = useState<Record<number, string>>({})
  const [target, setTarget] = useState<DownloadTarget | null>(null)
  const [unit, setUnit] = useState<Work | null>(null)
  const [edition, setEdition] = useState<Collection | null>(null)
  const [link, setLink] = useState<Collection | null>(null)
  const [file, setFile] = useState<SeriesFile | null>(null)
  const [relationships, setRelationships] = useState(false)
  const [showFiles, setShowFiles] = useState(false)
  const [fileFilter, setFileFilter] = useState('')
  const [filePage, setFilePage] = useState(1)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const { confirm, confirmation } = useConfirmAction()
  const changed = async () => {
    await qc.invalidateQueries({ queryKey: ['series', seriesID] })
  }
  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError('')
    try {
      await fn()
      await changed()
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }
  const allTargets = targets.data ?? []
  const visible = collections
    .filter((c) => c.syncState !== 'superseded')
    .sort(
      (a, b) =>
        Number(b.monitored) - Number(a.monitored) ||
        Number(a.type === 'chapter') - Number(b.type === 'chapter') ||
        a.editionName.localeCompare(b.editionName),
    )
  const pending = targets.isPending || slots.isPending
  const allUnits = Object.values(slots.data ?? {}).flat()
  return (
    <Stack gap="lg">
      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          Editions separate languages and publication formats. Collected versions keep different
          releases of the same edition.
        </Text>
        <Group>
          <Button size="xs" variant="default" onClick={() => setRelationships(true)}>
            Content relationships
          </Button>
          <Button size="xs" onClick={onAddEdition}>
            Add edition
          </Button>
        </Group>
      </Group>
      <QueryFeedback
        pending={pending}
        error={targets.error || slots.error}
        onRetry={() => {
          void targets.refetch()
          void slots.refetch()
        }}
      />
      {error && <Alert color="red">{error}</Alert>}
      {!pending &&
        visible.map((c) => {
          const versions = allTargets.filter((t) => t.collectionId === c.id)
          const t =
            versions.find((t) => String(t.id) === chosen[c.id]) ??
            versions.find((t) => t.isDefault) ??
            versions[0]
          const units = t ? (slots.data?.[String(t.id)] ?? []) : []
          const owned = units.filter((s) => s.fileIds.length > 0).length
          const show = expanded[c.id] ?? (c.type === 'volume' && !!t?.monitored)
          const page = Math.min(pages[t?.id ?? 0] ?? 1, Math.max(1, Math.ceil(units.length / 25)))
          const pageUnits = units.slice((page - 1) * 25, page * 25)
          const actions = (s: UnitSlot) => (
            <Group gap={4} wrap="wrap">
              <Checkbox
                aria-label={`Collect unit ${s.work.sequenceStr}`}
                checked={s.monitored}
                disabled={busy}
                onChange={(e) =>
                  void run(() =>
                    api.unitMonitor(seriesID, t.id, s.work.id, e.currentTarget.checked),
                  )
                }
              />
              <Button size="compact-xs" variant="subtle" onClick={() => setUnit(s.work)}>
                Edit
              </Button>
              {s.fileIds.length > 0 && (
                <Button
                  size="compact-xs"
                  variant="subtle"
                  onClick={() => {
                    const f = files.find((f) => f.id === s.fileIds[0])
                    if (f) setFile(f)
                  }}
                >
                  File mappings
                </Button>
              )}
              {s.fileIds.map((fid) => {
                const a = coverage.data?.find(
                  (a) => a.fileId === fid && a.targetId === t.id && a.workId === s.work.id,
                )
                return a ? (
                  <Button
                    key={fid}
                    size="compact-xs"
                    variant="subtle"
                    disabled={busy}
                    onClick={() =>
                      void run(() =>
                        api.pinCoverage(seriesID, {
                          ...a,
                          pinned: !a.pinned,
                        }),
                      )
                    }
                  >
                    {a.pinned ? 'Unpin copy' : 'Pin copy'}
                  </Button>
                ) : null
              })}
            </Group>
          )
          return (
            <Card key={c.id} padding="md" withBorder>
              <Stack gap="sm">
                <Group justify="space-between" align="flex-start">
                  <div style={{ minWidth: 0 }}>
                    <Text fw={600} style={{ overflowWrap: 'anywhere' }}>
                      {c.editionName}
                    </Text>
                    <Text size="xs" c="dimmed" mt={4}>
                      {[c.language, c.publisher, c.medium].filter(Boolean).join(' · ')}
                    </Text>
                    <Group gap="xs" mt={4}>
                      <Badge size="xs" variant="outline">
                        {c.type}
                      </Badge>
                      <Badge size="xs" color={c.syncState === 'stale' ? 'orange' : 'gray'}>
                        {c.syncState === 'stale'
                          ? 'Catalog entry missing'
                          : c.custom
                            ? 'Local edition'
                            : 'MangaBaka'}
                      </Badge>
                      <Text size="xs" c="dimmed">
                        {owned}/{units.length} owned
                      </Text>
                    </Group>
                  </div>
                  <Group gap="xs">
                    <Button size="compact-xs" variant="subtle" onClick={() => setEdition(c)}>
                      Edit edition
                    </Button>
                    {c.custom && (
                      <EditionActions
                        seriesID={seriesID}
                        collection={c}
                        others={visible.filter((o) => o.id !== c.id)}
                        run={async (fn) => {
                          await run(fn)
                        }}
                      />
                    )}
                    {c.custom && (
                      <Button size="compact-xs" variant="subtle" onClick={() => setLink(c)}>
                        Link to catalog
                      </Button>
                    )}
                  </Group>
                </Group>
                {t && (
                  <Group align="flex-end">
                    <Select
                      label="Collected version"
                      data={versions.map((v) => ({
                        value: String(v.id),
                        label: v.name,
                      }))}
                      value={String(t.id)}
                      onChange={(v) => {
                        if (v) setChosen({ ...chosen, [c.id]: v })
                      }}
                      style={{ minWidth: 170, flex: 1, maxWidth: 300 }}
                    />
                    <Switch
                      label="Collect this version"
                      checked={t.monitored}
                      disabled={busy}
                      onChange={(e) =>
                        void run(() =>
                          api.saveTarget(seriesID, {
                            ...t,
                            monitored: e.currentTarget.checked,
                          }),
                        )
                      }
                    />
                    <Button variant="default" size="xs" onClick={() => setTarget(t)}>
                      Version settings
                    </Button>
                    <Button
                      variant="subtle"
                      size="xs"
                      onClick={() =>
                        setTarget({
                          ...t,
                          id: 0,
                          name: '',
                          isDefault: false,
                          requiredGroup: '',
                        })
                      }
                    >
                      Add collected version
                    </Button>
                    <Button
                      variant="default"
                      size="xs"
                      onClick={() =>
                        onSlot({
                          ledger: c.type === 'chapter' ? 'chapter' : 'volume',
                          collectionId: c.id,
                          targetId: t.id,
                          all: true,
                          number: 0,
                          collectionName: `${editionLabel(c)} · ${t.name}`,
                          monitored: t.monitored,
                          wanted: true,
                          fileIds: [],
                          autoSearch: true,
                        })
                      }
                    >
                      Find missing releases
                    </Button>
                  </Group>
                )}
                <Group>
                  <Button
                    size="compact-xs"
                    variant="subtle"
                    onClick={() =>
                      setUnit({
                        id: 0,
                        collectionId: c.id,
                        sequenceNum: 0,
                        sequenceStr: '',
                        numberKnown: true,
                        countType: 'main',
                        pages: 0,
                        syncState: 'local',
                      })
                    }
                  >
                    Add unit
                  </Button>
                  {t && !t.isDefault && (
                    <Button
                      size="compact-xs"
                      variant="subtle"
                      color="red"
                      onClick={() =>
                        confirm({
                          title: 'Remove collected version?',
                          message:
                            'Files stay on disk. Other versions keep their files and ownership.',
                          label: 'Remove version',
                          onConfirm: async () => {
                            await api.deleteTarget(seriesID, t.id)
                            await changed()
                          },
                        })
                      }
                    >
                      Remove version
                    </Button>
                  )}
                </Group>
                {units.length > 0 && (
                  <Button
                    variant="subtle"
                    size="compact-xs"
                    aria-expanded={show}
                    onClick={() => setExpanded({ ...expanded, [c.id]: !show })}
                  >
                    {show ? 'Hide' : 'Show'} {c.type === 'chapter' ? 'chapters' : 'units'} (
                    {units.length})
                  </Button>
                )}
                {show &&
                  (units.length === 0 ? (
                    <Text size="sm" c="dimmed">
                      No units are known yet. Add volumes, chapters, or specials explicitly.
                    </Text>
                  ) : small ? (
                    <Stack gap="xs">
                      {pageUnits.map((s) => (
                        <Card key={s.work.id} withBorder padding="xs">
                          <Stack gap={6}>
                            <Group justify="space-between">
                              <Button
                                variant="subtle"
                                size="compact-xs"
                                onClick={() => onSlot(selection(c, t, s))}
                              >
                                {s.work.sequenceStr}
                              </Button>
                              <Badge size="xs" color={s.fileIds.length ? 'green' : 'gray'}>
                                {stateLabel[s.state] ?? s.state}
                              </Badge>
                            </Group>
                            <Text size="xs" c="dimmed">
                              {s.work.releaseDate || 'Publication date unknown'}
                              {s.work.countType !== 'main' ? ` · ${s.work.countType}` : ''}
                            </Text>
                            {actions(s)}
                          </Stack>
                        </Card>
                      ))}
                    </Stack>
                  ) : (
                    <Table.ScrollContainer minWidth={530}>
                      <Table verticalSpacing="xs" style={{ tableLayout: 'fixed' }}>
                        <Table.Thead>
                          <Table.Tr>
                            <Table.Th w="27%">Unit</Table.Th>
                            <Table.Th w="20%">Published</Table.Th>
                            <Table.Th w="18%">Status</Table.Th>
                            <Table.Th>Actions</Table.Th>
                          </Table.Tr>
                        </Table.Thead>
                        <Table.Tbody>
                          {pageUnits.map((s) => (
                            <Table.Tr key={s.work.id}>
                              <Table.Td>
                                <Button
                                  variant="subtle"
                                  size="compact-xs"
                                  onClick={() => onSlot(selection(c, t, s))}
                                >
                                  {s.work.sequenceStr}
                                </Button>
                                <Text size="xs" c="dimmed">
                                  {s.work.countType === 'main' ? '' : s.work.countType}
                                  {s.work.revision ? ` · ${s.work.revision}` : ''}
                                </Text>
                              </Table.Td>
                              <Table.Td>
                                <Text size="xs">{s.work.releaseDate || 'Unknown'}</Text>
                              </Table.Td>
                              <Table.Td>
                                <Badge
                                  size="xs"
                                  color={
                                    s.state === 'owned'
                                      ? 'green'
                                      : s.state === 'pending'
                                        ? 'blue'
                                        : 'gray'
                                  }
                                >
                                  {stateLabel[s.state] ?? s.state}
                                </Badge>
                              </Table.Td>
                              <Table.Td>{actions(s)}</Table.Td>
                            </Table.Tr>
                          ))}
                        </Table.Tbody>
                      </Table>
                    </Table.ScrollContainer>
                  ))}
                {show && units.length > 25 && (
                  <Pagination
                    size="sm"
                    total={Math.ceil(units.length / 25)}
                    value={page}
                    onChange={(page) => setPages({ ...pages, [t.id]: page })}
                  />
                )}
              </Stack>
            </Card>
          )
        })}
      {(bindings.data ?? []).some((b) => b.state === 'active') && (
        <Card withBorder>
          <Text fw={600}>Source edition mappings</Text>
          <Text size="sm" c="dimmed" mb="sm">
            Use verified listing identity to distinguish editions. A selected download version alone
            is not evidence.
          </Text>
          <Stack>
            {bindings.data
              ?.filter((b) => b.state === 'active')
              .map((b) => (
                <Group key={b.id}>
                  <Text size="sm" style={{ minWidth: 0, flex: 1 }}>
                    {b.sourceName} · {b.mangaTitle}
                  </Text>
                  <Select
                    aria-label={`Edition for ${b.mangaTitle}`}
                    placeholder="Edition not verified"
                    clearable
                    data={visible.map((c) => ({
                      value: String(c.id),
                      label: editionLabel(c),
                    }))}
                    value={b.collectionId ? String(b.collectionId) : null}
                    disabled={busy}
                    onChange={(v) =>
                      void run(() => api.setBindingEdition(seriesID, b.id, Number(v) || 0))
                    }
                    style={{ minWidth: 180, flex: 1 }}
                  />
                </Group>
              ))}
          </Stack>
        </Card>
      )}
      {files.length > 0 && (
        <Card withBorder>
          <Stack gap="sm">
            <Group justify="space-between">
              <Text fw={600}>Files and coverage</Text>
              <Button
                size="compact-xs"
                variant="subtle"
                onClick={() => {
                  setShowFiles(!showFiles)
                  setFilePage(1)
                }}
              >
                {showFiles ? 'Show review only' : 'Manage all files'}
              </Button>
            </Group>
            {showFiles && (
              <TextInput
                placeholder="Find a file"
                aria-label="Find a file"
                value={fileFilter}
                onChange={(e) => {
                  setFileFilter(e.currentTarget.value)
                  setFilePage(1)
                }}
              />
            )}
            {(() => {
              const review = (f: SeriesFile) =>
                f.needsReview ||
                f.unrequested ||
                coverage.data?.some((a) => a.fileId === f.id && !a.verified)
              const list = files
                .filter(
                  (f) =>
                    (showFiles || review(f)) &&
                    f.path.toLowerCase().includes(fileFilter.toLowerCase()),
                )
                .sort((a, b) => Number(!!review(b)) - Number(!!review(a)))
              return (
                <>
                  {list.length === 0 && (
                    <Text size="sm" c="dimmed">
                      No files need review.
                    </Text>
                  )}
                  {list.slice((filePage - 1) * 20, filePage * 20).map((f) => (
                    <Group key={f.id} justify="space-between">
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <Text size="sm" title={f.path} style={{ overflowWrap: 'anywhere' }}>
                          {f.path
                            .split(/[\\/]/)
                            .pop()
                            ?.replace(/ \[[a-f0-9]{64}\](?=\.[^.]+$)/i, '')}
                        </Text>
                        <Text size="xs" c="dimmed">
                          {review(f) ? 'Needs review' : 'Confirmed coverage'} ·{' '}
                          {f.facts?.language || 'Language unknown'}
                          {f.facts?.group ? ` · ${f.facts.group}` : ''}
                        </Text>
                      </div>
                      <Button size="xs" variant="default" onClick={() => setFile(f)}>
                        {review(f) ? 'Match file' : 'Edit mappings'}
                      </Button>
                    </Group>
                  ))}
                  {list.length > 20 && (
                    <Pagination
                      size="sm"
                      total={Math.ceil(list.length / 20)}
                      value={filePage}
                      onChange={setFilePage}
                    />
                  )}
                </>
              )
            })()}
          </Stack>
        </Card>
      )}
      {(observations.data ?? []).filter((o) => o.state === 'pending').length > 0 && (
        <Card withBorder>
          <Text fw={600}>Chapter observations</Text>
          <Text size="sm" c="dimmed" mb="sm">
            Source observations do not create units until you accept them.
          </Text>
          {observations.data
            ?.filter((o) => o.state === 'pending')
            .map((o) => (
              <Group key={o.id} justify="space-between">
                <Text size="sm">
                  {o.label} · {o.source}
                </Text>
                <Group>
                  <Button
                    size="xs"
                    disabled={busy}
                    onClick={() => void run(() => api.reviewObservation(seriesID, o.id, true))}
                  >
                    Accept unit
                  </Button>
                  <Button
                    size="xs"
                    variant="default"
                    disabled={busy}
                    onClick={() => void run(() => api.reviewObservation(seriesID, o.id, false))}
                  >
                    Dismiss
                  </Button>
                </Group>
              </Group>
            ))}
        </Card>
      )}
      {target && (
        <TargetEditor
          key={target.id}
          value={target}
          chapter={collections.find((c) => c.id === target.collectionId)?.type === 'chapter'}
          save={async (value) => {
            await api.saveTarget(seriesID, value)
            await changed()
            setTarget(null)
          }}
          close={() => setTarget(null)}
        />
      )}
      {unit && (
        <UnitEditor
          key={unit.id}
          value={unit}
          save={async (value) => {
            await api.saveUnit(seriesID, value)
            await changed()
            setUnit(null)
          }}
          reset={async () => {
            await api.resetUnit(seriesID, unit.id)
            await changed()
            setUnit(null)
          }}
          remove={async () => {
            await api.deleteUnit(seriesID, unit.id)
            await changed()
            setUnit(null)
          }}
          close={() => setUnit(null)}
        />
      )}
      {edition && (
        <EditionEditor
          value={edition}
          reset={async () => {
            await api.editEdition(seriesID, edition.id, {
              reset: [
                'edition_name',
                'language',
                'publisher',
                'medium',
                'edition_layout',
                'content_variant',
              ],
            })
            await changed()
            setEdition(null)
          }}
          save={async (patch) => {
            await api.editEdition(seriesID, edition.id, {
              ...patch,
              reset: [],
            })
            if (patch.profileId !== edition.profileId)
              await api.setCollectionProfile(seriesID, edition.id, patch.profileId || 0)
            await changed()
            setEdition(null)
          }}
          close={() => setEdition(null)}
        />
      )}
      {link && (
        <CatalogLink
          seriesID={seriesID}
          value={link}
          collections={visible}
          done={async () => {
            await changed()
            setLink(null)
          }}
          close={() => setLink(null)}
        />
      )}
      {file && (
        <MappingEditor
          value={file}
          targets={allTargets}
          existing={coverage.data?.filter((a) => a.fileId === file.id) ?? []}
          units={allUnits}
          collections={collections}
          save={async (slots) => {
            await api.mapFile(seriesID, file.id, slots)
            await changed()
            setFile(null)
          }}
          close={() => setFile(null)}
        />
      )}
      {relationships && (
        <Relationships
          seriesID={seriesID}
          units={allUnits}
          collections={collections}
          close={() => setRelationships(false)}
        />
      )}
      {confirmation}
    </Stack>
  )
}
function selection(c: Collection, t: DownloadTarget, s: UnitSlot): SlotSelection {
  return {
    ledger: c.type === 'chapter' ? 'chapter' : 'volume',
    collectionId: c.id,
    targetId: t.id,
    workId: s.work.id,
    number: s.work.sequenceNum,
    label: s.work.sequenceStr,
    collectionName: `${editionLabel(c)} · ${t.name}`,
    fileIds: s.fileIds,
    monitored: s.monitored,
    wanted: s.wanted,
  }
}
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
function TargetEditor({
  value,
  chapter,
  save,
  close,
}: {
  value: DownloadTarget
  chapter: boolean
  save: (v: DownloadTarget) => Promise<void>
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
      title={v.id ? 'Version settings' : 'Add collected version'}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void edit.run(() => save(v))
        }}
      >
        <Stack>
          <Text size="sm" c="dimmed">
            Versions share edition metadata and keep independent copies. Files are preserved when a
            version is removed.
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
                  label="Monitor chapters after"
                  value={v.cutoffNum / 100}
                  min={0}
                  decimalScale={2}
                  onChange={(n) => set({ ...v, cutoffNum: Math.round(Number(n) * 100) })}
                />
              )}
              <Checkbox
                label="Allow releases with unknown language"
                checked={v.allowUnknown}
                onChange={(e) => set({ ...v, allowUnknown: e.currentTarget.checked })}
              />
              <Checkbox
                label="Search units with unknown publication dates"
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
function UnitEditor({
  value,
  save,
  remove,
  reset,
  close,
}: {
  value: Work
  save: (v: Work) => Promise<void>
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
        title={v.id ? 'Edit unit' : 'Add unit'}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void edit.run(() => save(v))
          }}
        >
          <Stack>
            <Text size="sm" c="dimmed">
              Numbers identify units within this edition. Specials can use a label without a number.
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
                  label="Unit type"
                  data={['main', 'extra', 'other']}
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
                      title: v.providerId ? 'Retire catalog unit?' : 'Delete unit?',
                      message: 'Owned units must be unlinked first. Files stay on disk.',
                      label: v.providerId ? 'Retire unit' : 'Delete unit',
                      onConfirm: remove,
                    })
                  }
                >
                  {v.providerId ? 'Retire unit' : 'Delete unit'}
                </Button>
              )}
              <Group>
                <Button type="submit" loading={edit.busy}>
                  Save unit
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
function EditionEditor({
  value,
  save,
  reset,
  close,
}: {
  value: Collection
  save: (p: {
    profileId?: number
    editionName: string
    language: string
    publisher: string
    medium: string
    editionLayout: string
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
          <Select
            label="Edition layout"
            data={[
              { value: 'standard', label: 'Standard' },
              { value: 'omnibus', label: 'Omnibus' },
              { value: 'boxset', label: 'Box set' },
              { value: 'deluxe', label: 'Deluxe' },
              { value: 'custom', label: 'Other layout' },
            ]}
            value={v.editionLayout || 'standard'}
            onChange={(layout) => set({ ...v, editionLayout: layout ?? 'standard' })}
            disabled={edit.busy}
          />
          <Select
            label="Content version"
            data={[
              { value: 'original', label: 'Original' },
              { value: 'colored', label: 'Colored' },
            ]}
            value={v.contentVariant || 'original'}
            onChange={(variant) => set({ ...v, contentVariant: variant ?? 'original' })}
            disabled={edit.busy}
          />
          {v.type === 'chapter' && v.custom && (
            <Select
              label="Chapter numbering"
              description="Use explicit units for gaps or edition-specific numbering"
              data={[
                { value: 'manual', label: 'Explicit units only' },
                { value: 'series_count', label: 'Infer 1 to the series chapter count' },
                { value: 'catalog', label: 'Catalog units only' },
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
function CatalogLink({
  seriesID,
  value,
  collections,
  done,
  close,
}: {
  seriesID: number
  value: Collection
  collections: Collection[]
  done: () => Promise<void>
  close: () => void
}) {
  const [id, setID] = useState<string | null>(null)
  const [preview, setPreview] = useState<CatalogLinkPreview | null>(null)
  const edit = useEditor()
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
          Your edition keeps its local identity, files, and edits. Review the proposed unit matches
          before linking.
        </Text>
        <Select
          label="Catalog edition"
          data={collections
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
          onClick={() =>
            void edit.run(async () =>
              setPreview(await api.catalogLinkPreview(seriesID, value.id, Number(id))),
            )
          }
        >
          Preview matches
        </Button>
        {preview && (
          <>
            <Text size="sm">
              {Object.keys(preview.mapping).length} units matched; {preview.unmatched.length} local
              units remain unmatched.
            </Text>
            <Stack gap={4}>
              {preview.localUnits.map((w) => (
                <Text key={w.id} size="xs">
                  {w.sequenceStr} · {w.countType} →{' '}
                  {preview.mapping[w.id]
                    ? `${preview.catalogUnits.find((v) => v.id === preview.mapping[w.id])?.sequenceStr} · ${preview.catalogUnits.find((v) => v.id === preview.mapping[w.id])?.countType}`
                    : 'No catalog match; kept locally'}
                </Text>
              ))}
            </Stack>
            <Text size="xs" c="dimmed">
              Unmatched local units stay intact. The unused catalog row is retired after linking.
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
function MappingEditor({
  value,
  targets,
  existing,
  units,
  collections,
  save,
  close,
}: {
  value: SeriesFile
  targets: DownloadTarget[]
  existing: import('../types/store').AssetCoverage[]
  units: UnitSlot[]
  collections: Collection[]
  save: (slots: import('../types/store').ExpectedSlot[]) => Promise<void>
  close: () => void
}) {
  const initial = existing.map((a) => ({
    targetId: a.targetId,
    workId: a.workId,
    collectionId: targets.find((t) => t.id === a.targetId)?.collectionId ?? 0,
    number: units.find((s) => s.work.id === a.workId)?.work.sequenceNum ?? 0,
    ledger:
      collections.find((c) => c.id === targets.find((t) => t.id === a.targetId)?.collectionId)
        ?.type ?? 'volume',
  }))
  const [draft, setDraft] = useState(initial)
  const [target, setTarget] = useState<string | null>(
    existing[0] ? String(existing[0].targetId) : null,
  )
  const edit = useEditor()
  const t = targets.find((t) => String(t.id) === target)
  const chosen = draft.filter((s) => s.targetId === t?.id).map((s) => String(s.workId))
  return (
    <EditorModal
      dirty={JSON.stringify(draft) !== JSON.stringify(initial)}
      busy={edit.busy}
      onClose={close}
      title="Match imported file"
    >
      <Stack>
        <Text size="xs" style={{ overflowWrap: 'anywhere' }}>
          {value.path}
        </Text>
        <Text size="sm">
          Confirm the edition and units this archive contains. This confirmation overrides parsed
          language for coverage. Group requirements still apply. A pack can cover several units.
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
        <Select
          label="Collected version"
          data={targets.map((t) => ({
            value: String(t.id),
            label: `${editionLabel(collections.find((c) => c.id === t.collectionId)!)} · ${t.name}`,
          }))}
          value={target}
          onChange={setTarget}
        />
        <MultiSelect
          label="Units in this file"
          searchable
          disabled={!t}
          data={units
            .filter((s) => s.targetId === t?.id)
            .map((s) => ({ value: String(s.work.id), label: s.work.sequenceStr }))}
          value={chosen}
          onChange={(ids) => {
            if (!t) return
            setDraft([
              ...draft.filter((s) => s.targetId !== t.id),
              ...ids.map((id) => ({
                targetId: t.id,
                workId: Number(id),
                collectionId: t.collectionId,
                number: units.find((s) => s.work.id === Number(id))?.work.sequenceNum ?? 0,
                ledger: collections.find((c) => c.id === t.collectionId)?.type ?? 'volume',
              })),
            ])
          }}
        />
        <Stack gap={2}>
          {draft.map((s) => (
            <Text size="xs" key={`${s.targetId}-${s.workId}`}>
              {targets.find((t) => t.id === s.targetId)?.name} ·{' '}
              {units.find((u) => u.work.id === s.workId)?.work.sequenceStr}
            </Text>
          ))}
        </Stack>
        {draft.length === 0 && (
          <Text size="sm" c="dimmed">
            This file will stay in the library without claiming ownership of any unit.
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
          <Button loading={edit.busy} onClick={() => void edit.run(() => save(draft))}>
            Save coverage
          </Button>
        </Group>
      </Stack>
    </EditorModal>
  )
}

function Relationships({
  seriesID,
  units,
  collections,
  close,
}: {
  seriesID: number
  units: UnitSlot[]
  collections: Collection[]
  close: () => void
}) {
  const [from, setFrom] = useState<string | null>(null)
  const [to, setTo] = useState<string | null>(null)
  const [relation, setRelation] = useState<string | null>('contains')
  const edit = useEditor()
  const q = useQuery({
    queryKey: ['series', seriesID, 'relations'],
    queryFn: () => api.relations(seriesID),
  })
  const options = [
    ...new Map(
      units.map((s) => [
        s.work.id,
        {
          value: String(s.work.id),
          label: `${editionLabel(collections.find((c) => c.id === s.work.collectionId)!)} · ${s.work.sequenceStr}`,
        },
      ]),
    ).values(),
  ]
  return (
    <Modal opened onClose={close} title="Content relationships" size="lg">
      <Stack>
        <Text size="sm">
          Record known relationships, such as omnibus 1 containing standard volumes 1–3. A
          relationship does not mark the other edition as owned.
        </Text>
        <Select label="From unit" searchable data={options} value={from} onChange={setFrom} />
        <Select
          label="Relationship"
          data={[
            { value: 'contains', label: 'Contains' },
            { value: 'equivalent', label: 'Equivalent content' },
            { value: 'revision', label: 'Revision of' },
          ]}
          value={relation}
          onChange={setRelation}
        />
        <Select
          label="To unit"
          searchable
          data={options.filter((o) => o.value !== from)}
          value={to}
          onChange={setTo}
        />
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Button
          disabled={!from || !to || !relation}
          loading={edit.busy}
          onClick={() =>
            void edit.run(async () => {
              await api.saveRelation(seriesID, {
                fromId: Number(from),
                toId: Number(to),
                relation: relation!,
              })
              await q.refetch()
            })
          }
        >
          Add relationship
        </Button>
        {q.data?.map((r) => (
          <Group key={`${r.fromId}-${r.toId}-${r.relation}`}>
            <Text size="xs" style={{ minWidth: 0, flex: 1, overflowWrap: 'anywhere' }}>
              {options.find((o) => Number(o.value) === r.fromId)?.label} {r.relation}{' '}
              {options.find((o) => Number(o.value) === r.toId)?.label}
            </Text>
            <Button
              size="compact-xs"
              color="red"
              variant="subtle"
              disabled={edit.busy}
              onClick={() =>
                void edit.run(async () => {
                  await api.deleteRelation(seriesID, r)
                  await q.refetch()
                })
              }
            >
              Remove
            </Button>
          </Group>
        ))}
      </Stack>
    </Modal>
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
