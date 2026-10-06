import { useState } from 'react'
import { Alert, Card, Checkbox, Group, Select, Stack, Text } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Collection } from '../types/store'

// Which edition each bound source listing publishes. A verified mapping
// is what lets a source release count as evidence for one edition.
export default function SourceMappings({
  seriesID,
  collections,
}: {
  seriesID: number
  collections: Collection[]
}) {
  const qc = useQueryClient()
  const bindings = useQuery({
    queryKey: ['series', seriesID, 'bindings'],
    queryFn: () => api.bindings(seriesID),
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError('')
    try {
      await fn()
      await qc.invalidateQueries({ queryKey: ['series', seriesID] })
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }
  const active = (bindings.data ?? []).filter((b) => b.state === 'active')
  const editions = collections.filter((c) => c.syncState !== 'superseded')
  if (!active.length) return null
  return (
    <Card withBorder>
      <Text fw={600}>Source edition mappings</Text>
      <Text size="sm" c="dimmed" mb="sm">
        Use verified listing identity to distinguish editions. A selected download version alone is
        not evidence.
      </Text>
      {error && (
        <Alert color="red" mb="sm">
          {error}
        </Alert>
      )}
      <Stack>
        {active.map((b) => (
          <Group key={b.id}>
            <Text size="sm" style={{ minWidth: 0, flex: 1 }}>
              {b.sourceName} · {b.mangaTitle}
            </Text>
            <Select
              aria-label={`Edition for ${b.mangaTitle}`}
              placeholder="Edition not verified"
              clearable
              data={editions.map((c) => ({
                value: String(c.id),
                label: [c.language, c.editionName, c.publisher, c.medium]
                  .filter(Boolean)
                  .join(' · '),
              }))}
              value={b.collectionId ? String(b.collectionId) : null}
              disabled={busy}
              onChange={(v) =>
                void run(() => api.setBindingEdition(seriesID, b.id, Number(v) || 0))
              }
              style={{ minWidth: 180, flex: 1 }}
            />
            {collections.find((c) => c.id === b.collectionId)?.type === 'chapter' && (
              <Checkbox
                label="Trust new chapters from this listing"
                description="Add observed chapters automatically when chapter discovery is enabled for this edition."
                checked={b.trustChapters}
                disabled={busy}
                onChange={(e) =>
                  void run(() => api.trustBinding(seriesID, b.id, e.currentTarget.checked))
                }
              />
            )}
          </Group>
        ))}
      </Stack>
    </Card>
  )
}
