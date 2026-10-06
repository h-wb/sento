import {
  EditorModal,
  TargetEditor,
  UnitEditor,
  EditionEditor,
  CatalogLink,
  MappingEditor,
  Relationships,
} from './EditionDialogs'
import ReviewInbox from './ReviewInbox'
import EditionBooks from './EditionBooks'
import EditionActions from './EditionActions'
import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  Group,
  Menu,
  Pagination,
  Select,
  Stack,
  Switch,
  SegmentedControl,
  Text,
  TextInput,
} from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Collection, DownloadTarget, Work, File as SeriesFile } from '../types/store'
import type { SlotSelection } from './SlotModal'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'

const editionLabel = (c: Collection) =>
  [c.language, c.editionName, c.publisher, c.medium].filter(Boolean).join(' · ')
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
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const targets = useQuery({
    queryKey: ['series', seriesID, 'targets'],
    queryFn: () => api.targets(seriesID),
  })
  const slots = useQuery({
    queryKey: ['series', seriesID, 'edition-overview'],
    queryFn: () => api.editionOverview(seriesID),
  })
  const bindings = useQuery({
    queryKey: ['series', seriesID, 'bindings'],
    queryFn: () => api.bindings(seriesID),
  })
  const coverage = useQuery({
    queryKey: ['series', seriesID, 'coverage'],
    queryFn: () => api.coverage(seriesID),
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
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['series', seriesID] }),
      qc.invalidateQueries({ queryKey: ['review'] }),
    ])
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
  const details = useQuery({
    queryKey: ['series', seriesID, 'target-slots'],
    queryFn: () => api.targetSlots(seriesID),
    enabled: !!file || relationships,
  })
  const allUnits = Object.values(details.data ?? {}).flat()
  const [otherEditions, setOtherEditions] = useState(false)
  const [language, setLanguage] = useState<string | null>(null)
  const [layout, setLayout] = useState<string | null>(null)
  const [section, setSection] = useState(
    new URLSearchParams(window.location.search).get('view') === 'review' ? 'review' : 'editions',
  )
  const summary = (id: number) => slots.data?.targets.find((t) => t.targetId === id)
  const isYours = (c: Collection) =>
    (c.custom && c.numberingSource === 'manual') ||
    allTargets.some(
      (t) => t.collectionId === c.id && (t.monitored || (summary(t.id)?.onDisk ?? 0) > 0),
    )
  const others = visible.filter((c) => !isYours(c))

  return (
    <Stack gap="lg">
      <SegmentedControl
        value={section}
        onChange={setSection}
        data={[
          { value: 'editions', label: 'Your editions' },
          { value: 'review', label: 'Review' },
          { value: 'settings', label: 'Advanced' },
        ]}
      />
      {section === 'editions' && (
        <>
          <Group justify="space-between">
            <Text size="sm" c="dimmed">
              Choose the editions you want to collect.
            </Text>
            <Group>
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
            visible.filter(isYours).map((c) => {
              const versions = allTargets.filter((t) => t.collectionId === c.id)
              const t =
                versions.find((t) => String(t.id) === chosen[c.id]) ??
                versions.find((t) => t.isDefault) ??
                versions[0]
              const stats = t ? summary(t.id) : undefined
              const owned = stats?.owned ?? 0
              const total = stats?.total ?? 0
              const show = expanded[c.id] ?? false
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
                            {owned}/{total} owned
                          </Text>
                        </Group>
                      </div>
                      <Group gap="xs">
                        {c.syncState === 'stale' && (
                          <Button
                            size="compact-xs"
                            variant="default"
                            onClick={() =>
                              confirm({
                                title: 'Continue as a local edition?',
                                message:
                                  'Keep these books and files and manage their metadata yourself. Future catalog refreshes will not update this edition.',
                                label: 'Continue locally',
                                onConfirm: async () => {
                                  await api.editEdition(seriesID, c.id, {
                                    makeLocal: true,
                                    reset: [],
                                  })
                                  await changed()
                                },
                              })
                            }
                          >
                            Continue locally
                          </Button>
                        )}
                        <EditionActions
                          seriesID={seriesID}
                          collection={c}
                          others={visible.filter((o) => o.id !== c.id)}
                          run={async (fn) => {
                            await run(fn)
                          }}
                        >
                          <Menu.Item onClick={() => setEdition(c)}>Edit edition</Menu.Item>
                          {t && (
                            <Menu.Item
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
                              Keep an additional copy
                            </Menu.Item>
                          )}

                          <Menu.Item
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
                            Add {c.type === 'chapter' ? 'chapter' : 'volume'}
                          </Menu.Item>
                          {t && !t.isDefault && (
                            <Menu.Item
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
                            </Menu.Item>
                          )}

                          {c.custom && (
                            <Menu.Item onClick={() => setLink(c)}>Link to catalog</Menu.Item>
                          )}
                        </EditionActions>
                      </Group>
                    </Group>
                    {t && (
                      <Group align="flex-end">
                        {versions.length > 1 && (
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
                        )}
                        <Text size="xs" c="dimmed">
                          {stats?.profileName || 'Default profile'} ·{' '}
                          {stats?.profileSource === 'this version'
                            ? 'custom profile'
                            : `inherited from ${stats?.profileSource ?? 'series'}`}
                        </Text>
                        <Switch
                          label={versions.length > 1 ? 'Collect this version' : 'Collect'}
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
                          Collection settings
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
                    {total > 0 && (
                      <Button
                        variant="subtle"
                        size="compact-xs"
                        aria-expanded={show}
                        onClick={() => setExpanded({ ...expanded, [c.id]: !show })}
                      >
                        {show ? 'Hide' : 'Show'} {c.type === 'chapter' ? 'chapters' : 'volumes'} (
                        {total})
                      </Button>
                    )}
                    {show && t && (
                      <EditionBooks
                        key={t.id}
                        seriesID={seriesID}
                        edition={c}
                        target={t}
                        files={files}
                        coverage={coverage.data ?? []}
                        onSlot={onSlot}
                        onEdit={setUnit}
                        onFile={setFile}
                        run={run}
                        busy={busy}
                      />
                    )}
                  </Stack>
                </Card>
              )
            })}
          <Button
            variant="subtle"
            onClick={() => setOtherEditions(!otherEditions)}
            aria-expanded={otherEditions}
          >
            Other editions ({others.length})
          </Button>
          {otherEditions && (
            <Stack gap="sm">
              <Group>
                <Select
                  placeholder="All languages"
                  clearable
                  value={language}
                  onChange={setLanguage}
                  data={[...new Set(others.map((c) => c.language))]}
                />
                <Select
                  placeholder="All formats"
                  clearable
                  value={layout}
                  onChange={setLayout}
                  data={[...new Set(others.flatMap((c) => c.editionTraits).filter(Boolean))]}
                />
              </Group>
              {others
                .filter(
                  (c) =>
                    (!language || c.language === language) &&
                    (!layout || c.editionTraits.includes(layout)),
                )
                .map((c) => (
                  <Card key={c.id} withBorder padding="sm">
                    <Group justify="space-between">
                      <div>
                        <Text fw={500}>{c.editionName}</Text>
                        <Text size="xs" c="dimmed">
                          {[c.language, c.publisher, c.medium].filter(Boolean).join(' · ')}
                        </Text>
                      </div>
                      <Button
                        size="xs"
                        variant="default"
                        disabled={busy}
                        onClick={() => {
                          const t = allTargets.find((t) => t.collectionId === c.id && t.isDefault)
                          if (t) void run(() => api.saveTarget(seriesID, { ...t, monitored: true }))
                        }}
                      >
                        Collect edition
                      </Button>
                    </Group>
                  </Card>
                ))}
            </Stack>
          )}
        </>
      )}
      {section === 'settings' && (
        <>
          <Button size="xs" variant="default" onClick={() => setRelationships(true)}>
            Content relationships
          </Button>
          {(bindings.data ?? []).some((b) => b.state === 'active') && (
            <Card withBorder>
              <Text fw={600}>Source edition mappings</Text>
              <Text size="sm" c="dimmed" mb="sm">
                Use verified listing identity to distinguish editions. A selected download version
                alone is not evidence.
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
                      {collections.find((c) => c.id === b.collectionId)?.type === 'chapter' && (
                        <Checkbox
                          label="Trust new chapters from this listing"
                          description="Add observed chapters automatically when chapter discovery is enabled for this edition."
                          checked={b.trustChapters}
                          onChange={(e) =>
                            void run(() =>
                              api.trustBinding(seriesID, b.id, e.currentTarget.checked),
                            )
                          }
                        />
                      )}
                    </Group>
                  ))}
              </Stack>
            </Card>
          )}
        </>
      )}
      {section === 'review' && (
        <>
          <ReviewInbox
            seriesID={seriesID}
            onFile={(id) => {
              const f = files.find((f) => f.id === id)
              if (f) setFile(f)
            }}
          />
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
        </>
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
                'edition_traits',
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
      {file && (!details.data || !coverage.data) && (
        <EditorModal
          title="Match imported file"
          dirty={false}
          busy={false}
          onClose={() => setFile(null)}
        >
          <QueryFeedback
            pending={details.isPending || coverage.isPending}
            error={details.error || coverage.error}
            onRetry={() => {
              void details.refetch()
              void coverage.refetch()
            }}
          />
        </EditorModal>
      )}
      {file && details.data && coverage.data && (
        <MappingEditor
          seriesID={seriesID}
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
