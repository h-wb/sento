import { cardLabel, fileName } from './editionText'
import { useState } from 'react'
import { Alert, Button, Group, MultiSelect, Select, Stack, Text, TextInput } from '@mantine/core'
import { api } from '../api'
import type { Edition, CollectedVersion, File as SeriesFile } from '../types/store'
import type { BookStatus } from '../types/service'
import { EditorModal } from './EditorModal'
import { useEditor } from './useEditor'

export function MappingEditor({
  seriesID,
  value,
  versions,
  existing,
  proposal,
  books,
  editions,
  save,
  close,
}: {
  seriesID: number
  value: SeriesFile
  versions: CollectedVersion[]
  existing: import('../types/store').AssetCoverage[]
  proposal?: import('../types/store').ReviewProposal
  books: BookStatus[]
  editions: Edition[]
  save: (books: import('../types/store').BookRef[]) => Promise<void>
  close: () => void
}) {
  const mapped = existing.map((a) => ({
    versionId: a.versionId,
    bookId: a.bookId,
    editionId: versions.find((t) => t.id === a.versionId)?.editionId ?? 0,
    number: books.find((s) => s.book.id === a.bookId)?.book.sequenceNum ?? 0,
    ledger:
      editions.find((c) => c.id === versions.find((t) => t.id === a.versionId)?.editionId)
        ?.type ?? 'volume',
  }))
  // An unmapped file starts from review's proposal; saving accepts it.
  const initial =
    mapped.length || !proposal
      ? mapped
      : proposal.books.map((b) => ({
          versionId: b.versionId ?? 0,
          bookId: b.bookId ?? 0,
          editionId: b.editionId,
          number: b.number,
          ledger: b.ledger,
        }))
  const [draft, setDraft] = useState(initial)
  const [version, setVersion] = useState<string | null>(
    existing[0]
      ? String(existing[0].versionId)
      : proposal?.books[0]
        ? String(proposal.books[0].versionId)
        : null,
  )
  const edit = useEditor()
  const [language, setLanguage] = useState(value.facts?.language ?? '')
  const [group, setGroup] = useState(value.facts?.group ?? '')
  const t = versions.find((t) => String(t.id) === version)
  const chosen = draft.filter((s) => s.versionId === t?.id).map((s) => String(s.bookId))
  return (
    <EditorModal
      dirty={
        JSON.stringify(draft) !== JSON.stringify(initial) ||
        language !== (value.facts?.language ?? '') ||
        group !== (value.facts?.group ?? '')
      }
      busy={edit.busy}
      onClose={close}
      title="Match imported file"
    >
      <Stack>
        <div>
          <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>
            {value.facts?.title || fileName(value.path)}
          </Text>
          <details>
            <summary style={{ fontSize: 'var(--mantine-font-size-xs)', cursor: 'pointer' }}>
              Stored at
            </summary>
            <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
              {value.path}
            </Text>
          </details>
        </div>
        <Text size="sm">
          Confirm the edition and books this archive contains. This confirmation overrides parsed
          language for coverage. Group requirements still apply. A pack can cover several books.
          Existing mappings to other versions are kept.
        </Text>
        {proposal && !existing.length && (
          <Alert variant="light" p="xs">
            <Text size="sm">
              Proposed: <b>{proposal.label}</b> of {proposal.edition} ({proposal.evidence}).
              Save to accept, or change it below.
            </Text>
          </Alert>
        )}
        {value.facts && (
          <Text size="xs" c="dimmed">
            {[
              value.facts.language || 'Language unknown',
              value.facts.group || 'Group unknown',
              value.facts.format,
            ]
              .filter(Boolean)
              .join(' · ')}
          </Text>
        )}
        <Text size="xs" c="dimmed">
          Release advertised:{' '}
          {[value.claims?.language, value.claims?.group].filter(Boolean).join(' · ') ||
            'No claims recorded'}
        </Text>
        <Group grow>
          <TextInput
            label="Confirmed language"
            value={language}
            onChange={(e) => setLanguage(e.currentTarget.value)}
          />
          <TextInput
            label="Confirmed release group"
            value={group}
            onChange={(e) => setGroup(e.currentTarget.value)}
          />
        </Group>
        <Select
          label="Edition"
          data={versions.map((t) => ({
            value: String(t.id),
            label: cardLabel(editions.find((c) => c.id === t.editionId)!, t, versions),
          }))}
          value={version}
          onChange={setVersion}
        />
        <MultiSelect
          label="Books in this file"
          searchable
          disabled={!t}
          data={books
            .filter((s) => s.versionId === t?.id)
            .map((s) => ({ value: String(s.book.id), label: s.book.sequenceStr }))}
          value={chosen}
          onChange={(ids) => {
            if (!t) return
            setDraft([
              ...draft.filter((s) => s.versionId !== t.id),
              ...ids.map((id) => ({
                versionId: t.id,
                bookId: Number(id),
                editionId: t.editionId,
                number: books.find((s) => s.book.id === Number(id))?.book.sequenceNum ?? 0,
                ledger: editions.find((c) => c.id === t.editionId)?.type ?? 'volume',
              })),
            ])
          }}
        />
        <Stack gap={2}>
          {draft.map((s) => (
            <Text size="xs" key={`${s.versionId}-${s.bookId}`}>
              {versions.find((t) => t.id === s.versionId)?.name} ·{' '}
              {books.find((u) => u.book.id === s.bookId)?.book.sequenceStr}
            </Text>
          ))}
        </Stack>
        {draft.length === 0 && (
          <Text size="sm" c="dimmed">
            This file will stay in the library without claiming ownership of any book.
          </Text>
        )}
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Group justify="space-between">
          <Button
            variant="subtle"
            color="red"
            disabled={edit.busy || draft.length === 0}
            onClick={() => setDraft([])}
          >
            Clear mappings
          </Button>
          <Button
            loading={edit.busy}
            onClick={() =>
              void edit.run(async () => {
                if (
                  language !== (value.facts?.language ?? '') ||
                  group !== (value.facts?.group ?? '')
                )
                  await api.correctFileFacts(seriesID, value.id, { language, group })
                await save(draft)
              })
            }
          >
            Save coverage
          </Button>
        </Group>
      </Stack>
    </EditorModal>
  )
}
