import { useEffect, useState } from 'react'
import { Alert, Badge, Button, Card, Checkbox, Group, Pagination, Select, Stack, Text, UnstyledButton } from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { notify } from '../ui'
import type { FileScore, ReviewBookPlan, ReviewPlan, ReviewPlanRequest, ReviewFileChoice, ReviewFileOutcome, ReviewFilePlan } from '../types/service'
import { QueryFeedback } from './QueryFeedback'
import { fileName } from './editionText'

type Props = {
  seriesID: number
  request: ReviewPlanRequest
  applied: () => Promise<void>
  // Start from the cards that use the file now, proposing no new use: for
  // a file that is already matched.
  keepCurrent?: boolean
  // One file, in its own dialog: the wording speaks of the file and its
  // cards instead of a batch of incoming archives.
  single?: boolean
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
}

const pageSize = 25

const bookKey = (b: { versionId?: number; bookId?: number }) => `${b.versionId}:${b.bookId}`

function initialChoices(plan: ReviewPlan, request: ReviewPlanRequest, missingOnly = false, keepCurrent = false): ReviewFileChoice[] {
  const selected = new Set<string>()
  return plan.files.map((f, i) => {
    const disposition = (f.reason && request.files[i].books === null) || (f.books.length > 0 && f.books.every((b) => !b.currentSelected && (!!b.reason || !b.incoming.factsKnown))) ? 'later' : 'keep'
    return {
      fileId: f.fileId, disposition,
      use: f.books.filter((b) => {
        const wanted = keepCurrent ? b.currentSelected : missingOnly ? b.currentSelected || (!b.reason && b.incoming.factsKnown && !b.owners.some((o) => o.present)) : b.useIncoming
        if (!wanted || f.reason || disposition === 'later' || selected.has(bookKey(b.book))) return false
        selected.add(bookKey(b.book))
        return true
      }).map((b) => b.book),
    }
  })
}

function Score({ value }: { value: FileScore }) {
  return (
    <details>
      <summary style={{ fontSize: 'var(--mantine-font-size-xs)', cursor: 'pointer' }}>
        Score {value.score} · {value.profileName || 'No profile'}
      </summary>
      <Text size="xs" c="dimmed">
        {value.formats.map((f) => `${f.name}: ${f.score > 0 ? '+' : ''}${f.score}`).join(' · ') || 'No custom formats matched.'}
      </Text>
      {!value.factsKnown && <Text size="xs" c="orange">File facts are unknown; choose explicitly after checking the archive.</Text>}
    </details>
  )
}

