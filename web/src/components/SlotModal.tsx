import { useEffect, useRef, useState } from 'react'
import { Alert, Badge, Button, Card, Group, Loader, Modal, Stack, Tabs, Text } from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { IconFile, IconListSearch, IconRefresh, IconSearch, IconTrash } from '@tabler/icons-react'
import { api, fmtNum, fmtSize, type ReleaseCandidate, type SeriesFile } from '../api'
import { notify } from '../ui'
import CandidateList from './CandidateList'
import { useConfirmAction } from './ConfirmAction'

export type SlotSelection = {
  ledger: 'volume' | 'chapter'
  number: number // hundredths
  collectionId?: number
  collectionName?: string
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
  searched: string[]
  pending: string[] // "provider/site" entries still searching
  candidates: ReleaseCandidate[]
  dropped: number
  notes: string[]
  errors: string[]
  done: boolean
  stopped?: boolean
}

export default function SlotModal({
  seriesID,
  sel,
  files,
  onClose,
  onChanged,
}: {
  seriesID: number
  sel: SlotSelection | null
  files: SeriesFile[]
  onClose: () => void
  onChanged: () => void
}) {
  if (!sel) return null
  // Keyed by slot so every open starts fresh (tab choice, search state).
  return (
    <Inner
      key={`${sel.ledger}-${sel.collectionId ?? 0}-${sel.number}`}
      seriesID={seriesID}
      sel={sel}
      files={files}
      onClose={onClose}
      onChanged={onChanged}
    />
  )
}

