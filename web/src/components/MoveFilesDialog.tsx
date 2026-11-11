import { useState } from 'react'
import { Alert, Button, Checkbox, Group, Modal, Radio, Select, Stack, Text } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { notify } from '../ui'
import type {
  AssetCoverage,
  BookRef,
  CollectedVersion,
  Edition,
  File as SeriesFile,
} from '../types/store'
import type { BookStatus, ReviewFileChoice, ReviewPlanRequest } from '../types/service'
import { cardLabel, fileName } from './editionText'
import { QueryFeedback } from './QueryFeedback'
import { shortBook } from './useReviewChoices'
import classes from './FileDialog.module.css'

const key = (b: { versionId?: number; bookId?: number }) => `${b.versionId}:${b.bookId}`

// Give files to one edition at once: the same edition collected with
// another profile, or another edition, whose books the files then stand for
// by number. The server checks each file against the edition's profile, as
// it does for one.
export function MoveFilesDialog({
  seriesID,
  files,
  editions,
  versions,
  coverage,
  books,
  done,
  close,
}: {
  seriesID: number
  files: SeriesFile[]
  editions: Edition[]
  versions: CollectedVersion[]
  coverage: AssetCoverage[]
  books: BookStatus[]
  done: () => Promise<void>
  close: () => void
}) {
  const qc = useQueryClient()
  const [target, setTarget] = useState<string | null>(null)
  // Whether the editions using the files now go on using them.
  const [share, setShare] = useState(false)
  const only = !share
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const card = versions.find((t) => String(t.id) === target)
  const editionOf = (versionId: number) => versions.find((t) => t.id === versionId)?.editionId ?? 0
  const number = (bookId: number) => books.find((s) => s.book.id === bookId)?.book.sequenceStr
  const entries = (editionId: number, bookIds: number[]): BookRef[] =>
    versions
      .filter((t) => t.editionId === editionId)
      .flatMap((t) =>
        bookIds.map((bookId) => ({
          versionId: t.id,
          bookId,
          editionId,
          number: books.find((s) => s.book.id === bookId)?.book.sequenceNum ?? 0,
          ledger: editions.find((c) => c.id === editionId)?.type ?? 'volume',
        })),
      )
  // What each file holds now, by edition.
  const held = (f: SeriesFile) => {
    const pairs = [
      ...coverage
        .filter((a) => a.fileId === f.id)
        .map((a) => ({ editionId: editionOf(a.versionId), bookId: a.bookId })),
      ...(f.contentBooks ?? []).map((b) => ({ editionId: b.editionId, bookId: b.bookId ?? 0 })),
    ].filter((p) => p.editionId && p.bookId)
    const out = new Map<number, Set<number>>()
    for (const p of pairs) out.set(p.editionId, (out.get(p.editionId) ?? new Set()).add(p.bookId))
    return out
  }
  const request: ReviewPlanRequest | undefined = card && {
    versionId: card.id,
    files: files.map((f) => {
      const now = held(f)
      if (now.size === 0) return { fileId: f.id, books: null } // read from its name
      // In the card's own edition the books are the same; from another
      // edition they are the books of the same numbers.
      const same = now.get(card.editionId)
      const numbers = [...now.values()].flatMap((ids) => [...ids].map(number))
      const there =
        same ??
        new Set(
          books
            .filter(
              (s) => editionOf(s.versionId) === card.editionId && numbers.includes(s.book.sequenceStr),
            )
            .map((s) => s.book.id),
        )
      const kept = only
        ? []
        : [...now].filter(([id]) => id !== card.editionId).flatMap(([id, ids]) => entries(id, [...ids]))
      return { fileId: f.id, books: [...entries(card.editionId, [...there]), ...kept] }
    }),
  }
  const preview = useQuery({
    queryKey: ['series', seriesID, 'review-choices', 'move', request],
    queryFn: () => api.previewReviewChoices(seriesID, request!),
    enabled: !!request,
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
  })
  // One file per book of the card; the others stay as they are.
  const taken = new Set<string>()
  const plan = preview.data
  const rows = (plan?.files ?? []).map((f) => {
    const mine = f.books.filter((b) => b.book.versionId === card?.id)
    const usable = f.reason ? [] : mine.filter((b) => !b.reason && !taken.has(key(b.book)))
    usable.forEach((b) => taken.add(key(b.book)))
    const why =
      f.reason ||
      mine.find((b) => b.reason)?.reason ||
      (mine.length === 0
        ? 'This edition has no book of that number'
        : usable.length === 0
          ? 'Another selected file already supplies this book'
          : '')
    const others = only ? [] : f.books.filter((b) => b.book.versionId !== card?.id && b.currentSelected)
    const choice: ReviewFileChoice = {
      fileId: f.fileId,
      use: [...usable, ...others].map((b) => b.book),
      // A file the card cannot take is left exactly as it is.
      disposition: usable.length ? 'keep' : 'later',
    }
    return { file: f, usable, why, choice }
  })
  const moved = rows.filter((r) => r.usable.length > 0)
  const refused = rows.filter((r) => r.usable.length === 0)
  const apply = async () => {
    if (!plan || !request) return
    setBusy(true)
    setError('')
    try {
      const outcomes = await api.applyReviewChoices(seriesID, {
        request,
        token: plan.token,
        choices: rows.map((r) => r.choice),
      })
      await Promise.all(
        ['series', 'activity', 'review', 'queue', 'wanted'].map((k) =>
          qc.invalidateQueries({ queryKey: [k] }),
        ),
      )
      await done()
      const notes = outcomes.filter((o) => o.note)
      if (notes.length) notify.err(`${notes.length} files still need attention: ${notes[0].note}`)
      else notify.ok(`${moved.length} ${moved.length === 1 ? 'file is' : 'files are'} now used by ${cardName}`)
      close()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  const label = (t: CollectedVersion) =>
    cardLabel(editions.find((c) => c.id === t.editionId)!, t, versions)
  const cardName = card ? label(card) : ''
  const one = files.length === 1
  const ids = new Set(files.map((f) => f.id))
  const usedElsewhere = !!card && coverage.some((a) => ids.has(a.fileId) && a.versionId !== card.id)
  const onDisk = (t: CollectedVersion) => {
    const mine = books.filter((s) => s.versionId === t.id)
    return `${mine.filter((s) => s.fileIds.length > 0).length} of ${mine.length} books on disk`
  }
  return (
    <Modal
      opened
      onClose={() => !busy && close()}
      title={one ? 'Use this file for an edition' : `Use ${files.length} files for an edition`}
    >
      <Stack gap="md">
        <Text size="sm" c="dimmed">
          {one ? 'The file becomes' : 'Each file becomes'} the book of the same number in the
          edition you pick.
        </Text>
        {versions.length > 6 ? (
          <Select
            label="Edition"
            placeholder="Pick an edition"
            disabled={busy}
            data={versions.map((t) => ({ value: String(t.id), label: label(t) }))}
            value={target}
            onChange={setTarget}
          />
        ) : (
          <Radio.Group label="Edition" value={target} onChange={setTarget}>
            <Stack gap={10} mt={8}>
              {versions.map((t) => (
                <Radio
                  key={t.id}
                  value={String(t.id)}
                  disabled={busy}
                  label={label(t)}
                  description={onDisk(t)}
                />
              ))}
            </Stack>
          </Radio.Group>
        )}
        {usedElsewhere && (
          <Checkbox
            label={`The editions using ${one ? 'it' : 'them'} now keep using ${one ? 'it' : 'them'} too`}
            description={`Otherwise they stop using ${one ? 'it' : 'them'}.`}
            checked={share}
            disabled={busy}
            onChange={(e) => setShare(e.currentTarget.checked)}
          />
        )}
        {request && (
          <QueryFeedback
            pending={preview.isPending}
            error={preview.error}
            onRetry={() => void preview.refetch()}
          />
        )}
        {plan && card && (
          <div>
            {!one && (
              <Text size="sm" fw={500} role="status" mb={6}>
                {moved.length === rows.length
                  ? `All ${rows.length} files can go to ${cardName}`
                  : `${moved.length} of ${rows.length} files can go to ${cardName}`}
              </Text>
            )}
            <div className={classes.moves}>
              {/* What cannot move comes first: it is what needs a look. */}
              {[...refused, ...moved].map((r) => (
                <div key={r.file.fileId} className={classes.move}>
                  <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
                    {fileName(r.file.path)}
                  </Text>
                  <Text size="xs" c={r.usable.length ? 'dimmed' : 'orange'}>
                    {r.usable.length
                      ? `Becomes ${r.usable.map((b) => shortBook(b.label)).join(', ')}`
                      : `Stays as it is: ${r.why.charAt(0).toLowerCase()}${r.why.slice(1)}`}
                  </Text>
                </div>
              ))}
            </div>
          </div>
        )}
        {error && (
          <Alert color="red">
            {error}
            <Button size="xs" variant="subtle" onClick={() => void preview.refetch()}>
              Check again
            </Button>
          </Alert>
        )}
        <Group justify="flex-end" gap="sm">
          <Button variant="default" disabled={busy} onClick={close}>
            Cancel
          </Button>
          <Button loading={busy} disabled={!plan || moved.length === 0} onClick={() => void apply()}>
            {one
              ? 'Use file'
              : moved.length
                ? `Use ${moved.length} ${moved.length === 1 ? 'file' : 'files'}`
                : 'Use files'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
