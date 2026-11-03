import { useEffect, useRef, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Loader,
  Modal,
  SegmentedControl,
  Stack,
  Tabs,
  Text,
} from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { useQueryClient } from '@tanstack/react-query'
import { IconFile, IconListSearch, IconRefresh, IconSearch, IconTrash } from '@tabler/icons-react'
import { api, fmtNum, fmtSize, type ReleaseCandidate, type SeriesFile } from '../api'
import { notify } from '../ui'
import CandidateList from './CandidateList'
import AcquisitionPlan from './AcquisitionPlan'
import type { EditionStart } from './AddEdition'
import type { AcquisitionPlan as Plan } from '../types/service'
import { useConfirmAction } from './ConfirmAction'

export type BookSelection = {
  // A download this search looks to replace: it keeps running until a
  // release is picked here.
  replaceQueueId?: number
  replaceTitle?: string
  // The collected version being searched; bookId narrows it to one book,
  // all widens it to every wanted book.
  versionId: number
  bookId?: number
  all?: boolean
  label?: string
  ledger: 'volume' | 'chapter'
  number: number // hundredths
  editionId?: number
  editionLabel?: string
  releaseDate?: string
  monitored: boolean
  wanted: boolean
  fileIds: number[]
  // Fire the streaming search as soon as the modal opens (the
  // interactive-search buttons; a plain row click opens quietly).
  autoSearch?: boolean
}

// Accumulated state of one streaming search: batches land per provider.
type StreamResult = {
  plan?: Plan
  queries: string[]
  searched: string[]
  pending: string[] // "provider/site" entries still searching
  candidates: ReleaseCandidate[]
  dropped: number
  notes: string[]
  errors: string[]
  done: boolean
  stopped?: boolean
}