function Inner({
  seriesID,
  sel,
  files,
  onClose,
  onChanged,
}: {
  seriesID: number
  sel: SlotSelection
  files: SeriesFile[]
  onClose: () => void
  onChanged: () => void
}) {
  // number 0 = the whole collection (every wanted volume/chapter): the
  // place to grab packs and complete releases.
  const editionWide = sel.number === 0
  const linked = files.filter((f) => sel.fileIds.includes(f.id))

  const { confirm, confirmation } = useConfirmAction()
  const searchController = useRef<AbortController | null>(null)
  useEffect(() => () => searchController.current?.abort(), [])
  const [busy, setBusy] = useState(false)
  const [searching, setSearching] = useState(false)
  const [result, setResult] = useState<StreamResult | null>(null)
  // Search buttons land on Search; a quiet row click lands on Files when
  // there's a file to look at.
  const [tab, setTab] = useState<string | null>(
    sel.autoSearch || editionWide || linked.length === 0 ? 'search' : 'files',
  )
  const small = useMediaQuery('(max-width: 48em)')

  // Streaming search: each provider's batch renders as soon as it lands,
  // so fast extension sources show up while torrent indexers still load.
  const interactiveSearch = async () => {
    if (searching || busy) return
    const controller = new AbortController()
    searchController.current = controller
    const slotRef = { collectionId: sel.collectionId ?? 0, number: sel.number }
    setSearching(true)
    setResult({
      searched: [],
      pending: [],
      candidates: [],
      dropped: 0,
      notes: [],
      errors: [],
      done: false,
    })
    try {
      await api.searchStream(seriesID, { ledger: sel.ledger, slot: slotRef }, (ev) => {
        setResult((prev) => {
          if (!prev) return prev
          switch (ev.type) {
            case 'sites':
              return {
                ...prev,
                searched: ev.searched ?? [],
                pending: ev.searched ?? [],
              }
            case 'batch':
              return {
                ...prev,
                pending: prev.pending.filter((p) => !(ev.sites ?? []).includes(p)),
                candidates: [...prev.candidates, ...(ev.candidates ?? [])],
                dropped: prev.dropped + (ev.dropped ?? 0),
                notes: [...prev.notes, ...(ev.bindingNotes ?? [])],
                errors: ev.error ? [...prev.errors, ev.error] : prev.errors,
              }
            case 'done':
              return { ...prev, pending: [], done: true }
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
      }, controller.signal)
    } catch (e) {
      if (controller.signal.aborted) setResult((prev) => prev ? { ...prev, stopped: true } : prev)
      if (!controller.signal.aborted) setResult((prev) => prev ? { ...prev, errors: [...prev.errors, e instanceof Error ? e.message : String(e)] } : prev)
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

  const label = editionWide
    ? `${sel.collectionName ?? 'Collection'} — wanted ${sel.ledger}s`
    : `${sel.ledger === 'volume' ? 'Volume' : 'Chapter'} ${fmtNum(sel.number)}`

  const grab = async (c: ReleaseCandidate, force: boolean) => {
    setBusy(true)
    try {
      await api.search(seriesID, { ledger: sel.ledger, grab: c.guid, force, slot: { collectionId: sel.collectionId ?? 0, number: sel.number } })
      onChanged()
      notify.ok('Release queued for download')
    } finally { setBusy(false) }
  }

  return (
    <Modal opened onClose={onClose} title={label} size="xl" fullScreen={small} closeOnEscape={!busy} closeOnClickOutside={!busy} withCloseButton={!busy}>
      <Stack gap="sm">
        {editionWide ? (
          <Text size="sm" c="dimmed">
            Browse releases for the wanted {sel.ledger}s in this collection. Packs can include several {sel.ledger}s.
          </Text>
        ) : (
          <Group gap={6}>
            {sel.collectionName && <Badge variant="light">{sel.collectionName}</Badge>}
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

        <Tabs value={tab} onChange={setTab}>
          <Tabs.List>
            <Tabs.Tab value="search" leftSection={<IconListSearch size={14} />}>
              Search
            </Tabs.Tab>
            {!editionWide && (
              <Tabs.Tab value="files" leftSection={<IconFile size={14} />}>
                Files ({linked.length})
              </Tabs.Tab>
            )}
          </Tabs.List>

          <Tabs.Panel value="search" pt="sm">
            <Stack gap="sm">
              {!result && !searching && (
                <Button
                  size="xs"
                  leftSection={<IconSearch size={14} />}
                  style={{ alignSelf: 'flex-start' }}
                  onClick={interactiveSearch}
                >
                  Search releases for this {editionWide ? 'collection' : sel.ledger}
                </Button>
              )}
              {(searching || (result?.pending.length ?? 0) > 0 || result?.done) && (
                <Group gap={6}>
                  {searching && <Button size="compact-xs" variant="default" onClick={() => searchController.current?.abort()}>Stop search</Button>}
                  {result?.done && (
                    <Button
                      size="compact-xs"
                      variant="default"
                      leftSection={<IconRefresh size={12} />}
                      onClick={interactiveSearch}
                      disabled={busy}
                    >
                      Search again
                    </Button>
                  )}
                  {result?.pending.map((p) => (
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
                </Group>
              )}

              {result && (
                <Stack gap={4}>
                  {result.notes.map((n) => (
                    <Text key={n} size="xs" c="dimmed">
                      {n}
                    </Text>
                  ))}
                  {result.errors.map((e) => (
                    <Alert key={e} color="orange" p="xs">
                      {e}
                    </Alert>
                  ))}
                  {result.done && !result.stopped && result.errors.length === 0 && result.candidates.length === 0 && (
                    <Text size="sm" c="dimmed">
                      No releases found. Check that this series has an enabled indexer for {sel.ledger}s and that its source matches the correct title.
                    </Text>
                  )}
                  {result.candidates.length > 0 && (
                    <CandidateList
                      candidates={result.candidates}
                      dropped={result.dropped}
                      onGrab={grab}
                      busy={busy}
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
                    No file for this slot.
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
                        onClick={() => confirm({ title: 'Delete file from disk?', message: `Delete ${f.path}? This cannot be undone. Sento may download the missing ${sel.ledger} again.`, label: 'Delete file', onConfirm: async () => { await api.deleteFile(f.id); onChanged(); notify.ok('File deleted') } })}
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
