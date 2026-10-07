import {
  EditorModal,
  VersionEditor,
  BookEditor,
  EditionEditor,
  CatalogLink,
  MappingEditor,
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
  Group,
  Menu,
  Pagination,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, fmtSize } from '../api'
import type { Edition, CollectedVersion, Book, File as SeriesFile } from '../types/store'
import type { BookSelection } from './BookDialog'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'

const editionLabel = (c: Edition) =>
  [c.language, c.editionName, c.publisher, c.medium].filter(Boolean).join(' · ')
// The series page shows one section per tab. They share this component
// because all three open the same book, version and file-mapping dialogs.
export type EditionSection = 'editions' | 'review' | 'files'
type Props = {
  section: EditionSection
  seriesID: number
  editions: Edition[]
  files: SeriesFile[]
  onBook: (s: BookSelection) => void
  onAddEdition: () => void
}
export default function EditionManager({
  section,
  seriesID,
  editions,
  files,
  onBook,
  onAddEdition,
}: Props) {
  const qc = useQueryClient()
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const versions = useQuery({
    queryKey: ['series', seriesID, 'versions'],
    queryFn: () => api.versions(seriesID),
  })
  const overview = useQuery({
    queryKey: ['series', seriesID, 'edition-overview'],
    queryFn: () => api.editionOverview(seriesID),
  })
  const coverage = useQuery({
    queryKey: ['series', seriesID, 'coverage'],
    queryFn: () => api.coverage(seriesID),
  })
  const [chosen, setChosen] = useState<Record<number, string>>({})
  const [version, setVersion] = useState<CollectedVersion | null>(null)
  const [book, setBook] = useState<Book | null>(null)
  const [edition, setEdition] = useState<Edition | null>(null)
  const [link, setLink] = useState<Edition | null>(null)
  const [file, setFile] = useState<SeriesFile | null>(null)
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
  const allVersions = versions.data ?? []
  const visible = editions
    .filter((c) => c.syncState !== 'superseded')
    .sort(
      (a, b) =>
        Number(b.monitored) - Number(a.monitored) ||
        Number(a.type === 'chapter') - Number(b.type === 'chapter') ||
        a.editionName.localeCompare(b.editionName),
    )
  const pending = versions.isPending || overview.isPending
  const details = useQuery({
    queryKey: ['series', seriesID, 'version-books'],
    queryFn: () => api.versionBooks(seriesID),
    enabled: !!file,
  })
  const allBooks = Object.values(details.data ?? {}).flat()
  const [otherEditions, setOtherEditions] = useState(false)
  const [language, setLanguage] = useState<string | null>(null)
  const [layout, setLayout] = useState<string | null>(null)
  const summary = (id: number) => overview.data?.versions.find((t) => t.versionId === id)
  const isYours = (c: Edition) =>
    (c.custom && c.numberingSource === 'manual') ||
    allVersions.some(
      (t) => t.editionId === c.id && (t.monitored || (summary(t.id)?.onDisk ?? 0) > 0),
    )
  const yours = visible.filter(isYours)
  const others = visible.filter((c) => !isYours(c))

  return (
    <Stack gap="lg">
      {section === 'editions' && (
        <>
          <QueryFeedback
            pending={pending}
            error={versions.error || overview.error}
            onRetry={() => {
              void versions.refetch()
              void overview.refetch()
            }}
          />
          {error && <Alert color="red">{error}</Alert>}
          {!pending && yours.length === 0 && (
            <Text size="sm" c="dimmed">
              No edition is being collected yet. Pick one from the other editions below, or add your
              own.
            </Text>
          )}
          {!pending &&
            yours.map((c) => {
              const versions = allVersions.filter((t) => t.editionId === c.id)
              const t =
                versions.find((t) => String(t.id) === chosen[c.id]) ??
                versions.find((t) => t.isDefault) ??
                versions[0]
              const stats = t ? summary(t.id) : undefined
              const owned = stats?.owned ?? 0
              const total = stats?.total ?? 0
              const noun = c.type === 'chapter' ? 'chapters' : 'volumes'
              // With one or two editions there is nothing to scan past.
              const show = expanded[c.id] ?? yours.length <= 2
              const profile = `${stats?.profileName || 'Default profile'} · ${
                stats?.profileSource === 'this version'
                  ? 'custom profile'
                  : `inherited from ${stats?.profileSource ?? 'series'}`
              }`
              const findMissing = () =>
                t &&
                onBook({
                  ledger: c.type === 'chapter' ? 'chapter' : 'volume',
                  editionId: c.id,
                  versionId: t.id,
                  all: true,
                  number: 0,
                  editionLabel: `${editionLabel(c)} · ${t.name}`,
                  monitored: t.monitored,
                  wanted: true,
                  fileIds: [],
                  autoSearch: true,
                })
              return (
                <Card key={c.id} padding="md" withBorder>
                  <Stack gap="sm">
                    <Group justify="space-between" align="flex-start" wrap="nowrap" gap="sm">
                      <div style={{ minWidth: 0 }}>
                        <Group gap="xs">
                          <Text fw={600} style={{ overflowWrap: 'anywhere' }}>
                            {c.editionName}
                          </Text>
                          {(c.custom || c.syncState === 'stale') && (
                            <Badge size="xs" color={c.syncState === 'stale' ? 'orange' : 'gray'}>
                              {c.syncState === 'stale' ? 'Catalog entry missing' : 'Local edition'}
                            </Badge>
                          )}
                        </Group>
                        <Text size="xs" c="dimmed" mt={2}>
                          {[c.language, c.publisher, c.medium, t && profile]
                            .filter(Boolean)
                            .join(' · ')}
                        </Text>
                      </div>
                      <Group gap="xs" wrap="nowrap">
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
                        {t && (
                          <Button
                            variant="default"
                            size="xs"
                            visibleFrom="sm"
                            onClick={() => setVersion(t)}
                          >
                            Version settings
                          </Button>
                        )}
                        {t && (
                          <Button
                            variant="default"
                            size="xs"
                            visibleFrom="sm"
                            onClick={findMissing}
                          >
                            Search releases
                          </Button>
                        )}
                        <EditionActions
                          seriesID={seriesID}
                          edition={c}
                          others={visible.filter((o) => o.id !== c.id)}
                          run={async (fn) => {
                            await run(fn)
                          }}
                        >
                          {t && (
                            <Menu.Item hiddenFrom="sm" onClick={() => setVersion(t)}>
                              Version settings
                            </Menu.Item>
                          )}
                          {t && (
                            <Menu.Item hiddenFrom="sm" onClick={findMissing}>
                              Search releases
                            </Menu.Item>
                          )}
                          <Menu.Item onClick={() => setEdition(c)}>Edit edition</Menu.Item>
                          {t && (
                            <Menu.Item
                              onClick={() =>
                                setVersion({
                                  ...t,
                                  id: 0,
                                  name: '',
                                  isDefault: false,
                                  requiredGroup: '',
                                })
                              }
                            >
                              Keep an additional version
                            </Menu.Item>
                          )}

                          <Menu.Item
                            onClick={() =>
                              setBook({
                                id: 0,
                                editionId: c.id,
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
                                    await api.deleteVersion(seriesID, t.id)
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
                      <Group justify="space-between" gap="sm">
                        <Group gap="sm">
                          {total > 0 ? (
                            <Button
                              variant="subtle"
                              size="compact-sm"
                              px={4}
                              leftSection={
                                show ? (
                                  <IconChevronDown size={14} />
                                ) : (
                                  <IconChevronRight size={14} />
                                )
                              }
                              aria-expanded={show}
                              aria-label={`${show ? 'Hide' : 'Show'} ${noun} (${total})`}
                              onClick={() => setExpanded({ ...expanded, [c.id]: !show })}
                            >
                              {total} {noun} · {owned} owned
                            </Button>
                          ) : (
                            <Text size="sm" c="dimmed">
                              No {noun} yet
                            </Text>
                          )}
                          {versions.length > 1 && (
                            <Select
                              aria-label="Collected version"
                              size="xs"
                              data={versions.map((v) => ({
                                value: String(v.id),
                                label: v.name,
                              }))}
                              value={String(t.id)}
                              onChange={(v) => {
                                if (v) setChosen({ ...chosen, [c.id]: v })
                              }}
                              w={190}
                            />
                          )}
                        </Group>
                        <Switch
                          size="sm"
                          label={versions.length > 1 ? 'Collect this version' : 'Collect'}
                          checked={t.monitored}
                          disabled={busy}
                          onChange={(e) =>
                            void run(() =>
                              api.saveVersion(seriesID, {
                                ...t,
                                monitored: e.currentTarget.checked,
                              }),
                            )
                          }
                        />
                      </Group>
                    )}
                    {show && t && (
                      <EditionBooks
                        key={t.id}
                        seriesID={seriesID}
                        edition={c}
                        version={t}
                        files={files}
                        coverage={coverage.data ?? []}
                        onBook={onBook}
                        onEdit={setBook}
                        onFile={setFile}
                        run={run}
                        busy={busy}
                      />
                    )}
                  </Stack>
                </Card>
              )
            })}
          <Group justify="space-between">
            <Button
              variant="subtle"
              size="compact-sm"
              leftSection={
                otherEditions ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />
              }
              onClick={() => setOtherEditions(!otherEditions)}
              aria-expanded={otherEditions}
            >
              Other editions ({others.length})
            </Button>
            <Button size="xs" variant="default" onClick={onAddEdition}>
              Add edition
            </Button>
          </Group>
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
                          const t = allVersions.find((t) => t.editionId === c.id && t.isDefault)
                          if (t) void run(() => api.saveVersion(seriesID, { ...t, monitored: true }))
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
      {section === 'review' && (
        <ReviewInbox
          seriesID={seriesID}
          onFile={(id) => {
            const f = files.find((f) => f.id === id)
            if (f) setFile(f)
          }}
        />
      )}
      {section === 'files' && files.length > 0 && (
        <Stack gap="sm">
          {error && <Alert color="red">{error}</Alert>}
          <TextInput
            placeholder="Find a file"
            aria-label="Find a file"
            size="xs"
            maw={320}
            value={fileFilter}
            onChange={(e) => {
              setFileFilter(e.currentTarget.value)
              setFilePage(1)
            }}
          />
          {(() => {
            const review = (f: SeriesFile) =>
              f.needsReview ||
              f.unrequested ||
              coverage.data?.some((a) => a.fileId === f.id && !a.verified)
            const list = files
              .filter((f) => f.path.toLowerCase().includes(fileFilter.toLowerCase()))
              .sort((a, b) => Number(!!review(b)) - Number(!!review(a)))
            return (
              <>
                {list.length === 0 && (
                  <Text size="sm" c="dimmed">
                    No files match.
                  </Text>
                )}
                {list.slice((filePage - 1) * 20, filePage * 20).map((f) => (
                  <Group key={f.id} justify="space-between" wrap="nowrap">
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <Text size="sm" title={f.path} style={{ overflowWrap: 'anywhere' }}>
                        {f.path
                          .split(/[\\/]/)
                          .pop()
                          ?.replace(/ \[[a-f0-9]{64}\](?=\.[^.]+$)/i, '')}
                      </Text>
                      <Text size="xs" c={review(f) ? 'yellow' : 'dimmed'}>
                        {[
                          review(f) ? 'Needs review' : 'Confirmed coverage',
                          f.facts?.language || 'Language unknown',
                          f.facts?.group,
                          f.provenance,
                          fmtSize(f.size),
                        ]
                          .filter(Boolean)
                          .join(' · ')}
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
      )}
      {version && (
        <VersionEditor
          key={version.id}
          value={version}
          chapter={editions.find((c) => c.id === version.editionId)?.type === 'chapter'}
          save={async (value) => {
            await api.saveVersion(seriesID, value)
            await changed()
            setVersion(null)
          }}
          close={() => setVersion(null)}
        />
      )}
      {book && (
        <BookEditor
          key={book.id}
          value={book}
          save={async (value) => {
            await api.saveBook(seriesID, value)
            await changed()
            setBook(null)
          }}
          reset={async () => {
            await api.resetBook(seriesID, book.id)
            await changed()
            setBook(null)
          }}
          remove={async () => {
            await api.deleteBook(seriesID, book.id)
            await changed()
            setBook(null)
          }}
          close={() => setBook(null)}
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
              await api.setEditionProfile(seriesID, edition.id, patch.profileId || 0)
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
          editions={visible}
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
          versions={allVersions}
          existing={coverage.data?.filter((a) => a.fileId === file.id) ?? []}
          books={allBooks}
          editions={editions}
          save={async (books) => {
            await api.mapFile(seriesID, file.id, books)
            await changed()
            setFile(null)
          }}
          close={() => setFile(null)}
        />
      )}
      {confirmation}
    </Stack>
  )
}
