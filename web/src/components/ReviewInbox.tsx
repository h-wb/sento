import { useState } from 'react'
import { Alert, Button, Card, Group, Modal, Stack, Text, TextInput } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'
import type { Edition, ReviewItem, ReviewProposal } from '../types/store'
import { CatalogLink } from './EditionDialogs'
import { fileName } from './editionText'

export default function ReviewInbox({
  seriesID = 0,
  onFile,
  onSources,
}: {
  seriesID?: number
  onFile?: (id: number, proposal?: ReviewProposal) => void
  // Open the series' sources, for a source that found nothing.
  onSources?: () => void
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
  const [link, setLink] = useState<{ item: ReviewItem; editions: Edition[] } | null>(null)
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
  const proposed = (q.data ?? []).filter((i) => i.proposal)
  // Series with files waiting: one recheck for them all, not one per file.
  const fileSeries = [...new Set((q.data ?? []).filter((i) => i.kind === 'file').map((i) => i.seriesId))]
  const accept = (i: ReviewItem) => api.mapFile(i.seriesId, i.itemId, i.proposal!.books)
  return (
    <Stack gap="sm">
      <Text size="sm" c="dimmed">
        Resolve file matches, new chapters, catalog editions that match yours, sources that found
        nothing, and downloads that need attention. Existing downloads stay reserved until you
        resolve or cancel them.
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
      {(proposed.length > 1 || fileSeries.length > 0) && (
        <Group>
          {proposed.length > 1 && (
            <Button
              size="xs"
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  for (const i of proposed) await accept(i)
                })
              }
            >
              Accept {proposed.length} proposed matches
            </Button>
          )}
          {fileSeries.length > 0 && (
            <Button
              size="xs"
              variant="default"
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  for (const id of fileSeries) await api.checkFiles(id)
                })
              }
            >
              Recheck files
            </Button>
          )}
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
              {seriesID ? '' : `${i.seriesTitle} · `}
              {i.kind === 'chapter'
                ? 'New chapter'
                : i.kind === 'file'
                  ? 'File review'
                  : i.kind === 'edition'
                    ? 'Catalog edition available'
                    : i.kind === 'source'
                      ? 'Source without a match'
                      : 'Download'}
            </Text>
            <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>
              {i.kind === 'file' ? fileName(i.title) : i.title}
            </Text>
            {i.proposal ? (
              <Text size="sm">
                Proposed: <b>{i.proposal.label}</b> of {i.proposal.edition}{' '}
                <Text span size="xs" c="dimmed">
                  ({i.proposal.evidence})
                </Text>
              </Text>
            ) : (
              <Text
                size="xs"
                c="dimmed"
                lineClamp={3}
                title={i.detail}
                style={{ overflowWrap: 'anywhere' }}
              >
                {i.detail}
              </Text>
            )}
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
              {i.proposal && (
                <Button size="xs" disabled={busy} onClick={() => void run(() => accept(i))}>
                  Accept
                </Button>
              )}
              {i.kind === 'file' &&
                (onFile ? (
                  <Button
                    size="xs"
                    variant="default"
                    onClick={() => onFile(i.itemId, i.proposal)}
                  >
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
              {i.kind === 'edition' && (
                <>
                  <Button
                    size="xs"
                    disabled={busy}
                    onClick={() =>
                      void run(async () =>
                        setLink({ item: i, editions: (await api.getSeries(i.seriesId)).editions }),
                      )
                    }
                  >
                    Review and link
                  </Button>
                  <Button
                    size="xs"
                    variant="subtle"
                    disabled={busy}
                    onClick={() =>
                      void run(() => api.declineCatalogLink(i.seriesId, i.itemId, i.relatedId ?? 0))
                    }
                  >
                    Not the same edition
                  </Button>
                </>
              )}
              {i.kind === 'source' &&
                (onSources ? (
                  <Button size="xs" variant="default" onClick={onSources}>
                    Open sources
                  </Button>
                ) : (
                  <Button
                    size="xs"
                    variant="default"
                    component={Link}
                    to={`/series/${i.seriesId}?sources=1`}
                  >
                    Open sources
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
      {link && link.editions.some((e) => e.id === link.item.itemId) && (
        <CatalogLink
          seriesID={link.item.seriesId}
          value={link.editions.find((e) => e.id === link.item.itemId)!}
          editions={link.editions}
          catalogId={link.item.relatedId}
          done={async () => {
            setLink(null)
            await Promise.all([
              qc.invalidateQueries({ queryKey: ['review'] }),
              qc.invalidateQueries({ queryKey: ['series'] }),
            ])
          }}
          close={() => setLink(null)}
        />
      )}
      {confirmation}
    </Stack>
  )
}
