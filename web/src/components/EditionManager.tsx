import { EditorModal } from './EditorModal'
import { DownloadSettings } from './DownloadSettings'
import { BookEditor } from './BookEditor'
import { EditionEditor } from './EditionEditor'
import { CatalogLink } from './CatalogLink'
import { CollectAgain } from './CollectAgain'
import { MappingEditor } from './MappingEditor'
import ReviewInbox from './ReviewInbox'
import EditionBooks from './EditionBooks'
import EditionActions from './EditionActions'
import { useState } from 'react'
import {
  Alert,
  Anchor,
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
import type {
  Edition,
  CollectedVersion,
  Book,
  File as SeriesFile,
  ReviewProposal,
} from '../types/store'
import type { BookSelection } from './BookDialog'
import type { EditionStart } from './AddEdition'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'
import { cardLabel, cardTitle, editionDetails, editionMedia } from './editionText'
import { EditionOrigin, MangaBakaMark } from './Origin'

// The series page shows one section per tab. They share this component
// because all three open the same book, version and file-mapping dialogs.
export type EditionSection = 'editions' | 'review' | 'files'
type Props = {
  section: EditionSection
  seriesID: number
  editions: Edition[]
  // The volume count MangaBaka gives for the series, 0 when it gives none.
  volumeCount: number
  // The series' page on MangaBaka.
  catalogUrl: string
  files: SeriesFile[]
  onBook: (s: BookSelection) => void
  onAddEdition: (start?: EditionStart) => void
  onSources: () => void
  // The series' release profile (0: the instance default).
  seriesProfileId: number
}
export default function EditionManager({
  section,
  seriesID,
  editions,
  volumeCount,
  catalogUrl,
  files,
  onBook,
  onAddEdition,
  onSources,
  seriesProfileId,
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
  const [collectAgain, setCollectAgain] = useState<Edition | null>(null)
  const [version, setVersion] = useState<CollectedVersion | null>(null)
  const [book, setBook] = useState<Book | null>(null)
  const [edition, setEdition] = useState<Edition | null>(null)
  const [link, setLink] = useState<Edition | null>(null)
  const [file, setFile] = useState<SeriesFile | null>(null)
  // Review's proposal for the file it opened; other ways in open it plain.
  const [proposal, setProposal] = useState<{ fileId: number; value: ReviewProposal } | null>(
    null,
  )
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
  const visible = [...editions].sort(
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
  const cardCount = allVersions.filter((t) => yours.some((c) => c.id === t.editionId)).length
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
          {!pending && !editions.some((c) => c.type === 'volume') && (
            <Alert
              color="blue"
              variant="light"
              icon={<MangaBakaMark />}
              title="MangaBaka lists no volumes for this series"
            >
              <Group justify="space-between" align="center" gap="sm">
                <Text size="sm" style={{ flex: 1, minWidth: 220 }}>
                  Its volumes are not in MangaBaka’s catalog yet, so there is nothing to collect
                  from.{' '}
                  {volumeCount > 0 &&
                    `MangaBaka only gives a count for the original run: ${volumeCount} volumes. `}
                  You can add the edition yourself and link it when MangaBaka lists it.{' '}
                  <Anchor href={catalogUrl} target="_blank" rel="noreferrer" size="sm">
                    See the series on MangaBaka
                  </Anchor>
                </Text>
                <Button size="xs" onClick={() => onAddEdition()}>
                  Add a volume edition
                </Button>
              </Group>
            </Alert>
          )}
          {!pending && yours.length === 0 && (
            <Text size="sm" c="dimmed">
              No edition is being collected yet. Pick one from the other editions below, or add your
              own.
            </Text>
          )}
          {!pending &&
            // One card per way of collecting an edition: the edition with
            // a release profile. Two cards of one edition are the same
            // books wanted as different releases (colored, black-and-white).
            yours.flatMap((c) => {
              const versions = allVersions.filter((t) => t.editionId === c.id)
              return versions.map((t) => {
              const stats = summary(t.id)
              const owned = stats?.owned ?? 0
              const total = stats?.total ?? 0
              const noun = (c.type === 'chapter' ? 'chapter' : 'volume') + (total === 1 ? '' : 's')
              // With one or two cards there is nothing to scan past.
              const show = expanded[t.id] ?? cardCount <= 2
              const title = cardTitle(c, t, allVersions)
              const missing = stats?.wanted ?? 0
              const findMissing = () =>
                t &&
                onBook({
                  ledger: c.type === 'chapter' ? 'chapter' : 'volume',
                  editionId: c.id,
                  versionId: t.id,
                  all: true,
                  number: 0,
                  editionLabel: cardLabel(c, t, allVersions),
                  monitored: t.monitored,
                  wanted: true,
                  fileIds: [],
                  autoSearch: true,
                })
              return (
                <Card key={t.id} padding="md" withBorder>
                  <Stack gap="sm">
                    <Group justify="space-between" align="flex-start" wrap="nowrap" gap="sm">
                      <div style={{ minWidth: 0 }}>
                        <Group gap="xs">
                          <Text fw={600} style={{ overflowWrap: 'anywhere' }}>
                            {title}
                          </Text>
                          <EditionOrigin edition={c} href={catalogUrl} />
                        </Group>
                        <Text size="xs" c="dimmed" mt={2}>
                          {/* A profile chosen here and not in the title. */}
                          {editionDetails(c, versions.length === 1 && t.profileName)}
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
                        {t && missing > 0 && (
                          <Button variant="default" size="xs" onClick={findMissing}>
                            Search {missing} missing
                          </Button>
                        )}
                        <EditionActions
                          seriesID={seriesID}
                          edition={c}
                          title={title}
                          others={visible.filter((o) => o.id !== c.id)}
                          run={async (fn) => {
                            await run(fn)
                          }}
                          more={
                            <>
                              <Menu.Item onClick={() => setCollectAgain(c)}>
                                Collect again with another profile…
                              </Menu.Item>
                              {versions.length > 1 && (
                                <Menu.Item
                                  color="red"
                                  onClick={() =>
                                    confirm({
                                      title: `Stop collecting ${title} this way?`,
                                      message:
                                        'The card goes; files stay on disk, and the edition’s other cards keep theirs.',
                                      label: 'Remove card',
                                      onConfirm: async () => {
                                        await api.deleteVersion(seriesID, t.id)
                                        await changed()
                                      },
                                    })
                                  }
                                >
                                  Remove this card
                                </Menu.Item>
                              )}
                              {c.custom && (
                                <Menu.Item onClick={() => setLink(c)}>Link to MangaBaka</Menu.Item>
                              )}
                            </>
                          }
                        >
                          <Menu.Item onClick={findMissing}>Search releases</Menu.Item>
                          <Menu.Item onClick={() => setEdition(c)}>Edit edition</Menu.Item>
                          <Menu.Item onClick={() => setVersion(t)}>Download settings</Menu.Item>
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
                                overrides: [],
                                copies: 0,
                              })
                            }
                          >
                            Add {c.type === 'chapter' ? 'chapter' : 'volume'}
                          </Menu.Item>
                        </EditionActions>
                      </Group>
                    </Group>
                    {(
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
                              onClick={() => setExpanded({ ...expanded, [t.id]: !show })}
                            >
                              {total} {noun} · {owned} owned
                            </Button>
                          ) : (
                            <Text size="sm" c="dimmed">
                              No {noun} yet
                            </Text>
                          )}
                        </Group>
                        <Switch
                          size="sm"
                          label="Collect"
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
                    {show && (
                      <EditionBooks
                        key={t.id}
                        seriesID={seriesID}
                        edition={c}
                        version={t}
                        label={cardLabel(c, t, allVersions)}
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
              })
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
            <Button size="xs" variant="default" onClick={() => onAddEdition()}>
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
                  placeholder="All media"
                  clearable
                  value={layout}
                  onChange={setLayout}
                  data={[...new Set(others.flatMap(editionMedia))]}
                />
              </Group>
              {others
                .filter(
                  (c) =>
                    (!language || c.language === language) &&
                    (!layout || editionMedia(c).includes(layout)),
                )
                .map((c) => (
                  <Card key={c.id} withBorder padding="sm">
                    <Group justify="space-between">
                      <div>
                        <Text fw={500}>{c.editionName}</Text>
                        <Text size="xs" c="dimmed">
                          {editionDetails(c)}
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
          onSources={onSources}
          onFile={(id, proposal) => {
            const f = files.find((f) => f.id === id)
            if (f) {
              setFile(f)
              setProposal(proposal ? { fileId: id, value: proposal } : null)
            }
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
        <DownloadSettings
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
          seriesID={seriesID}
          edition={editions.find((c) => c.id === book.editionId)}
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
              reset: ['edition_name', 'language', 'publisher', 'medium'],
            })
            await changed()
            setEdition(null)
          }}
          save={async (patch) => {
            await api.editEdition(seriesID, edition.id, {
              ...patch,
              reset: [],
            })
            await changed()
            setEdition(null)
          }}
          close={() => setEdition(null)}
        />
      )}
      {collectAgain && (
        <CollectAgain
          seriesID={seriesID}
          edition={collectAgain}
          versions={allVersions.filter((t) => t.editionId === collectAgain.id)}
          seriesProfileId={seriesProfileId}
          save={async () => {
            await changed()
            setCollectAgain(null)
          }}
          close={() => setCollectAgain(null)}
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
          proposal={proposal?.fileId === file.id ? proposal.value : undefined}
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
