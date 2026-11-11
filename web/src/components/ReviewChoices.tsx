import { useState } from 'react'
import { Alert, Badge, Button, Card, Checkbox, Group, Pagination, Select, Stack, Text, UnstyledButton } from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import type { FileScore, ReviewBookPlan, ReviewFileOutcome, ReviewFilePlan, ReviewPlan, ReviewPlanRequest } from '../types/service'
import { QueryFeedback } from './QueryFeedback'
import { fileName } from './editionText'
import { bookKey, initialChoices, useReviewChoices, useReviewPlan } from './useReviewChoices'

type Props = {
  seriesID: number
  request: ReviewPlanRequest
  applied: () => Promise<void>
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
}

const pageSize = 25

// How a file scores with a profile, and the formats that made the score.
function Score({ value }: { value: FileScore }) {
  return (
    <details className="more">
      <summary>Score {value.score} · {value.profileName || 'No profile'}</summary>
      <Text size="xs" c="dimmed">
        {value.formats.map((f) => `${f.name}: ${f.score > 0 ? '+' : ''}${f.score}`).join(' · ') || 'No custom formats matched.'}
      </Text>
      {!value.factsKnown && <Text size="xs" c="orange">File facts are unknown; choose explicitly after checking the archive.</Text>}
    </details>
  )
}

const outcomeText: Record<string, string> = {
  imported: 'Imported',
  kept: 'Kept archive',
  trashed: 'Moved to trash',
  trash_pending: 'Trash move pending',
  later: 'Left for later',
  settlement_pending: 'Finishing download pending',
}

// What was saved, when some of it still needs attention.
export function ReviewReceipt({ plan, receipt, done }: { plan: ReviewPlan; receipt: ReviewFileOutcome[]; done: () => void }) {
  return (
    <Stack gap="sm">
      <Text size="sm" fw={500}>Choices saved</Text>
      {receipt.map((f, i) => <Text key={`${f.fileId}-${i}`} size="sm" style={{ overflowWrap: 'anywhere' }}>
        {fileName(plan.files.find((p) => p.fileId === f.fileId)?.path ?? '')}: {outcomeText[f.status] ?? f.status}{f.note && ` · ${f.note}`}
      </Text>)}
      <Text size="xs" c="dimmed">Pending trash moves will retry automatically. Archives already imported or retained keep their saved choices.</Text>
      <Button onClick={done}>Done</Button>
    </Stack>
  )
}

