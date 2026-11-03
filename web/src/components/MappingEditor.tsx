import { cardLabel, fileName } from './editionText'
import { useState } from 'react'
import { Alert, Group, MultiSelect, Select, Stack, Text, TextInput } from '@mantine/core'
import type { Edition, CollectedVersion, File as SeriesFile, FileHint } from '../types/store'
import type { BookStatus } from '../types/service'
import { EditorModal } from './EditorModal'
import { ReviewChoices } from './ReviewChoices'
import { FileScoreDetails } from './FileScoreDetails'

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
  existing: import('../types/store').AssetCoverage[]
  proposal?: import('../types/store').ReviewProposal
  hint?: FileHint
  books: BookStatus[]
  editions: Edition[]
  applied: () => Promise<void>
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
  // Confirmed contents remain editable even when this copy is unused.
  const confirmed = (value.contentBooks ?? []).flatMap((b) => {
    const t = versions.find((t) => t.editionId === b.editionId && t.isDefault)
    return t && !mapped.some((a) => a.bookId === b.bookId) ? [{ ...b, versionId: t.id }] : []
  })
  const initial = mapped.length || confirmed.length
    ? [...mapped, ...confirmed]
    : (proposal?.books ?? []).map((b) => ({
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
        : confirmed[0] ? String(confirmed[0].versionId) : null,
  )
  const [busy, setBusy] = useState(false)
  const [choicesDirty, setChoicesDirty] = useState(false)
  // The release the file came from advertises language and group even
  // when the file's own name does not say them: prefill from the claims
  // so review only has to confirm. Saving records the confirmed values.
  const initialLanguage = value.facts?.sources?.language === 'manual' ? value.facts.language ?? '' : value.facts?.language || value.claims?.language || ''
  const initialGroup = value.facts?.sources?.group === 'manual' ? value.facts.group ?? '' : value.facts?.group || value.claims?.group || ''
  const [language, setLanguage] = useState(initialLanguage)
  const [group, setGroup] = useState(initialGroup)
  // Numbers the file's own name carries. Shown greyed until an edition is
  // picked; picking one maps them onto that edition's books, so the number
  // never has to be retyped.
  const [pending, setPending] = useState<string[]>(hint?.numbers ?? [])
  const t = versions.find((t) => String(t.id) === version)
  const chosen = draft.filter((s) => s.versionId === t?.id).map((s) => String(s.bookId))
  const pickBooks = (versionId: number, numbers: string[]) => {
    const tv = versions.find((x) => x.id === versionId)
    if (!tv) return
    const matching = books.filter(
      (s) => s.versionId === tv.id && numbers.includes(s.book.sequenceStr),
    )
    if (matching.length === 0) return
    setDraft((d) => [
      ...d.filter((s) => s.versionId !== tv.id),
      ...matching.map((s) => ({
        versionId: tv.id,
        bookId: s.book.id,
        editionId: tv.editionId,
        number: s.book.sequenceNum,
        ledger: editions.find((c) => c.id === tv.editionId)?.type ?? 'volume',
      })),
    ])
  }
  return (
    <EditorModal
      dirty={
        choicesDirty || JSON.stringify(draft) !== JSON.stringify(initial) ||
        language !== initialLanguage ||
        group !== initialGroup
      }
      busy={busy}
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
              Confirm the contents, then choose which copies to keep below.
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
            disabled={busy}
            value={language}
            onChange={(e) => setLanguage(e.currentTarget.value)}
          />
          <TextInput
            label="Confirmed release group"
            disabled={busy}
            value={group}
            onChange={(e) => setGroup(e.currentTarget.value)}
          />
        </Group>
        <Select
          label="Edition"
          disabled={busy}
          data={versions.map((t) => ({
            value: String(t.id),
            label: cardLabel(editions.find((c) => c.id === t.editionId)!, t, versions),
          }))}
          value={version}
          onChange={(v) => {
            setVersion(v)
            if (!v) return
            // Without a previous edition the numbers come from the file's
            // own name; otherwise carry the current selection over.
            const nums = t
              ? chosen.map((id) => books.find((s) => s.book.id === Number(id))?.book.sequenceStr ?? '')
              : pending
            pickBooks(Number(v), nums)
          }}
        />
        {t ? (
          <>
            <FileScoreDetails seriesID={seriesID} versionID={t.id} fileID={value.id} />
            {(language !== initialLanguage || group !== initialGroup) && (
              <Text size="xs" c="dimmed">The score uses saved facts. Save your corrections to update it.</Text>
            )}
          </>
        ) : <Text size="xs" c="dimmed">Choose an edition to see this file’s current score.</Text>}
        <MultiSelect
          label="Books in this file"
          searchable
          disabled={busy || !t}
          data={
            t
              ? books
                  .filter((s) => s.versionId === t.id)
                  .map((s) => ({ value: String(s.book.id), label: s.book.sequenceStr }))
              : pending.map((n) => ({ value: n, label: n }))
          }
          value={t ? chosen : pending}
          onChange={(ids) => {
            if (!t) {
              setPending(ids)
              return
            }
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
        <ReviewChoices
          seriesID={seriesID}
          request={{ files: [{
            fileId: value.id,
            books: draft.length || initial.length || JSON.stringify(draft) !== JSON.stringify(initial) ? draft : null,
            ...(language !== (value.facts?.language ?? '') ? { language } : {}),
            ...(group !== (value.facts?.group ?? '') ? { group } : {}),
          }] }}
          applied={applied}
          onDirtyChange={setChoicesDirty}
          onBusyChange={setBusy}
        />
      </Stack>
    </EditorModal>
  )
}
