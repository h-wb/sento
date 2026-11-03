import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Alert, Anchor, Badge, Button, Checkbox, Group, NumberInput, Paper, Popover, Stack, Text, Tooltip, UnstyledButton } from '@mantine/core'
import { api, fmtNum, fmtSize, type ReleaseCandidate } from '../api'
import type { AcquisitionPlan as Plan, PlanBook, PlanOutcome, PlanPick, SearchScope } from '../types/service'
import type { BookRef } from '../types/store'
import CandidateList from './CandidateList'
import classes from './AcquisitionPlan.module.css'

const bookKey = (b: BookRef) => `${b.versionId}/${b.bookId}`
// Past this many books the shelf stops being readable at a glance; the
// legend spells the ranges out instead.
const SHELF_LIMIT = 80
// What a search says about a book, in the order the legend lists it.
// "planned" is the selection itself; the rest are search observations.
const states: [string, string][] = [
  ['planned', 'Will download'],
  ['upgrade', 'Will upgrade'],
  ['available', 'Available, not selected'],
  ['outside_profile', 'Found, needs review'],
  ['no_seeders', 'No seeders reported'],
  ['not_found', 'Not found in this search'],
  ['unknown', 'Not checked'],
]
const bookName = (b: PlanBook) => b.label || fmtNum(b.book.number)
// "1–3, 5": consecutive whole numbers collapse into a range.
const ranges = (books: PlanBook[]) => {
  const sorted = [...books].sort((a, b) => a.book.number - b.book.number)
  const out: string[] = []
  let start = 0
  for (let i = 1; i <= sorted.length; i++) {
    if (i < sorted.length && sorted[i].book.number - sorted[i - 1].book.number === 100) continue
    out.push(start === i - 1 ? bookName(sorted[start]) : `${bookName(sorted[start])}–${bookName(sorted[i - 1])}`)
    start = i
  }
  return out.join(', ')
}
const outcomeBadges: Record<string, [string, string]> = {
  queued: ['blue', 'Queued'],
  skipped: ['gray', 'Skipped'],
  failed: ['red', 'Failed'],
}
const count = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`

export default function AcquisitionPlan({ plan, candidates, searching = false, seriesID, ledger, scope, busy, onGrab, onUnblock, onChanged, onBusyChange }: {
  plan: Plan
  candidates: ReleaseCandidate[]
  // Sources are still answering: the plan is provisional and read-only.
  searching?: boolean
  seriesID: number
  ledger: 'volume' | 'chapter'
  scope: SearchScope
  busy: boolean
  onGrab: (c: ReleaseCandidate, force: boolean) => Promise<void>
  onUnblock: (c: ReleaseCandidate) => Promise<void>
  onChanged: () => void
  onBusyChange: (busy: boolean) => void
}) {
  // Releases listed in the plan keep their place when unticked, so a
  // choice can be undone where it was made.
  const [listed, setListed] = useState(() => plan.picks.map((p) => p.key))
  const [selected, setSelected] = useState(() => new Set(plan.picks.map((p) => p.key)))
  const [browsing, setBrowsing] = useState(false)
  // A book picked on the shelf narrows the release list to what holds it.
  const [focus, setFocus] = useState<PlanBook | null>(null)
  const [sending, setSending] = useState(false)
  const [outcomes, setOutcomes] = useState<PlanOutcome[] | null>(null)
  const [error, setError] = useState('')
  const [rangeOpen, setRangeOpen] = useState(false)
  const [range, setRange] = useState<{ from: number | string; to: number | string }>({ from: '', to: '' })
  const releases = useMemo(() => new Map(candidates.map((c) => [c.key, c])), [candidates])
  const known = useMemo(() => new Map(plan.books.map((b) => [bookKey(b.book), b])), [plan.books])

  // Overlapping books go to the first selected release that has them, in
  // the planner's order.
  const assigned = new Set<string>()
  // The same torrent offered by two indexers is one download.
  const content = new Map<string, number>()
  const rows = listed.flatMap((key) => {
    const c = releases.get(key)
    if (!c) return []
    const on = selected.has(key)
    const books = !on ? [] : (c.coverage ?? []).filter((b) => {
      if (assigned.has(bookKey(b))) return false
      assigned.add(bookKey(b))
      return true
    })
    if (books.length > 0 && !content.has(c.contentKey || c.key)) content.set(c.contentKey || c.key, c.size ?? 0)
    return [{ c, on, books }]
  })
  const picks: PlanPick[] = rows.filter((r) => r.books.length > 0)
    .map((r) => ({ key: r.c.key, books: r.books, additionalBooks: Math.max(0, r.c.representedBooks - r.books.length) }))

  const sizes = [...content.values()]
  const size = sizes.reduce((sum, n) => sum + n, 0)
  const unknownSizes = sizes.filter((n) => !n).length
  const noun = ledger === 'volume' ? 'volume' : 'chapter'
  const short = ledger === 'volume' ? 'Vol.' : 'Ch.'
  const stateOf = (b: PlanBook) =>
    !assigned.has(bookKey(b.book)) ? b.availability : b.ownership === 'upgrade_eligible' ? 'upgrade' : 'planned'
  const stateLabel = (state: string) =>
    searching && state === 'unknown' ? 'Still searching' : (states.find(([s]) => s === state)?.[1] ?? state)
  const upgrades = plan.books.filter((b) => stateOf(b) === 'upgrade').length
  const locked = busy || sending || searching || outcomes !== null
  const outcomeOf = new Map((outcomes ?? []).map((o) => [o.key, o]))
  const queuedCount = (outcomes ?? []).filter((o) => o.status === 'queued').length
  const holds = (c: ReleaseCandidate, b: PlanBook) =>
    [...(c.contents ?? []), ...(c.coverage ?? [])].some((ref) => bookKey(ref) === bookKey(b.book))
  const listedCandidates = focus ? candidates.filter((c) => holds(c, focus)) : candidates
  const toggle = (c: ReleaseCandidate) => {
    setListed((before) => (before.includes(c.key) ? before : [...before, c.key]))
    setSelected((before) => {
      const next = new Set(before)
      if (next.has(c.key)) next.delete(c.key)
      else next.add(c.key)
      return next
    })
  }
  // Select every downloadable release whose books all fall in the range;
  // overlapping ones still give each book to the first.
  const selectRange = () => {
    const from = Number(range.from) * 100
    const to = range.to === '' ? Infinity : Number(range.to) * 100
    const keys = candidates
      .filter((c) => c.decision?.canDownload && c.coverage?.length && c.coverage.every((b) => b.number >= from && b.number <= to))
      .map((c) => c.key)
    setListed((before) => [...before, ...keys.filter((k) => !before.includes(k))])
    setSelected((before) => new Set([...before, ...keys]))
    setRangeOpen(false)
  }
  const named = (refs: BookRef[]) =>
    ranges(refs.map((b) => known.get(bookKey(b)) ?? { book: b, label: '', ownership: '', availability: '' }))
  const submit = async () => {
    setSending(true)
    onBusyChange(true)
    setError('')
    try {
      const response = await api.search(seriesID, { ledger, scope, picks })
      setOutcomes(response.outcomes ?? [])
      onChanged()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally { setSending(false); onBusyChange(false) }
  }
  const total = plan.books.length
  return (
    <Stack gap="md">
      <Paper withBorder p="md" data-testid="acquisition-plan">
        <Stack gap="sm">
          <Group justify="space-between" align="flex-start">
            <div>
              <Text fw={600}>
                {total > 0 ? `${assigned.size} of ${count(total, scope.allBooks ? noun : `wanted ${noun}`)} selected` : `${count(assigned.size, noun)} selected`}
              </Text>
              <Text size="sm" c="dimmed">
                {assigned.size === 0 ? 'Nothing will be downloaded yet.' : <>
                  {count(content.size, 'release')} · {size > 0 ? fmtSize(size) : 'size unknown'}{size > 0 && unknownSizes > 0 ? ' + unknown sizes' : ''}
                  {upgrades > 0 && ` · ${count(upgrades, 'upgrade')}`}
                </>}
              </Text>
            </div>
            {!outcomes && (
              <Button size="sm" disabled={locked || assigned.size === 0 || picks.length > 200} loading={sending} onClick={() => void submit()}>
                {searching ? 'Searching…' : `Download ${count(assigned.size, noun)}`}
              </Button>
            )}
          </Group>

          {total > 0 && total <= SHELF_LIMIT && (
            <div className={classes.shelf}>
              {plan.books.map((b) => {
                const state = stateOf(b)
                const on = focus !== null && bookKey(focus.book) === bookKey(b.book)
                return (
                  <Tooltip key={bookKey(b.book)} label={`${short} ${bookName(b)} — ${stateLabel(state)}. Show its releases.`} openDelay={200}>
                    <UnstyledButton className={classes.book} data-state={state} data-focus={on || undefined} aria-pressed={on}
                      aria-label={`${noun} ${bookName(b)}: ${stateLabel(state)}`} onClick={() => setFocus(on ? null : b)}>
                      {bookName(b)}
                    </UnstyledButton>
                  </Tooltip>
                )
              })}
            </div>
          )}
          {total > 0 && (
            <Group gap="md" style={{ rowGap: 4 }}>
              {states.map(([state]) => {
                const books = plan.books.filter((b) => stateOf(b) === state)
                if (!books.length) return null
                return (
                  <Group key={state} gap={6} wrap="nowrap" align="center" maw="100%">
                    <span className={classes.dot} data-state={state} />
                    <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                      {stateLabel(state)} · {total <= SHELF_LIMIT ? books.length : ranges(books)}
                    </Text>
                  </Group>
                )
              })}
            </Group>
          )}
          {scope.allBooks && !outcomes && (
            <Group gap="xs">
              <Popover opened={rangeOpen} onChange={setRangeOpen} position="bottom-start" withArrow trapFocus>
                <Popover.Target>
                  <Button size="compact-xs" variant="subtle" disabled={locked} onClick={() => setRangeOpen(!rangeOpen)}>
                    Select range
                  </Button>
                </Popover.Target>
                <Popover.Dropdown>
                  <Group gap="xs" align="flex-end">
                    <NumberInput label="From" size="xs" w={90} min={0} hideControls value={range.from}
                      onChange={(from) => setRange({ ...range, from })} />
                    <NumberInput label="To" size="xs" w={90} min={0} hideControls value={range.to}
                      onChange={(to) => setRange({ ...range, to })} onKeyDown={(e) => e.key === 'Enter' && selectRange()} />
                    <Button size="xs" onClick={selectRange}>Select</Button>
                  </Group>
                </Popover.Dropdown>
              </Popover>
              {selected.size > 0 && (
                <Button size="compact-xs" variant="subtle" disabled={locked} onClick={() => setSelected(new Set())}>
                  Clear selection
                </Button>
              )}
            </Group>
          )}
          {!plan.complete && !searching && <Text size="sm" c="orange">Some sources could not be checked, so this plan may be missing releases.</Text>}

          {rows.length > 0 && (
            <div className={classes.picks}>
              {rows.map(({ c, on, books }) => {
                const extra = Math.max(0, c.representedBooks - books.length)
                const outcome = outcomeOf.get(c.key)
                const [color, word] = outcomeBadges[outcome?.status ?? ''] ?? ['gray', outcome?.status]
                return (
                  <div key={c.key} className={classes.pick} data-off={on && books.length > 0 ? undefined : true}>
                    <Checkbox checked={on} disabled={locked} onChange={() => toggle(c)} aria-label={`Include ${c.title}`} mt={2} />
                    <div className={classes.pickBody}>
                      <Group gap={8} wrap="nowrap" align="baseline">
                        <Badge variant="light" color={books.length ? 'blue' : 'gray'} className={classes.pickBooks}>
                          {books.length ? `${short} ${named(books)}` : on ? 'Nothing new' : 'Not selected'}
                        </Badge>
                        <Text size="sm" fw={500} lineClamp={1} title={c.title} style={{ overflowWrap: 'anywhere' }}>{c.title}</Text>
                      </Group>
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        {c.site} · {c.size ? fmtSize(c.size) : 'Size unknown'}
                        {on && books.length === 0 && ' · Everything in it is covered by the releases above'}
                        {books.length > 0 && extra > 0 && ` · Pack also holds ${count(extra, 'book')} you don’t need from it`}
                      </Text>
                      {books.length > 0 && books.length * 2 < c.representedBooks && (
                        <Text size="xs" c="orange">Mostly books you don’t need, so it is only downloaded because you chose it.</Text>
                      )}
                      {outcome && outcome.status !== 'queued' && (
                        <Text size="xs" c={outcome.status === 'failed' ? 'red' : 'dimmed'} style={{ overflowWrap: 'anywhere' }}>{outcome.message}</Text>
                      )}
                    </div>
                    {outcome && <Badge variant="light" color={color} style={{ flex: 'none' }}>{word}</Badge>}
                  </div>
                )
              })}
            </div>
          )}

          {error && <Alert color="red" role="alert" styles={{ message: { overflowWrap: 'anywhere' } }}>{error}</Alert>}
          {outcomes ? (
            <Text size="sm" aria-live="polite">
              {queuedCount > 0 ? `${count(queuedCount, 'download')} queued` : 'Nothing was queued'}
              {queuedCount < outcomes.length && ` · ${outcomes.length - queuedCount} not queued, see above`}.{' '}
              {queuedCount > 0 && <><Anchor component={Link} to="/queue" size="sm">Follow in Activity</Anchor>.{' '}</>}
              <Text span size="sm" c="dimmed">Search again to see what is still available.</Text>
            </Text>
          ) : (
            <Group justify="space-between" gap="xs">
              <Text size="xs" c="dimmed">
                {searching ? 'Filling in as sources answer.' : `Searched at ${new Date(plan.searchedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}. Each release is checked again before it is queued.`}
              </Text>
              {rows.length > 0 && (
                <Button variant="subtle" size="compact-xs" aria-expanded={browsing} onClick={() => setBrowsing(!browsing)}>
                  {browsing ? 'Hide all releases' : `Browse all ${count(candidates.length, 'release')}`}
                </Button>
              )}
            </Group>
          )}
        </Stack>
      </Paper>
      {focus && (
        <Group gap="xs">
          <Text size="sm">
            {listedCandidates.length ? `Releases containing ${noun} ${bookName(focus)}` : `No release in this search contains ${noun} ${bookName(focus)}.`}
          </Text>
          <Button variant="subtle" size="compact-xs" onClick={() => setFocus(null)}>Show all releases</Button>
        </Group>
      )}
      {(browsing || focus || rows.length === 0) && (listedCandidates.length > 0 || !focus) && (
        <CandidateList key={focus ? bookKey(focus.book) : 'all'} candidates={listedCandidates} onGrab={onGrab} onUnblock={onUnblock} busy={locked}
          searching={searching} editionWide selected={selected} onToggle={toggle} showAll={!!focus} allBooks={scope.allBooks} />
      )}
    </Stack>
  )
}