// "host/Japscan (fr): binding 9 (Japscan (fr)): host: Cloudflare…" reads
// as "Japscan (fr): Cloudflare…": the routing detail is for logs.
function tidyProblem(message: string): string {
  return message
    .replace(/^(host|torrent)\//, '')
    .split(': ')
    .filter((part) => part !== 'host' && !/^binding \d+ \(/.test(part))
    .join(': ')
}

export default function BookDialog({
  seriesID,
  sel,
  files,
  onClose,
  onChanged,
  onAddEdition,
}: {
  seriesID: number
  sel: BookSelection | null
  files: SeriesFile[]
  onClose: () => void
  onChanged: () => void
  // Releases are an edition the series lacks: add it, then search it.
  onAddEdition?: (start: EditionStart) => void
}) {
  if (!sel) return null
  // Keyed by book so every open starts fresh (tab choice, search state).
  return (
    <Inner
      key={`${sel.versionId}-${sel.bookId ?? 'all'}`}
      seriesID={seriesID}
      sel={sel}
      files={files}
      onClose={onClose}
      onChanged={onChanged}
      onAddEdition={onAddEdition}
    />
  )
}

function Inner({
  seriesID,
  sel,
  files,
  onClose,
  onChanged,
  onAddEdition,
}: {
  seriesID: number
  sel: BookSelection
  files: SeriesFile[]
  onClose: () => void
  onChanged: () => void
  onAddEdition?: (start: EditionStart) => void
}) {
  // Without a book, the search covers the whole collected version: the
  // place to grab packs and complete releases.
  const editionWide = sel.all ?? !sel.bookId
  const linked = files.filter((f) => sel.fileIds.includes(f.id))

  const { confirm, confirmation } = useConfirmAction()
  const qc = useQueryClient()
  const searchController = useRef<AbortController | null>(null)
  useEffect(() => () => searchController.current?.abort(), [])
  const [busy, setBusy] = useState(false)
  const [searching, setSearching] = useState(false)
  const [result, setResult] = useState<StreamResult | null>(null)
  // Edition-wide searches cover the wanted books unless widened to all.
  const [allBooks, setAllBooks] = useState(false)
  // Search buttons land on Search; a quiet row click lands on Files when
  // there's a file to look at.
  const [tab, setTab] = useState<string | null>(
    sel.autoSearch || editionWide || linked.length === 0 ? 'search' : 'files',
  )
  const small = useMediaQuery('(max-width: 48em)')

  // Streaming search: each provider's batch renders as soon as it lands,
  // so fast extension sources show up while torrent indexers still load.
  const searchScope = (everyBook: boolean) => ({
    replaceQueueId: sel.replaceQueueId,
    versionId: sel.versionId,
    bookId: sel.bookId,
    all: sel.all,
    allBooks: editionWide && everyBook ? true : undefined,
  })
  const interactiveSearch = async (everyBook = allBooks) => {
    if (searching || busy) return
    const controller = new AbortController()
    searchController.current = controller
    setSearching(true)
    setResult({
      queries: [],
      searched: [],
      pending: [],
      candidates: [],
      dropped: 0,
      notes: [],
      errors: [],
      done: false,
    })
    try {
      await api.searchStream(
        seriesID,
        { ledger: sel.ledger, scope: searchScope(everyBook) },
        (ev) => {
          setResult((prev) => {
            if (!prev) return prev
            switch (ev.type) {
              case 'sites':
                return {
                  ...prev,
                  notes: ev.bindingNotes ?? [],
                  searched: ev.searched ?? [],
                  pending: ev.searched ?? [],
                }
              case 'batch':
                return {
                  ...prev,
                  queries: [...new Set([...prev.queries, ...(ev.queries ?? [])])],
                  pending: prev.pending.filter((p) => !(ev.sites ?? []).includes(p)),
                  candidates: [...prev.candidates, ...(ev.candidates ?? [])],
                  dropped: prev.dropped + (ev.dropped ?? 0),
                  notes: [...prev.notes, ...(ev.bindingNotes ?? [])],
                  errors: ev.error ? [...prev.errors, ev.error] : prev.errors,
                  plan: ev.plan ?? prev.plan,
                }
              case 'done':
                return { ...prev, pending: [], done: true, plan: ev.plan ?? prev.plan }
              case 'error':
                return {
                  ...prev,
                  pending: [],
                  done: true,
                  errors: [...prev.errors, ev.error ?? 'search failed'],
                }
              default:
                return prev
            }
          })
        },
        controller.signal,
      )
    } catch (e) {
      if (controller.signal.aborted) setResult((prev) => (prev ? { ...prev, stopped: true } : prev))
      if (!controller.signal.aborted)
        setResult((prev) =>
          prev
            ? {
                ...prev,
                errors: [...prev.errors, e instanceof Error ? e.message : String(e)],
              }
            : prev,
        )
    } finally {
      setSearching(false)
      setResult((prev) => (prev ? { ...prev, pending: [], done: true } : prev))
    }
  }

  // The interactive-search buttons open the modal already searching.
  // Deferred so the modal paints first (and StrictMode's double-invoke
  // cancels via cleanup).
  useEffect(() => {
    if (!sel.autoSearch) return
    const id = setTimeout(() => void interactiveSearch(), 0)
    return () => clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Source failures are secondary to the results: one line, opened on
  // demand, or straight away when they are all there is to show. Notes
  // ("no results", "no confident match") are answers, not failures.
  const problems = [...new Set(result?.errors ?? [])].map(tidyProblem)
  const notes = [...new Set(result?.notes ?? [])].map(tidyProblem)
  // Releases ruled out for being an edition the series has none of, by
  // edition: the one thing to do about them is add that edition.
  const byEdition = new Map<
    string,
    { name: string; language: string; volumes: number; titles: Set<string> }
  >()
  for (const c of result?.candidates ?? []) {
    const o = c.otherEdition
    if (!o || o.inLibrary) continue
    const key = `${o.name}/${o.language}`
    const group = byEdition.get(key) ?? { name: o.name, language: o.language, volumes: 0, titles: new Set() }
    group.titles.add(c.title)
    group.volumes = Math.max(group.volumes, o.volumes)
    byEdition.set(key, group)
  }
  const missing = [...byEdition.values()].map((m) => ({ ...m, releases: m.titles.size }))
  const [problemsOpen, setProblemsOpen] = useState<boolean | null>(null)
  const showProblems =
    problems.length > 0 && (problemsOpen ?? (!!result?.done && result.candidates.length === 0))

  const label = editionWide
    ? `${sel.editionLabel ?? 'Edition'} — ${allBooks ? 'all' : 'wanted'} ${sel.ledger}s`
    : `${sel.ledger === 'volume' ? 'Volume' : 'Chapter'} ${sel.label ?? fmtNum(sel.number)}`

  const grab = async (c: ReleaseCandidate, force: boolean) => {
    setBusy(true)
    try {
      await api.search(seriesID, {
        ledger: sel.ledger,
        grab: c.guid,
        grabProvider: c.provider,
        grabSiteId: c.siteId,
        grabVersionId: c.versionId,
        force,
        scope: searchScope(allBooks),
      })
      onChanged()
      notify.ok('Release queued for download')
    } finally {
      setBusy(false)
    }
  }

  const unblock = async (c: ReleaseCandidate) => {
    setBusy(true)
    try {
      await api.unblockReleases({ protocol: c.protocol, keys: [...new Set([c.externalKey, c.guid].filter(Boolean))] })
      void qc.invalidateQueries({ queryKey: ['blocklist'] })
      notify.ok('Release unblocked. Checking releases again…')
    } finally { setBusy(false) }
    await interactiveSearch()
  }

  return (
    <Modal
      opened
      onClose={onClose}
      title={label}
      size="min(1100px, 96vw)"
      fullScreen={small}
      closeOnEscape={!busy}
      closeOnClickOutside={!busy}
      withCloseButton={!busy}
    >
      <Stack gap="sm">
        {editionWide ? (
          <Group justify="space-between" gap="xs">
            <Text size="sm" c="dimmed">
              {allBooks
                ? `Releases for every ${sel.ledger} of this edition, including ones you already have or paused.`
                : `Releases for the ${sel.ledger}s this edition still wants.`}{' '}
              Packs can include several {sel.ledger}s.
            </Text>
            <SegmentedControl
              size="xs"
              aria-label="Books to search for"
              value={allBooks ? 'all' : 'wanted'}
              disabled={searching || busy}
              onChange={(v) => {
                setAllBooks(v === 'all')
                // The results on screen answer the other question: search again.
                if (result) void interactiveSearch(v === 'all')
              }}
              data={[
                { value: 'wanted', label: 'Wanted books' },
                { value: 'all', label: 'All books' },
              ]}
            />
          </Group>
        ) : (
          <Group gap={6}>
            {sel.editionLabel && <Badge variant="light">{sel.editionLabel}</Badge>}
            {sel.releaseDate && (
              <Text size="sm" c="dimmed">
                released {sel.releaseDate}
              </Text>
            )}
            <Badge size="sm" color={sel.monitored ? 'indigo' : 'gray'} variant="outline">
              {sel.monitored ? 'monitored' : 'unmonitored'}
            </Badge>
            {sel.wanted && (
              <Badge size="sm" color="yellow">
                wanted
              </Badge>
            )}
          </Group>
        )}

        {sel.replaceQueueId && (
          <Alert color="blue" title="Finding a replacement" styles={{ message: { overflowWrap: 'anywhere' } }}>
            {sel.replaceTitle ? `“${sel.replaceTitle}” keeps` : 'The stalled download keeps'} downloading while you look. When you
            download a release here for the same books, Sento stops tracking the old one. Its torrent and files stay in
            your download client.
          </Alert>
        )}
        <Tabs value={tab} onChange={setTab}>
          {/* A whole-edition search has nothing but results to show. */}
          {!editionWide && (
            <Tabs.List>
              <Tabs.Tab value="search" leftSection={<IconListSearch size={14} />}>
                Search
              </Tabs.Tab>
              <Tabs.Tab value="files" leftSection={<IconFile size={14} />}>
                Files ({linked.length})
              </Tabs.Tab>
            </Tabs.List>
          )}

          <Tabs.Panel value="search" pt={editionWide ? 0 : 'sm'}>
            <Stack gap="sm">
              {!result && !searching && (
                <Button
                  size="xs"
                  leftSection={<IconSearch size={14} />}
                  style={{ alignSelf: 'flex-start' }}
                  onClick={() => void interactiveSearch()}
                >
                  Search releases for this {editionWide ? 'edition' : sel.ledger}
                </Button>
              )}
              {result && (
                <Group justify="space-between" gap="xs">
                  <Group gap={6}>
                    <Text size="sm" role="status">
                      {searching
                        ? result.pending.length > 0
                          ? `Searching ${result.pending.length} of ${result.searched.length} ${result.searched.length === 1 ? 'source' : 'sources'}…`
                          : 'Searching…'
                        : result.stopped
                          ? 'Search stopped. Results already received are still available.'
                          : `Searched ${result.searched.length} ${result.searched.length === 1 ? 'source' : 'sources'}`}
                    </Text>
                    {result.queries.length > 0 && (
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        for {result.queries.join(' · ')}
                      </Text>
                    )}
                    {result.pending.map((p) => (
                      <Badge
                        key={p}
                        size="sm"
                        variant="light"
                        color="gray"
                        leftSection={<Loader size={10} />}
                      >
                        {p.split('/')[1] ?? p}
                      </Badge>
                    ))}
                    {problems.length > 0 && (
                      <Button
                        size="compact-xs"
                        variant="subtle"
                        color="orange"
                        aria-expanded={showProblems}
                        onClick={() => setProblemsOpen(!showProblems)}
                      >
                        {problems.length} {problems.length === 1 ? 'source' : 'sources'} had
                        problems
                      </Button>
                    )}
                  </Group>
                  {searching ? (
                    <Button
                      size="compact-xs"
                      variant="default"
                      onClick={() => searchController.current?.abort()}
                    >
                      Stop search
                    </Button>
                  ) : (
                    <Button
                      size="compact-xs"
                      variant="default"
                      leftSection={<IconRefresh size={12} />}
                      onClick={() => void interactiveSearch()}
                      disabled={busy}
                    >
                      Search again
                    </Button>
                  )}
                </Group>
              )}
              {result && (
                <Stack gap={4}>
                  {showProblems && (
                    <Stack
                      gap={2}
                      p="xs"
                      style={{
                        borderLeft: '3px solid var(--mantine-color-orange-6)',
                        background: 'var(--mantine-color-orange-light)',
                        borderRadius: 'var(--mantine-radius-sm)',
                      }}
                    >
                      {problems.map((p) => (
                        <Text key={p} size="xs" style={{ overflowWrap: 'anywhere' }}>
                          {p}
                        </Text>
                      ))}
                    </Stack>
                  )}
                  {!searching &&
                    onAddEdition &&
                    missing.map((m) => (
                      <Group key={`${m.name}/${m.language}`} gap="xs" justify="space-between">
                        <Text size="sm">
                          {m.releases === 1 ? '1 release calls itself' : `${m.releases} releases call themselves`}{' '}
                          {m.name} ({m.language}); no edition of this series has that name.
                        </Text>
                        <Button
                          size="compact-xs"
                          variant="default"
                          onClick={() =>
                            onAddEdition({ editionName: m.name, language: m.language, volumes: m.volumes || undefined })
                          }
                        >
                          Add {m.name}
                        </Button>
                      </Group>
                    ))}
                  {notes.map((n) => (
                    <Text key={n} size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                      {n}
                    </Text>
                  ))}
                  {result.done &&
                    !result.stopped &&
                    problems.length === 0 &&
                    notes.length === 0 &&
                    result.candidates.length === 0 && (
                      <Text size="sm" c="dimmed">
                        No releases found. Check that this series has an enabled indexer for{' '}
                        {sel.ledger}s and that its source matches the correct title.
                      </Text>
                    )}
                  {/* One book needs a choice of release, not a plan. */}
                  {result.plan && result.plan.books.length > 1 ? (
                    // Each batch brings a fresh provisional plan; nothing is
                    // editable until the search ends, so remounting is safe.
                    <AcquisitionPlan key={`${result.plan.searchedAt}/${result.candidates.length}/${searching}`}
                      plan={result.plan} candidates={result.candidates} searching={searching}
                      seriesID={seriesID} ledger={sel.ledger} scope={searchScope(allBooks)} busy={busy}
                      onGrab={grab} onUnblock={unblock} onChanged={onChanged} onBusyChange={setBusy} />
                  ) : result.candidates.length > 0 && (
                    <CandidateList
                      candidates={result.candidates}
                      dropped={result.dropped}
                      onGrab={grab}
                      onUnblock={unblock}
                      busy={busy}
                      searching={searching}
                      editionWide={editionWide}
                    />
                  )}
                </Stack>
              )}
            </Stack>
          </Tabs.Panel>

          {!editionWide && (
            <Tabs.Panel value="files" pt="sm">
              <Stack gap="xs">
                {linked.length === 0 && (
                  <Text size="sm" c="dimmed">
                    No file for this book.
                  </Text>
                )}
                {linked.map((f) => (
                  <Card key={f.id} withBorder padding="xs">
                    <Group justify="space-between" wrap="nowrap">
                      <div style={{ minWidth: 0 }}>
                        <Text size="xs" style={{ wordBreak: 'break-all' }}>
                          {f.path}
                        </Text>
                        <Group gap={6} mt={2}>
                          <Badge size="xs" variant="outline">
                            {f.provenance}
                          </Badge>
                          <Text size="xs" c="dimmed">
                            {fmtSize(f.size)}
                          </Text>
                        </Group>
                      </div>
                      <Button
                        size="compact-xs"
                        color="red"
                        variant="subtle"
                        leftSection={<IconTrash size={14} />}
                        disabled={busy}
                        onClick={() =>
                          confirm({
                            title: 'Delete file from disk?',
                            message: `Delete ${f.path}? This removes the archive from every book and edition that uses it. This cannot be undone. Missing books may download again.`,
                            label: 'Delete file',
                            onConfirm: async () => {
                              await api.deleteFile(f.id)
                              onChanged()
                              notify.ok('File deleted')
                            },
                          })
                        }
                      >
                        Delete
                      </Button>
                    </Group>
                  </Card>
                ))}
              </Stack>
            </Tabs.Panel>
          )}
        </Tabs>
      </Stack>
      {confirmation}
    </Modal>
  )
}
