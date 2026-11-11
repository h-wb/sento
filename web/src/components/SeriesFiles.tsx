import { useState } from 'react'
import {
  Badge,
  Button,
  Checkbox,
  Group,
  Pagination,
  Stack,
  Table,
  Text,
  TextInput,
} from '@mantine/core'
import { fmtSize } from '../api'
import type {
  AssetCoverage,
  CollectedVersion,
  Edition,
  File as SeriesFile,
} from '../types/store'
import type { BookStatus } from '../types/service'
import { BulkFactsDialog } from './BulkFactsDialog'
import { MoveFilesDialog } from './MoveFilesDialog'
import { cardTitle, fileName } from './editionText'
import { fileOrigin, flagLabels } from './factFlags'

const pageSize = 20

// A series' files as a table: what each is used for and what it is, with
// the actions that apply to one file or to a selection.
export default function SeriesFiles({
  seriesID,
  files,
  editions,
  versions,
  coverage,
  books,
  onOpen,
  changed,
}: {
  seriesID: number
  files: SeriesFile[]
  editions: Edition[]
  versions: CollectedVersion[]
  coverage?: AssetCoverage[]
  // Every card's books, once loaded.
  books?: BookStatus[]
  onOpen: (f: SeriesFile) => void
  changed: () => Promise<void>
}) {
  const [filter, setFilter] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulk, setBulk] = useState<'facts' | 'move' | null>(null)

  const review = (f: SeriesFile) =>
    f.needsReview ||
    (f.unrequested && !f.reviewAction) ||
    !!coverage?.some((a) => a.fileId === f.id && !a.verified)
  // "Vol. 2 · Standard Edition and Standard Edition · Colored"
  const usedFor = (f: SeriesFile) => {
    if (f.reviewAction === 'trash') return 'Trash pending'
    if (review(f)) return 'Needs review'
    if (!books) return ''
    const uses = books.filter((b) => b.fileIds.includes(f.id))
    if (uses.length === 0) return 'Not used by any edition'
    const numbers = [...new Set(uses.map((b) => b.book.sequenceStr))]
    const cards = [...new Set(uses.map((b) => b.versionId))].flatMap((id) => {
      const t = versions.find((v) => v.id === id)
      const c = t && editions.find((e) => e.id === t.editionId)
      return t && c ? [{ title: cardTitle(c, t, versions), chapter: c.type === 'chapter' }] : []
    })
    const kind = cards.some((c) => c.chapter) ? 'Ch.' : 'Vol.'
    const held = numbers.length > 3 ? `${numbers.length} books` : `${kind} ${numbers.join(', ')}`
    return `${held} · ${cards.map((c) => c.title).join(' and ')}`
  }
  const list = files
    .filter((f) => f.path.toLowerCase().includes(filter.toLowerCase()))
    .sort((a, b) => Number(review(b)) - Number(review(a)))
  const shown = list.slice((page - 1) * pageSize, page * pageSize)
  const picked = files.filter((f) => selected.has(f.id))
  const toggle = (id: number) =>
    setSelected((v) => {
      const next = new Set(v)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  const all = list.length > 0 && list.every((f) => selected.has(f.id))
  const done = async () => {
    await changed()
    setSelected(new Set())
  }
  return (
    <Stack gap="sm">
      <Group justify="space-between" gap="xs">
        <TextInput
          placeholder="Find a file"
          aria-label="Find a file"
          size="xs"
          maw={320}
          value={filter}
          onChange={(e) => {
            setFilter(e.currentTarget.value)
            setPage(1)
          }}
        />
        {picked.length > 0 && (
          <Group gap="xs">
            <Text size="sm">{picked.length} selected</Text>
            <Button size="xs" variant="default" onClick={() => setBulk('facts')}>
              Correct what they are…
            </Button>
            <Button size="xs" variant="default" disabled={!coverage || !books} onClick={() => setBulk('move')}>
              Use for an edition…
            </Button>
            <Button size="xs" variant="subtle" onClick={() => setSelected(new Set())}>
              Clear
            </Button>
          </Group>
        )}
      </Group>
      {list.length === 0 ? (
        <Text size="sm" c="dimmed">
          No files match.
        </Text>
      ) : (
        <Table.ScrollContainer minWidth={640}>
          <Table verticalSpacing="xs" highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th w={36}>
                  <Checkbox
                    aria-label="Select all files"
                    checked={all}
                    indeterminate={!all && list.some((f) => selected.has(f.id))}
                    onChange={() => setSelected(all ? new Set() : new Set(list.map((f) => f.id)))}
                  />
                </Table.Th>
                <Table.Th>File</Table.Th>
                <Table.Th>Used for</Table.Th>
                <Table.Th>What it is</Table.Th>
                <Table.Th ta="right">Size</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {shown.map((f) => {
                const name = fileName(f.path)
                const facts = [
                  f.facts?.language || f.claims?.language,
                  f.facts?.group || f.claims?.group,
                ].filter(Boolean)
                return (
                  <Table.Tr key={f.id}>
                    <Table.Td>
                      <Checkbox
                        aria-label={`Select ${name}`}
                        checked={selected.has(f.id)}
                        onChange={() => toggle(f.id)}
                      />
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" title={f.path} style={{ overflowWrap: 'anywhere' }}>
                        {name}
                      </Text>
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        {fileOrigin(f)}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" c={review(f) ? 'yellow' : undefined}>
                        {usedFor(f)}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" c={facts.length ? undefined : 'dimmed'}>
                        {facts.join(' · ') || 'Not known'}
                      </Text>
                      <Group gap={4} mt={2}>
                        {flagLabels(f).map((l) => (
                          <Badge key={l} size="xs" variant="light" color="gray">
                            {l}
                          </Badge>
                        ))}
                      </Group>
                    </Table.Td>
                    <Table.Td ta="right">
                      <Text size="sm" style={{ whiteSpace: 'nowrap' }}>
                        {fmtSize(f.size)}
                      </Text>
                    </Table.Td>
                    <Table.Td ta="right">
                      <Button size="xs" variant="default" onClick={() => onOpen(f)}>
                        {review(f) ? 'Match file' : 'Edit file'}
                      </Button>
                    </Table.Td>
                  </Table.Tr>
                )
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
      {list.length > pageSize && (
        <Pagination
          size="sm"
          total={Math.ceil(list.length / pageSize)}
          value={page}
          onChange={setPage}
        />
      )}
      {bulk === 'facts' && (
        <BulkFactsDialog seriesID={seriesID} files={picked} done={done} close={() => setBulk(null)} />
      )}
      {bulk === 'move' && coverage && books && (
        <MoveFilesDialog
          seriesID={seriesID}
          files={picked}
          editions={editions}
          versions={versions}
          coverage={coverage}
          books={books}
          done={done}
          close={() => setBulk(null)}
        />
      )}
    </Stack>
  )
}
