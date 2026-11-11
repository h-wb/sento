import { useState } from 'react'
import { Alert, Button, Divider, Group, MultiSelect, Select, Stack, Text } from '@mantine/core'
import type {
  AssetCoverage,
  CollectedVersion,
  Edition,
  File as SeriesFile,
  FileHint,
  ReviewProposal,
} from '../types/store'
import type { BookStatus } from '../types/service'
import { fileName } from './editionText'
import { EditorModal } from './EditorModal'
import { FileFacts } from './FileFacts'
import { factFlags, fileOrigin, hasFlag } from './factFlags'
import { ReviewChoices } from './ReviewChoices'

type Entry = { versionId: number; bookId: number; editionId: number; number: number; ledger: string }

// One file, in three questions: which books it holds, what it is, and which
// cards use it. A file waiting in review is matched here; a matched one is
// corrected or moved to another edition or card.
export function MappingEditor({
  seriesID,
  value,
  versions,
  existing,
  proposal,
  hint,
  books,
  editions,
  applied,
  close,
}: {
  seriesID: number
  value: SeriesFile
  versions: CollectedVersion[]
  existing: AssetCoverage[]
  proposal?: ReviewProposal
  hint?: FileHint
  books: BookStatus[]
  editions: Edition[]
  applied: () => Promise<void>
  close: () => void
}) {
  const editionOf = (versionId: number) => versions.find((t) => t.id === versionId)?.editionId ?? 0
  const ledgerOf = (editionId: number) =>
    editions.find((c) => c.id === editionId)?.type ?? 'volume'
  // Every card of an edition gets a say on each of its books the file
  // holds, so "which cards use it" lists them all, used or not.
  const forEveryCard = (editionId: number, bookIds: number[]): Entry[] =>
    versions
      .filter((t) => t.editionId === editionId)
      .flatMap((t) =>
        bookIds.map((bookId) => ({
          versionId: t.id,
          bookId,
          editionId,
          number: books.find((s) => s.book.id === bookId)?.book.sequenceNum ?? 0,
          ledger: ledgerOf(editionId),
        })),
      )
  const byEdition = (pairs: { editionId: number; bookId: number }[]): Entry[] =>
    [...new Set(pairs.map((p) => p.editionId))].flatMap((editionId) =>
      forEveryCard(editionId, [
        ...new Set(pairs.filter((p) => p.editionId === editionId).map((p) => p.bookId)),
      ]),
    )

  // What the file is known to hold: the books cards use it for, and the
  // contents confirmed without a card using them.
  const known = [
    ...existing.map((a) => ({ editionId: editionOf(a.versionId), bookId: a.bookId })),
    ...(value.contentBooks ?? []).map((b) => ({ editionId: b.editionId, bookId: b.bookId ?? 0 })),
  ].filter((p) => p.editionId && p.bookId)
  const proposed = (proposal?.books ?? []).map((b) => ({
    editionId: b.editionId,
    bookId: b.bookId ?? 0,
  }))
  const [initial] = useState<Entry[]>(() => byEdition(known.length ? known : proposed))
  const [draft, setDraft] = useState(initial)
  const [editionId, setEditionId] = useState<number | null>(initial[0]?.editionId ?? null)
  const [busy, setBusy] = useState(false)
  const [choicesDirty, setChoicesDirty] = useState(false)

  // The release the file came from says its language and group even when
  // the file's own name does not: start from those.
  const read = (key: 'language' | 'group') =>
    value.facts?.sources?.[key] === 'manual'
      ? (value.facts[key] ?? '')
      : value.facts?.[key] || value.claims?.[key] || ''
  const [language, setLanguage] = useState(read('language'))
  const [group, setGroup] = useState(read('group'))
  const [flags, setFlags] = useState<Record<string, boolean>>({})
  // What it is matters when a profile judges it wrong; until then one line
  // says what was read.
  const [factsOpen, setFactsOpen] = useState(false)
  const readAs = [
    language || 'language unknown',
    group || 'group unknown',
    ...factFlags.filter((f) => flags[f.key] ?? hasFlag(value, f.key)).map((f) => f.label),
  ].join(' · ')

  // Numbers the file's own name carries, shown until an edition is picked;
  // picking one maps them onto its books, so no number is typed twice.
  const [pending, setPending] = useState<string[]>(hint?.numbers ?? [])
  const [bookSearch, setBookSearch] = useState('')
  const booksOf = (id: number) => {
    const seen = new Set<number>()
    return books.filter(
      (s) => editionOf(s.versionId) === id && !seen.has(s.book.id) && seen.add(s.book.id),
    )
  }
  const chosen = [...new Set(draft.filter((s) => s.editionId === editionId).map((s) => s.bookId))]
  const setBooks = (id: number, bookIds: number[]) =>
    setDraft((d) => [
      // Another edition the file also holds books of stays as it is.
      ...d.filter((s) => s.editionId !== id && s.editionId !== editionId),
      ...forEveryCard(id, bookIds),
    ])
  const pickEdition = (id: number) => {
    const numbers = editionId
      ? chosen.map((bookId) => books.find((s) => s.book.id === bookId)?.book.sequenceStr ?? '')
      : pending
    setBooks(
      id,
      booksOf(id)
        .filter((s) => numbers.includes(s.book.sequenceStr))
        .map((s) => s.book.id),
    )
    setEditionId(id)
  }
  const changed = JSON.stringify(draft) !== JSON.stringify(initial)
  // Profiles may not test language, so a slip here would pass silently.
  const editionLanguage = editions.find((c) => c.id === editionId)?.language ?? ''
  const otherLanguage =
    !!language && !!editionLanguage && language.toLowerCase() !== editionLanguage.toLowerCase()
  const collected = editions.filter((c) => versions.some((t) => t.editionId === c.id))
  return (
    <EditorModal
      dirty={
        choicesDirty ||
        changed ||
        language !== read('language') ||
        group !== read('group') ||
        Object.keys(flags).length > 0
      }
      busy={busy}
      onClose={close}
      title={value.needsReview ? 'Match imported file' : 'File'}
    >
      <Stack>
        <div>
          <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>
            {fileName(value.path)}
          </Text>
          <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
            {fileOrigin(value)}
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
        {proposal && !known.length && (
          <Alert variant="light" p="xs">
            <Text size="sm">
              Proposed: <b>{proposal.label}</b> of {proposal.edition} ({proposal.evidence}). Check
              the books, then the cards that should use the file.
            </Text>
          </Alert>
        )}

        <Divider label="What’s in it" labelPosition="left" />
        <Select
          label="Edition"
          description="The edition whose books this file holds. Pick another to move the file there."
          disabled={busy}
          allowDeselect={false}
          data={collected.map((c) => ({
            value: String(c.id),
            label: [c.language, c.editionName].filter(Boolean).join(' · '),
          }))}
          value={editionId ? String(editionId) : null}
          onChange={(v) => v && pickEdition(Number(v))}
        />
        <MultiSelect
          label="Books in this file"
          description={
            editionId
              ? booksOf(editionId).length > 20
                ? 'Type a range such as 1-30 and press Enter to add every book in it.'
                : undefined
              : pending.length
                ? `Its name says ${pending.join(', ')}. Pick the edition to match them.`
                : 'Pick the edition first.'
          }
          searchable
          searchValue={bookSearch}
          onSearchChange={setBookSearch}
          onKeyDown={(e) => {
            const range = /^\s*(\d+(?:\.\d+)?)\s*[-–]\s*(\d+(?:\.\d+)?)\s*$/.exec(bookSearch)
            if (e.key !== 'Enter' || !range || !editionId) return
            e.preventDefault()
            const [low, high] = [Number(range[1]), Number(range[2])].sort((a, b) => a - b)
            const inRange = booksOf(editionId)
              .filter((s) => s.book.numberKnown && s.book.sequenceNum >= low * 100 && s.book.sequenceNum <= high * 100)
              .map((s) => s.book.id)
            setBooks(editionId, [...new Set([...chosen, ...inRange])])
            setBookSearch('')
          }}
          disabled={busy || !editionId}
          data={
            editionId
              ? booksOf(editionId).map((s) => ({
                  value: String(s.book.id),
                  label: s.book.sequenceStr,
                }))
              : pending.map((n) => ({ value: n, label: n }))
          }
          value={editionId ? chosen.map(String) : pending}
          onChange={(ids) => (editionId ? setBooks(editionId, ids.map(Number)) : setPending(ids))}
        />
        {otherLanguage && (
          <Text size="xs" c="orange">
            The file reads as {language}, but this edition is {editionLanguage}. Check the edition,
            or correct the language below if it was read wrong.
          </Text>
        )}

        <Divider label="What it is" labelPosition="left" />
        {factsOpen ? (
          <FileFacts
            file={value}
            language={language}
            group={group}
            flags={flags}
            disabled={busy}
            onLanguage={setLanguage}
            onGroup={setGroup}
            onFlags={setFlags}
          />
        ) : (
          <Group justify="space-between" wrap="nowrap" gap="sm">
            <Text size="sm" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
              Read as {readAs}
            </Text>
            <Button size="compact-sm" variant="default" onClick={() => setFactsOpen(true)}>
              Correct…
            </Button>
          </Group>
        )}

        <Divider label="Which cards use it" labelPosition="left" />
        <ReviewChoices
          seriesID={seriesID}
          request={{
            files: [
              {
                fileId: value.id,
                books: draft.length || initial.length || changed ? draft : null,
                ...(language !== (value.facts?.language ?? '') ? { language } : {}),
                ...(group !== (value.facts?.group ?? '') ? { group } : {}),
                ...(Object.keys(flags).length ? { flags } : {}),
              },
            ],
          }}
          // A matched file changes hands only where a person says so.
          keepCurrent={!value.needsReview && !changed}
          single
          applied={applied}
          onDirtyChange={setChoicesDirty}
          onBusyChange={setBusy}
        />
      </Stack>
    </EditorModal>
  )
}
