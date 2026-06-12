import { useEffect, useState } from 'react'
import { Alert, Badge, Button, Card, Group, Loader, Modal, Stack, Tabs, Text } from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { IconFile, IconListSearch, IconRefresh, IconSearch, IconTrash } from '@tabler/icons-react'
import { api, fmtNum, fmtSize, type ReleaseCandidate, type SeriesFile } from '../api'
import { notify } from '../ui'
import CandidateList from './CandidateList'

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
      })
    } catch (e) {
      notify.err(e)
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

  const run = async (fn: () => Promise<unknown>, doneMsg?: string) => {
    setBusy(true)
    try {
      await fn()
      if (doneMsg) notify.ok(doneMsg)
    } catch (e) {
      notify.err(e)
    } finally {
      setBusy(false)
    }
  }

  const grab = (c: ReleaseCandidate, force: boolean) =>
    run(
      () =>
        api
          .search(seriesID, {
            ledger: sel.ledger,
            grab: c.guid,
            force,
            slot: { collectionId: sel.collectionId ?? 0, number: sel.number },
          })
          .then(onChanged),
      `Grabbed via ${c.provider}/${c.site}`,
    )

  return (
    <Modal opened onClose={onClose} title={label} size="xl" fullScreen={small}>
      <Stack gap="sm">
        {editionWide ? (
          <Text size="sm" c="dimmed">
            Candidates are matched against every wanted {sel.ledger} of this collection —
            multi-{sel.ledger} packs cover several slots at once. Packs that are mostly already
            owned get flagged and need a force grab.
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
                  {result?.done && (
                    <Button
                      size="compact-xs"
                      variant="default"
                      leftSection={<IconRefresh size={12} />}
                      onClick={interactiveSearch}
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
                  {result.done && result.candidates.length === 0 && (
                    <Text size="sm" c="dimmed">
                      No releases found for this {sel.ledger}. Check the Indexers tab — sites must
                      be enabled for this ledger, and extension sources need a confident title
                      match.
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
                        onClick={() => {
                          if (
                            !window.confirm(
                              `Delete from disk?\n${f.path}\n\nThe slot becomes wanted again and may re-download.`,
                            )
                          )
                            return
                          run(() => api.deleteFile(f.id).then(onChanged), 'File deleted')
                        }}
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
    </Modal>
  )
}
