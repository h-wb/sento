import { useState } from 'react'
import { Alert, Button, Group, Stack, Text } from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Edition, ReviewItem } from '../types/store'
import { CatalogLink } from './CatalogLink'
import { fileName } from './editionText'
import { FileReviewDialog } from './FileReviewDialog'

export default function ReviewQuestions({
  items,
  seriesID = 0,
  onSources,
}: {
  items: ReviewItem[]
  seriesID?: number
  // Open the series' sources, for a source that found nothing.
  onSources?: () => void
}) {
  const qc = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [fileReview, setFileReview] = useState<ReviewItem | null>(null)
  const [link, setLink] = useState<{ item: ReviewItem; editions: Edition[] } | null>(null)
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
        qc.invalidateQueries({ queryKey: ['activity'] }),
        qc.invalidateQueries({ queryKey: ['series'] }),
        qc.invalidateQueries({ queryKey: ['queue'] }),
      ])
      setBusy(false)
    }
  }
  const chapters = items.filter((i) => i.kind === 'chapter')
  const proposed = items.filter((i) => i.proposal)
  // Series with files waiting: one recheck for them all, not one per file.
  const fileSeries = [...new Set(items.filter((i) => i.kind === 'file').map((i) => i.seriesId))]
  const accept = (i: ReviewItem) => api.mapFile(i.seriesId, i.itemId, i.proposal!.books)
  return (
    <Stack gap="sm">
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
      {items.map((i) => (
        <div
          key={i.id}
          style={{
            paddingBlock: 10,
            borderBottom: '1px solid var(--mantine-color-default-border)',
          }}
        >
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
              {i.kind === 'file' && (
                <Button size="xs" variant="default" onClick={() => setFileReview(i)}>
                  Review file
                </Button>
              )}
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
            </Group>
          </Stack>
        </div>
      ))}
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
              qc.invalidateQueries({ queryKey: ['activity'] }),
              qc.invalidateQueries({ queryKey: ['series'] }),
            ])
          }}
          close={() => setLink(null)}
        />
      )}
      {fileReview && <FileReviewDialog item={fileReview} close={() => setFileReview(null)} />}
    </Stack>
  )
}