export function ReviewChoices(props: Props) {
  const preview = useQuery({
    queryKey: ['series', props.seriesID, 'review-choices', props.request],
    queryFn: () => api.previewReviewChoices(props.seriesID, props.request),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  if (!preview.data || preview.isFetching) return (
    <Stack gap="sm">
      <Text size="sm" c="dimmed">
        Checking {props.request.files.length} incoming {props.request.files.length === 1 ? 'archive' : 'archives'} — a large pack takes a moment the first time.
      </Text>
      <QueryFeedback pending={preview.isPending || preview.isFetching} error={preview.error} onRetry={() => void preview.refetch()} />
    </Stack>
  )
  return <Choices key={preview.data.token} {...props} plan={preview.data} refresh={() => preview.refetch()} />
}

function Choices({ seriesID, request, applied, keepCurrent, single, onDirtyChange, onBusyChange, plan, refresh }: Props & { plan: ReviewPlan; refresh: () => Promise<unknown> }) {
  const defaults = initialChoices(plan, request, false, keepCurrent)
  // With several cards in play, every choice says which card it is for.
  const severalCards = new Set(plan.files.flatMap((f) => f.books.map((b) => b.card))).size > 1
  const forCard = (b: ReviewBookPlan) => (severalCards ? `${b.label} — ${b.card}` : b.label)
  const [choices, setChoices] = useState(defaults)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [page, setPage] = useState(1)
  const [receipt, setReceipt] = useState<ReviewFileOutcome[] | null>(null)
  // Big packs open as one-line summaries; a single page or the one-file
  // editor opens expanded, where every choice is already on screen.
  const [collapsed, setCollapsed] = useState<Set<number>>(
    () => new Set(plan.files.length > pageSize ? plan.files.map((f) => f.fileId) : []),
  )
  const qc = useQueryClient()
  const dirty = JSON.stringify(choices) !== JSON.stringify(defaults)
  useEffect(() => { onDirtyChange?.(dirty) }, [dirty, onDirtyChange])
  useEffect(() => { onBusyChange?.(busy) }, [busy, onBusyChange])
  const using = (i: number, b: { versionId?: number; bookId?: number }) => choices[i].use.some((v) => bookKey(v) === bookKey(b))
  const update = (i: number, change: Partial<ReviewFileChoice>) =>
    setChoices((v) => v.map((c, j) => i === j ? { ...c, ...change } : c))
  const selectBook = (i: number, b: ReviewBookPlan, checked: boolean) => setChoices((before) => before.map((c, j) => {
    const use = c.use.filter((v) => bookKey(v) !== bookKey(b.book))
    if (i === j) return { ...c, use: checked ? [...use, b.book] : use, disposition: 'keep' }
    return checked ? { ...c, use } : c
  }))
  let imports = 0, replacements = 0, kept = 0, missing = 0
  const outcomes = new Map<string, { current: Set<number>; chosen: Set<number>; reviewed: Set<number> }>()
  plan.files.forEach((f, i) => {
    f.books.forEach((b) => {
      const key = bookKey(b.book)
      const outcome = outcomes.get(key) ?? { current: new Set<number>(), chosen: new Set<number>(), reviewed: new Set<number>() }
      if (b.currentSelected) outcome.current.add(f.fileId)
      b.owners.filter((o) => o.present).forEach((o) => outcome.current.add(o.fileId))
      if (choices[i].disposition !== 'later') {
        outcome.reviewed.add(f.fileId)
        if (using(i, b.book)) outcome.chosen.add(f.fileId)
      }
      outcomes.set(key, outcome)
    })
  })
  outcomes.forEach((b) => {
    if (b.chosen.size) {
      if ([...b.chosen].some((id) => b.current.has(id))) kept++
      else if (b.current.size) replacements++
      else imports++
    } else if ([...b.current].some((id) => !b.reviewed.has(id))) kept++
    else missing++
  })
  const replacementChosen = (book: ReviewBookPlan['book']) => {
    const outcome = outcomes.get(bookKey(book))
    return !!outcome && [...outcome.chosen].some((id) => !outcome.current.has(id))
  }
  const trash = choices.filter((c) => c.disposition === 'trash').length
  const later = choices.filter((c) => c.disposition === 'later').length
  // One line per archive: what it would supply, and what happens to its
  // bytes after review. Shown on the collapsed card as well.
  const cardSummary = (f: ReviewFilePlan, i: number, withReason: boolean) => {
    const parts: string[] = []
    for (const b of f.books) {
      if (!using(i, b.book)) continue
      parts.push(b.currentSelected ? `Keep ${forCard(b)}` : b.owners.some((o) => o.present) ? `Use incoming instead for ${forCard(b)}` : `Import ${forCard(b)}`)
    }
    const d = choices[i].disposition
    parts.push(d === 'trash' ? 'Move to trash' : d === 'later' ? 'Leave for later' : choices[i].use.length ? 'Keep archive' : 'Keep unused file')
    if (withReason && f.reason) parts.push(f.reason)
    return parts.join(' · ')
  }
  const toggle = (fileId: number) => setCollapsed((v) => {
    const next = new Set(v)
    if (next.has(fileId)) next.delete(fileId)
    else next.add(fileId)
    return next
  })
  const apply = async () => {
    setBusy(true)
    setError('')
    try {
      const outcomes = await api.applyReviewChoices(seriesID, { request, token: plan.token, choices })
      await Promise.all(['series', 'activity', 'review', 'queue', 'wanted'].map((k) => qc.invalidateQueries({
        queryKey: [k],
        // Keep the open editor's snapshot until the saved receipt is dismissed.
        predicate: (q) => !q.queryKey.includes('review-choices') && !q.queryKey.includes('file-review'),
      })))
      const warnings = outcomes.filter((f) => f.note)
      if (warnings.length) {
        setReceipt(outcomes)
        notify.err('Choices saved. Some file operations still need attention; trash moves will retry automatically.')
        return
      }
      notify.ok(`Imported ${imports}; kept existing ${kept}; replaced ${replacements}; moved ${trash} to trash; still missing ${missing}; left for later ${later}`)
      await applied()
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  if (receipt) return (
    <Stack gap="sm">
      <Text size="sm" fw={500}>Choices saved</Text>
      {receipt.map((f, i) => <Text key={`${f.fileId}-${i}`} size="sm" style={{ overflowWrap: 'anywhere' }}>
        {fileName(plan.files.find((p) => p.fileId === f.fileId)?.path ?? '')}: {{ imported: 'Imported', kept: 'Kept archive', trashed: 'Moved to trash', trash_pending: 'Trash move pending', later: 'Left for later', settlement_pending: 'Finishing download pending' }[f.status] ?? f.status}{f.note && ` · ${f.note}`}
      </Text>)}
      <Text size="xs" c="dimmed">Pending trash moves will retry automatically. Archives already imported or retained keep their saved choices.</Text>
      <Button onClick={() => void applied()}>Done</Button>
    </Stack>
  )
  const from = (page - 1) * pageSize + 1
  const to = Math.min(page * pageSize, plan.files.length)
  return (
    <Stack gap="sm">
      <Text size="sm">{single ? 'Tick the cards that should use this file. A card keeps the copy it has unless you choose this one instead.' : 'Choose which copies to use. Existing copies stay selected unless you explicitly choose a replacement.'}</Text>
      <Text size="sm" fw={500} role="status">Import {imports} · Keep existing {kept} · Replace {replacements} · Trash {trash} · Still missing {missing} · Left for later {later}</Text>
      {plan.files.length > 1 && <Group gap="xs">
        <Button size="compact-xs" variant="default" disabled={busy} onClick={() => setChoices(initialChoices(plan, request, true))}>Import missing; keep existing</Button>
        <Button size="compact-xs" variant="default" disabled={busy} onClick={() => setChoices((v) => v.map((c, i) =>
          !c.use.length && !plan.files[i].trashReason ? { ...c, disposition: 'trash' } : c,
        ))}>Trash unused incoming files</Button>
      </Group>}
      {plan.files.length > pageSize && <Text size="xs" c="dimmed">Showing {from}–{to} of {plan.files.length} files. Expand an archive to choose its books and its fate.</Text>}
      {plan.files.slice((page - 1) * pageSize, page * pageSize).map((f, offset) => {
        const i = (page - 1) * pageSize + offset
        const open = !collapsed.has(f.fileId)
        return (
        <Card key={f.fileId} withBorder padding="sm" style={{ minWidth: 0 }}>
          <UnstyledButton w="100%" aria-expanded={open} onClick={() => toggle(f.fileId)}>
            <Group justify="space-between" gap="xs" wrap="nowrap">
              <div style={{ minWidth: 0 }}>
                <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>{fileName(f.path)}</Text>
                <Text size="xs" c={f.reason && !open ? 'orange' : 'dimmed'} lineClamp={2} style={{ overflowWrap: 'anywhere' }}>
                  {cardSummary(f, i, !open)}
                </Text>
              </div>
              {open ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />}
            </Group>
          </UnstyledButton>
          {open && <Stack gap="xs" mt="xs">
            <Text size="xs" c="dimmed">{[f.source, f.group].filter(Boolean).join(' · ') || 'Source/group unknown'}</Text>
            {f.reason && <Text size="xs" c="orange">{f.reason}</Text>}
            {f.books.map((b) => (
              <Stack key={bookKey(b.book)} gap={4}>
                <Checkbox
                  label={b.currentSelected ? `Keep this copy for ${forCard(b)}` : b.owners.some((o) => o.present) ? `Use incoming instead for ${forCard(b)}` : `Import ${forCard(b)}`}
                  checked={using(i, b.book)}
                  disabled={busy || (!!b.reason && !b.currentSelected) || !!f.reason || choices[i].disposition === 'later'}
                  onChange={(event) => selectBook(i, b, event.currentTarget.checked)}
                />
                {plan.files.some((_, j) => i !== j && using(j, b.book)) && <Text size="xs" c="dimmed">Another incoming archive is selected for this book. Select this copy to switch.</Text>}
                <Score value={b.incoming} />
                {b.owners.map((o) => (
                  <div key={o.fileId}>
                    <Text size="xs" style={{ overflowWrap: 'anywhere' }}>
                      {using(i, b.book) && !b.currentSelected ? 'Replace' : replacementChosen(b.book) ? 'Replaced by another incoming archive' : 'Keep existing'}: {fileName(o.path)}{!o.present && ' (missing)'}
                      {[o.source, o.group].filter(Boolean).length > 0 && ` · ${[o.source, o.group].filter(Boolean).join(' · ')}`}
                      {o.pinned && <Badge size="xs" ml={4}>Pinned</Badge>}
                    </Text>
                    <Score value={o.score} />
                  </div>
                ))}
                {b.requestedReplacement && <Text size="xs" c="blue">Replacement requested when this download was selected.</Text>}
                {b.reason && <Text size="xs" c="orange">{b.reason}</Text>}
                {b.currentSelected && b.reason && <Text size="xs" c="dimmed">Keeping this selection preserves existing ownership without replacing another copy.</Text>}
              </Stack>
            ))}
            <Select
              label={single ? 'The file itself' : `After review: ${fileName(f.path)}`}
              value={choices[i].disposition}
              disabled={busy}
              data={[
                { value: 'keep', label: choices[i].use.length ? (single ? 'Keep it for the ticked cards' : 'Keep archive for selected books') : (single ? 'Keep it, used by no card' : 'Keep unused file') },
                { value: 'trash', label: single ? 'Move it to trash' : 'Move incoming file to trash', disabled: choices[i].use.length > 0 || !!f.trashReason },
                { value: 'later', label: 'Leave for later' },
              ]}
              onChange={(v) => v && update(i, { disposition: v, ...(v === 'later' ? { use: [] } : {}) })}
            />
            {f.trashReason && <Text size="xs" c="dimmed">{f.trashReason}</Text>}
            {f.books.length > 1 && <Text size="xs" c="dimmed">Choices select which books this archive supplies. The archive stays intact.</Text>}
          </Stack>}
        </Card>
        )
      })}
      {plan.files.length > pageSize && <Pagination total={Math.ceil(plan.files.length / pageSize)} value={page} onChange={setPage} size="sm" />}
      {trash > 0 && <Text size="xs" c="dimmed">Only the selected incoming archives move to recoverable trash for 30 days. Download-client payloads stay in place.</Text>}
      {error && <Alert color="red">{error}<Button size="xs" variant="subtle" onClick={() => void refresh()}>Refresh preview</Button></Alert>}
      <Button loading={busy} disabled={choices.every((c) => c.disposition === 'later')} onClick={() => void apply()}>Apply choices</Button>
    </Stack>
  )
}
