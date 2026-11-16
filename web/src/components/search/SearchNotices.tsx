import { Button, Group, Stack, Text } from '@mantine/core'
import type { EditionStart } from '../AddEdition'
import type { BookSelection } from '../BookDialog'
import type { StreamResult } from './useReleaseSearch'

export function SearchNotices({
  result,
  searching,
  problems,
  notes,
  showProblems,
  onAddEdition,
  sel,
}: {
  result: StreamResult
  searching: boolean
  problems: string[]
  notes: string[]
  showProblems: boolean
  onAddEdition?: (start: EditionStart) => void
  sel: BookSelection
}) {
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
    const group = byEdition.get(key) ?? {
      name: o.name,
      language: o.language,
      volumes: 0,
      titles: new Set(),
    }
    group.titles.add(c.title)
    group.volumes = Math.max(group.volumes, o.volumes)
    byEdition.set(key, group)
  }
  const missing = [...byEdition.values()].map((m) => ({ ...m, releases: m.titles.size }))
  return (
    <>
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
            <Text key={p} size="xs" className="wrapAnywhere">
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
              {m.releases === 1
                ? '1 release calls itself'
                : `${m.releases} releases call themselves`}{' '}
              {m.name} ({m.language}); no edition of this series has that name.
            </Text>
            <Button
              size="compact-xs"
              variant="default"
              onClick={() =>
                onAddEdition({
                  editionName: m.name,
                  language: m.language,
                  volumes: m.volumes || undefined,
                })
              }
            >
              Add {m.name}
            </Button>
          </Group>
        ))}
      {notes.map((n) => (
        <Text key={n} size="xs" c="dimmed" className="wrapAnywhere">
          {n}
        </Text>
      ))}
      {result.done &&
        !result.stopped &&
        problems.length === 0 &&
        notes.length === 0 &&
        result.candidates.length === 0 && (
          <Text size="sm" c="dimmed">
            No releases found. Check that this series has an enabled indexer for {sel.ledger}s and
            that its source matches the correct title.
          </Text>
        )}
    </>
  )
}
