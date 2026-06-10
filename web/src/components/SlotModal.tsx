import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Divider,
  Group,
  Loader,
  Modal,
  Stack,
  Text,
  Tooltip,
} from '@mantine/core'
import { IconSearch, IconTrash } from '@tabler/icons-react'
import { api, fmtNum, fmtSize, type ReleaseCandidate, type SeriesFile } from '../api'

export type SlotSelection = {
  ledger: 'volume' | 'chapter'
  number: number // hundredths
  collectionId?: number
  collectionName?: string
  releaseDate?: string
  monitored: boolean
  wanted: boolean
  fileIds: number[]
}

// Rejections force-grab cannot bypass (mirror of the backend's hard set).
const hardRejections = new Set([
  'rejected.content_type',
  'rejected.series_match',
  'rejected.edition_routing',
  'rejected.no_coverage',
])

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
  const [busy, setBusy] = useState(false)
  const [searching, setSearching] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [result, setResult] = useState<StreamResult | null>(null)

  if (!sel) return null
  const linked = files.filter((f) => sel.fileIds.includes(f.id))
  const label = `${sel.ledger === 'volume' ? 'Volume' : 'Chapter'} ${fmtNum(sel.number)}`

  const run = async (fn: () => Promise<unknown>, doneMsg?: string) => {
    setBusy(true)
    setError(null)
    try {
      await fn()
      if (doneMsg) setNotice(doneMsg)
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  const slotRef = { collectionId: sel.collectionId ?? 0, number: sel.number }

  // Streaming search: each provider's batch renders as soon as it lands,
  // so fast extension sources show up while torrent indexers still load.
  const interactiveSearch = async () => {
    setSearching(true)
    setError(null)
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
                candidates: [...prev.candidates, ...(ev.candidates ?? [])].sort(
                  (a, b) => b.score - a.score,
                ),
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
          }
        })
      })
    } catch (e) {
      setError(String(e))
    } finally {
      setSearching(false)
      setResult((prev) => (prev ? { ...prev, pending: [], done: true } : prev))
    }
  }

  const grab = (c: ReleaseCandidate, force: boolean) =>
    run(
      () =>
        api
          .search(seriesID, { ledger: sel.ledger, grab: c.guid, force, slot: slotRef })
          .then(onChanged),
      `Grabbed via ${c.provider}/${c.site}`,
    )

  const close = () => {
    setResult(null)
    setError(null)
    setNotice(null)
    onClose()
  }

  return (
    <Modal opened onClose={close} title={label} size="xl">
      <Stack gap="sm">
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

        {error && (
          <Alert color="red" withCloseButton onClose={() => setError(null)}>
            {error}
          </Alert>
        )}
        {notice && (
          <Alert color="green" withCloseButton onClose={() => setNotice(null)}>
            {notice}
          </Alert>
        )}

        <Divider label="Files" labelPosition="left" />
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

        <Divider label="Interactive search" labelPosition="left" />
        <Group>
          <Button
            size="xs"
            leftSection={<IconSearch size={14} />}
            loading={searching}
            onClick={interactiveSearch}
          >
            Search releases for this {sel.ledger}
          </Button>
          {result?.pending.map((p) => (
            <Badge key={p} size="sm" variant="light" color="gray" leftSection={<Loader size={10} />}>
              {p.split('/')[1] ?? p}
            </Badge>
          ))}
        </Group>

        {result && (
          <Stack gap={4}>
            <Text size="xs" c="dimmed">
              {result.done ? 'Searched' : 'Searching'} {result.searched.join(', ') || 'nothing'}
              {result.dropped > 0 && ` · ${result.dropped} entries for the other ledger skipped`}
            </Text>
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
                No releases found for this {sel.ledger}. Check the Indexers tab — sites must be
                enabled for this ledger, and extension sources need a confident title match.
              </Text>
            )}
            {result.candidates.map((c) => {
              const rejected = c.rejections?.length ? c.rejections[0] : null
              const hard = rejected !== null && hardRejections.has(rejected)
              return (
                <Group key={c.guid} justify="space-between" wrap="nowrap">
                  <Text size="xs" lineClamp={1} style={{ flex: 1 }} title={c.title}>
                    {c.title}
                  </Text>
                  <Group gap={6} wrap="nowrap">
                    <Badge
                      size="xs"
                      variant="outline"
                      color={c.protocol === 'torrent' ? 'blue' : 'teal'}
                    >
                      {c.provider}/{c.site}
                    </Badge>
                    {c.protocol === 'torrent' && (
                      <Text size="xs" c="dimmed">
                        {fmtSize(c.size ?? 0)} · {c.seeders ?? 0}S
                      </Text>
                    )}
                    {c.scanlator && (
                      <Badge size="xs" variant="light">
                        {c.scanlator}
                      </Badge>
                    )}
                    {rejected ? (
                      <Badge size="xs" color="red" variant="light">
                        {rejected.replace('rejected.', '')}
                      </Badge>
                    ) : (
                      <Badge size="xs" color="green" variant="light">
                        {c.score}
                      </Badge>
                    )}
                    <Tooltip
                      label={
                        hard
                          ? 'Hard rejection — cannot grab'
                          : rejected
                            ? 'Force grab (bypasses soft rejection)'
                            : 'Grab'
                      }
                    >
                      <Button
                        size="compact-xs"
                        color={rejected ? 'orange' : 'indigo'}
                        disabled={hard || busy}
                        onClick={() => grab(c, !!rejected)}
                      >
                        {rejected ? 'Force' : 'Grab'}
                      </Button>
                    </Tooltip>
                  </Group>
                </Group>
              )
            })}
          </Stack>
        )}
      </Stack>
    </Modal>
  )
}
