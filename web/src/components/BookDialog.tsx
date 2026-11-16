import {
  Alert,
  Badge,
  Button,
  Checkbox,
  Group,
  Modal,
  SegmentedControl,
  Stack,
  Tabs,
  Text,
} from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { IconFile, IconListSearch, IconSearch } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type ReleaseCandidate, type SeriesFile } from '../api'
import { fmtNum } from '../format'
import { notify } from '../ui'
import AcquisitionPlan from './AcquisitionPlan'
import type { EditionStart } from './AddEdition'
import CandidateList from './CandidateList'
import { useConfirmAction } from './ConfirmAction'
import { BookFiles } from './books/BookFiles'
import { SearchNotices } from './search/SearchNotices'
import { SearchStatus } from './search/SearchStatus'
import { tidyProblem, useReleaseSearch } from './search/useReleaseSearch'

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
  // Files of the book on disk that this edition does not use.
  unusedFileIds?: number[]
  // Fire the streaming search as soon as the modal opens (the
  // interactive-search buttons; a plain row click opens quietly).
  autoSearch?: boolean
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
  const unused = files.filter((f) => sel.unusedFileIds?.includes(f.id) && !sel.fileIds.includes(f.id))
  const shown = [...linked, ...unused]

  const { confirm, confirmation } = useConfirmAction()
  const qc = useQueryClient()
  const [busy, setBusy] = useState(false)
  // Edition-wide searches cover the wanted books unless widened to all.
  const [allBooks, setAllBooks] = useState(false)
  const [replaceOnReview, setReplaceOnReview] = useState(false)
  // Search buttons land on Search; a quiet row click lands on Files when
  // there's a file to look at.
  const [tab, setTab] = useState<string | null>(
    sel.autoSearch || editionWide || shown.length === 0 ? 'search' : 'files',
  )
  const small = useMediaQuery('(max-width: 48em)')

  const { stop, searching, result, searchScope, interactiveSearch } = useReleaseSearch(seriesID, sel, editionWide, allBooks, busy)

  // Source failures are secondary to the results: one line, opened on
  // demand, or straight away when they are all there is to show. Notes
  // ("no results", "no confident match") are answers, not failures.
  const problems = [...new Set(result?.errors ?? [])].map(tidyProblem)
  const notes = [...new Set(result?.notes ?? [])].map(tidyProblem)
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
        ...(replaceOnReview ? { requestedReplacements: (c.coverage ?? []).filter((b) => b.versionId === sel.versionId && b.bookId === sel.bookId) } : {}),
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
            <Badge size="sm" color="gray" variant="outline">
              {sel.monitored ? 'Collected' : 'Paused'}
            </Badge>
            {sel.wanted && (
              <Badge size="sm" color="red">
                Wanted
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
                Files ({shown.length})
              </Tabs.Tab>
            </Tabs.List>
          )}

          <Tabs.Panel value="search" pt={editionWide ? 0 : 'sm'}>
            <Stack gap="sm">
              {!editionWide && sel.fileIds.length > 0 && <Checkbox size="sm" disabled={busy}
                label="Replace the existing copy if this download needs file review"
                checked={replaceOnReview} onChange={(event) => setReplaceOnReview(event.currentTarget.checked)} />}
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
                <SearchStatus result={result} searching={searching} problems={problems} showProblems={showProblems} setProblemsOpen={setProblemsOpen} stop={stop} interactiveSearch={interactiveSearch} busy={busy} />
              )}
              {result && (
                <Stack gap={4}>
                  <SearchNotices result={result} searching={searching} problems={problems} notes={notes} showProblems={showProblems} onAddEdition={onAddEdition} sel={sel} />
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
              <BookFiles shown={shown} unused={unused} seriesID={seriesID} sel={sel} busy={busy} confirm={confirm} onChanged={onChanged} />
            </Tabs.Panel>
          )}
        </Tabs>
      </Stack>
      {confirmation}
    </Modal>
  )
}