// Several incoming archives at once: which books each supplies and what
// happens to it afterwards. One file on its own is FileUses.
export function ReviewChoices(props: Props) {
  const preview = useReviewPlan(props.seriesID, props.request)
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

function Choices({ seriesID, request, applied, onDirtyChange, onBusyChange, plan, refresh }: Props & { plan: ReviewPlan; refresh: () => Promise<unknown> }) {
  const { choices, setChoices, busy, error, receipt, using, update, selectBook, replacementChosen, counts, nothingToApply, apply } =
    useReviewChoices({ seriesID, request, plan, applied, onDirtyChange, onBusyChange })
  // With several editions in play, every choice says which one it is for.
  const severalCards = new Set(plan.files.flatMap((f) => f.books.map((b) => b.card))).size > 1
  const forCard = (b: ReviewBookPlan) => (severalCards ? `${b.label} — ${b.card}` : b.label)
  const [page, setPage] = useState(1)
  // Big packs open as one-line summaries; a single page opens expanded,
  // where every choice is already on screen.
  const [collapsed, setCollapsed] = useState<Set<number>>(
    () => new Set(plan.files.length > pageSize ? plan.files.map((f) => f.fileId) : []),
  )
  const { imports, kept, replacements, trash, missing, later } = counts
  // Only what will happen: six counters, five of them zero, read as noise.
  const summary = ([[imports, 'Import'], [kept, 'Keep existing'], [replacements, 'Replace'], [trash, 'Trash'], [missing, 'Still missing'], [later, 'Left for later']] as const)
    .filter(([n]) => n > 0)
    .map(([n, label]) => `${label} ${n}`)
    .join(' · ') || 'Nothing chosen yet'
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
  if (receipt) return <ReviewReceipt plan={plan} receipt={receipt} done={() => void applied()} />
  const from = (page - 1) * pageSize + 1
  const to = Math.min(page * pageSize, plan.files.length)
  // Big packs open as one-line summaries to expand; otherwise every file
  // shows its choices at once and needs no toggle.
  const collapsible = plan.files.length > pageSize
  const fileChoices = (f: ReviewFilePlan, i: number) => (
    <Stack gap="xs">
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
      {f.trashReason && <Text size="xs" c="dimmed">{f.trashReason}</Text>}
      {f.books.length > 1 && <Text size="xs" c="dimmed">Choices select which books this archive supplies. The archive stays intact.</Text>}
    </Stack>
  )
  const fate = (f: ReviewFilePlan, i: number) => (
    <Select
      label="After review"
      aria-label={`After review: ${fileName(f.path)}`}
      size="xs"
      w={220}
      value={choices[i].disposition}
      disabled={busy}
      allowDeselect={false}
      data={[
        { value: 'keep', label: choices[i].use.length ? 'Keep for the ticked books' : 'Keep, unused' },
        { value: 'trash', label: 'Move to trash', disabled: choices[i].use.length > 0 || !!f.trashReason },
        { value: 'later', label: 'Leave for later' },
      ]}
      onChange={(v) => v && update(i, { disposition: v, ...(v === 'later' ? { use: [] } : {}) })}
    />
  )
  return (
    <Stack gap="sm">
      <Text size="sm" c="dimmed">Choose which copies to use. Existing copies stay selected unless you explicitly choose a replacement.</Text>
      <Text size="sm" fw={500} role="status">{summary}</Text>
      {plan.files.length > 1 && <Group gap="xs">
        <Button size="compact-xs" variant="default" disabled={busy} onClick={() => setChoices(initialChoices(plan, request, true))}>Import missing; keep existing</Button>
        <Button size="compact-xs" variant="default" disabled={busy} onClick={() => setChoices((v) => v.map((c, i) =>
          !c.use.length && !plan.files[i].trashReason ? { ...c, disposition: 'trash' } : c,
        ))}>Trash unused incoming files</Button>
      </Group>}
      {collapsible && <Text size="xs" c="dimmed">Showing {from}–{to} of {plan.files.length} files. Expand an archive to choose its books and its fate.</Text>}
      {plan.files.slice((page - 1) * pageSize, page * pageSize).map((f, offset) => {
        const i = (page - 1) * pageSize + offset
        const open = !collapsible || !collapsed.has(f.fileId)
        const origin = [f.source, f.group].filter(Boolean).join(' · ')
        return (
        <Card key={f.fileId} withBorder padding="sm" style={{ minWidth: 0 }}>
          <Group justify="space-between" gap="sm" wrap="nowrap" align="flex-start">
            <UnstyledButton component={collapsible ? 'button' : 'div'} style={{ minWidth: 0, flex: 1, cursor: collapsible ? 'pointer' : 'default' }}
              aria-expanded={collapsible ? open : undefined} onClick={collapsible ? () => toggle(f.fileId) : undefined}>
              <Group gap={6} wrap="nowrap" align="flex-start">
                {collapsible && (open ? <IconChevronDown size={14} style={{ marginTop: 3, flexShrink: 0 }} /> : <IconChevronRight size={14} style={{ marginTop: 3, flexShrink: 0 }} />)}
                <div style={{ minWidth: 0 }}>
                  <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>{fileName(f.path)}</Text>
                  <Text size="xs" c={f.reason && !open ? 'orange' : 'dimmed'} lineClamp={2} style={{ overflowWrap: 'anywhere' }}>
                    {open ? origin || 'Source and group unknown' : cardSummary(f, i, true)}
                  </Text>
                </div>
              </Group>
            </UnstyledButton>
            {open && fate(f, i)}
          </Group>
          {open && <div style={{ marginTop: 8 }}>{fileChoices(f, i)}</div>}
        </Card>
        )
      })}
      {collapsible && <Pagination total={Math.ceil(plan.files.length / pageSize)} value={page} onChange={setPage} size="sm" />}
      {trash > 0 && <Text size="xs" c="dimmed">Only the selected incoming archives move to recoverable trash for 30 days. Download-client payloads stay in place.</Text>}
      {error && <Alert color="red">{error}<Button size="xs" variant="subtle" onClick={() => void refresh()}>Refresh preview</Button></Alert>}
      {nothingToApply && (
        <Text size="xs" c="dimmed">Every file is left for later. Tick the books to import, or choose what happens to a file.</Text>
      )}
      <Button loading={busy} disabled={nothingToApply} onClick={() => void apply(`Saved: ${summary.toLowerCase()}`)}>Apply choices</Button>
    </Stack>
  )
}
