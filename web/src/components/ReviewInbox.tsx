import { useState } from 'react'
import { Alert, Button, Card, Group, Modal, Stack, Text, TextInput } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'
import type { ReviewItem } from '../types/store'

export default function ReviewInbox({
  seriesID = 0,
  onFile,
}: {
  seriesID?: number
  onFile?: (id: number) => void
}) {
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: ['review', seriesID],
    queryFn: () => api.review(seriesID),
    refetchInterval: 15000,
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [remote, setRemote] = useState<ReviewItem | null>(null)
  const [remoteID, setRemoteID] = useState('')
  const { confirm, confirmation } = useConfirmAction()
  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError('')
    try {
      await fn()
    } catch (e) {
      setError(String(e))
    } finally {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ['review'] }),
        qc.invalidateQueries({ queryKey: ['series'] }),
        qc.invalidateQueries({ queryKey: ['queue'] }),
      ])
      setBusy(false)
    }
  }
  const chapters = (q.data ?? []).filter((i) => i.kind === 'chapter')
  return (
    <Stack gap="sm">
      <Text size="sm" c="dimmed">
        Resolve file matches, new chapters, and downloads that need attention. Existing downloads
        stay reserved until you resolve or cancel them.
      </Text>
      <QueryFeedback pending={q.isPending} error={q.error} onRetry={() => void q.refetch()} />
      {error && <Alert color="red">{error}</Alert>}
      {chapters.length > 1 && (
        <Group>
          <Button
            size="xs"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                for (const i of chapters) await api.reviewObservation(i.seriesId, i.itemId, true)
              })
            }
          >
            Accept new chapters ({chapters.length})
          </Button>
          <Button
            size="xs"
            variant="default"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                for (const i of chapters) await api.reviewObservation(i.seriesId, i.itemId, false)
              })
            }
          >
            Dismiss new chapters
          </Button>
        </Group>
      )}
      {q.data?.length === 0 && (
        <Text c="dimmed" size="sm">
          Nothing needs review.
        </Text>
      )}
      {q.data?.map((i) => (
        <Card key={i.id} withBorder padding="sm">
          <Stack gap={6}>
            <Text size="xs" c="dimmed">
              {i.seriesTitle} ·{' '}
              {i.kind === 'file' && (
                <Button
                  size="xs"
                  variant="subtle"
                  disabled={busy}
                  onClick={() => void run(() => api.checkFiles(i.seriesId))}
                >
                  Recheck files
                </Button>
              )}
              {i.kind === 'chapter'
                ? 'New chapter'
                : i.kind === 'file'
                  ? 'File review'
                  : 'Download'}
            </Text>
            <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>
              {i.kind === 'file' ? i.title.split(/[\\/]/).pop() : i.title}
            </Text>
            <Text
              size="xs"
              c="dimmed"
              lineClamp={3}
              title={i.detail}
              style={{ overflowWrap: 'anywhere' }}
            >
              {i.detail}
            </Text>
            {i.detail.length > 180 && (
              <details>
                <summary style={{ fontSize: 'var(--mantine-font-size-xs)', cursor: 'pointer' }}>
                  Show full details
                </summary>
                <Text
                  component="pre"
                  size="xs"
                  mt="xs"
                  style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
                >
                  {i.detail}
                </Text>
              </details>
            )}
            <Group gap="xs">
              {i.kind === 'file' &&
                (onFile ? (
                  <Button size="xs" variant="default" onClick={() => onFile(i.itemId)}>
                    Review file
                  </Button>
                ) : (
                  <Button
                    size="xs"
                    variant="default"
                    component={Link}
                    to={`/series/${i.seriesId}?view=review`}
                  >
                    Review files
                  </Button>
                ))}
              {i.kind === 'chapter' && (
                <>
                  <Button
                    size="xs"
                    disabled={busy}
                    onClick={() =>
                      void run(() => api.reviewObservation(i.seriesId, i.itemId, true))
                    }
                  >
                    Add chapter
                  </Button>
                  <Button
                    size="xs"
                    variant="subtle"
                    disabled={busy}
                    onClick={() =>
                      void run(() => api.reviewObservation(i.seriesId, i.itemId, false))
                    }
                  >
                    Dismiss
                  </Button>
                </>
              )}
              {i.kind === 'download' && (
                <>
                  {i.status !== 'failed' && (
                    <Button
                      size="xs"
                      variant="default"
                      disabled={busy}
                      onClick={() => void run(() => api.retryQueue(i.itemId))}
                    >
                      Retry existing download
                    </Button>
                  )}
                  {i.status === 'dispatch_unknown' && (
                    <Button
                      size="xs"
                      variant="subtle"
                      onClick={() => {
                        setRemote(i)
                        setRemoteID('')
                      }}
                    >
                      Attach client ID
                    </Button>
                  )}
                  <Button
                    size="xs"
                    variant="subtle"
                    color="red"
                    onClick={() =>
                      confirm({
                        title: 'Cancel this job?',
                        message:
                          'Release its book reservations. Downloaded files stay on disk; the remote client is not modified.',
                        label: 'Cancel job',
                        onConfirm: async () => {
                          await api.cancelQueue(i.itemId)
                          await qc.invalidateQueries({ queryKey: ['review'] })
                          await qc.invalidateQueries({ queryKey: ['queue'] })
                          await qc.invalidateQueries({ queryKey: ['series'] })
                        },
                      })
                    }
                  >
                    Cancel job
                  </Button>
                  <Button
                    component={Link}
                    to={`/series/${i.seriesId}?view=review`}
                    size="xs"
                    variant="subtle"
                  >
                    Review files
                  </Button>
                </>
              )}
            </Group>
          </Stack>
        </Card>
      ))}
      <Modal
        opened={!!remote}
        onClose={() => {
          if (!busy) setRemote(null)
        }}
        title="Attach existing download"
      >
        <Stack>
          <Text size="sm">
            Enter the existing download's client ID (torrent hash for torrents). Sento will check
            and import it without submitting a new download.
          </Text>
          <TextInput
            label="Client download ID"
            value={remoteID}
            onChange={(e) => setRemoteID(e.currentTarget.value)}
          />
          <Button
            disabled={!remoteID.trim() || busy}
            onClick={() =>
              void run(async () => {
                await api.retryQueue(remote!.itemId, remoteID.trim())
                setRemote(null)
              })
            }
          >
            Attach download
          </Button>
        </Stack>
      </Modal>
      {confirmation}
    </Stack>
  )
}
