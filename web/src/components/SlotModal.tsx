import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Divider,
  Group,
  Modal,
  Stack,
  Text,
  Tooltip,
} from '@mantine/core'
import { IconSearch, IconTrash } from '@tabler/icons-react'
import {
  api,
  fmtNum,
  fmtSize,
  type ChapterCandidate,
  type SeriesFile,
  type VolumeCandidate,
} from '../api'

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
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [volCands, setVolCands] = useState<VolumeCandidate[] | null>(null)
  const [chCands, setChCands] = useState<ChapterCandidate[] | null>(null)

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

  const interactiveSearch = () =>
    run(async () => {
      if (sel.ledger === 'volume') {
        const r = await api.searchVolumes(seriesID, {
          dryRun: true,
          slot: { collectionId: sel.collectionId!, number: sel.number },
        })
        setVolCands(r.candidates)
      } else {
        const r = await api.searchChapters(seriesID, {
          dryRun: true,
          slots: [sel.number],
          all: true,
        })
        setChCands(r.candidates)
      }
    })

  const close = () => {
    setVolCands(null)
    setChCands(null)
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
                  if (!window.confirm(`Delete from disk?\n${f.path}\n\nThe slot becomes wanted again and may re-download.`)) return
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
            loading={busy}
            onClick={interactiveSearch}
          >
            Search releases for this {sel.ledger}
          </Button>
        </Group>

        {volCands && (
          <Stack gap={4}>
            {volCands.length === 0 && (
              <Text size="sm" c="dimmed">
                No releases found.
              </Text>
            )}
            {volCands.map((c) => {
              const rejected = c.rejections?.length ? c.rejections[0] : null
              const hard = rejected !== null && hardRejections.has(rejected)
              return (
                <Group key={c.guid} justify="space-between" wrap="nowrap">
                  <Text size="xs" lineClamp={1} style={{ flex: 1 }} title={c.title}>
                    {c.title}
                  </Text>
                  <Group gap={6} wrap="nowrap">
                    <Badge size="xs" variant="outline" color={c.protocol === 'torrent' ? 'blue' : 'teal'}>
                      {c.protocol}/{c.site}
                    </Badge>
                    {c.protocol === 'torrent' && (
                      <Text size="xs" c="dimmed">
                        {fmtSize(c.size ?? 0)} · {c.seeders ?? 0}S
                      </Text>
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
                    <Tooltip label={hard ? 'Hard rejection — cannot grab' : rejected ? 'Force grab (bypasses soft rejection)' : 'Grab'}>
                      <Button
                        size="compact-xs"
                        color={rejected ? 'orange' : 'indigo'}
                        disabled={hard || busy}
                        onClick={() =>
                          run(
                            () =>
                              api
                                .searchVolumes(seriesID, {
                                  grab: c.guid,
                                  force: !!rejected,
                                  slot: { collectionId: sel.collectionId!, number: sel.number },
                                })
                                .then(onChanged),
                            'Grabbed — sent to qBittorrent',
                          )
                        }
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

        {chCands && (
          <Stack gap={4}>
            {chCands.length === 0 && (
              <Text size="sm" c="dimmed">
                No source chapters matched this slot.
              </Text>
            )}
            {chCands.map((c, i) => (
              <Group key={`${c.chapterId}-${i}`} justify="space-between" wrap="nowrap">
                <Text size="xs" lineClamp={1} style={{ flex: 1 }}>
                  {c.chapterName}
                </Text>
                <Group gap={6} wrap="nowrap">
                  {c.scanlator && (
                    <Badge size="xs" variant="outline">
                      {c.scanlator}
                    </Badge>
                  )}
                  <Button
                    size="compact-xs"
                    disabled={busy}
                    onClick={() =>
                      run(
                        () => api.grabChapter(seriesID, c).then(onChanged),
                        'Grabbed — downloading via Suwayomi',
                      )
                    }
                  >
                    Grab
                  </Button>
                </Group>
              </Group>
            ))}
          </Stack>
        )}
      </Stack>
    </Modal>
  )
}
