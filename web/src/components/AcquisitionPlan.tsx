import { useMemo, useState } from 'react'
import { Alert, Button, Checkbox, Group, Paper, Stack, Text } from '@mantine/core'
import { api, fmtNum, fmtSize, type ReleaseCandidate } from '../api'
import type { AcquisitionPlan as Plan, PlanOutcome, PlanPick, SearchScope } from '../types/service'
import type { BookRef } from '../types/store'
import CandidateList from './CandidateList'

const bookKey = (b: BookRef) => `${b.versionId}/${b.bookId}`
const availabilityLabels: Record<string, string> = {
  available: 'Available',
  not_found: 'Not found in this search',
  unknown: 'Availability unknown',
  no_seeders: 'No seeders reported',
  outside_profile: 'Found, needs review',
}

export default function AcquisitionPlan({ plan, candidates, seriesID, ledger, scope, busy, onGrab, onChanged, onBusyChange }: {
  plan: Plan
  candidates: ReleaseCandidate[]
  seriesID: number
  ledger: 'volume' | 'chapter'
  scope: SearchScope
  busy: boolean
  onGrab: (c: ReleaseCandidate, force: boolean) => Promise<void>
  onChanged: () => void
  onBusyChange: (busy: boolean) => void
}) {
  const [selected, setSelected] = useState(() => new Set(plan.picks.map((p) => p.key)))
  const [choosing, setChoosing] = useState(false)
  const [sending, setSending] = useState(false)
  const [outcomes, setOutcomes] = useState<PlanOutcome[] | null>(null)
  const [error, setError] = useState('')
  const releases = useMemo(() => [...new Map(candidates.map((c) => [c.key, c])).values()], [candidates])
  // Preserve the planner's order when assigning overlapping books to jobs.
  const chosen = [...selected].flatMap((key) => {
    const release = releases.find((c) => c.key === key)
    return release ? [release] : []
  })
  const assigned = new Set<string>()
  const content = new Set<string>()
  let size = 0, unknownSizes = 0
  const picks: PlanPick[] = chosen.map((c) => {
    const books = (c.coverage ?? []).filter((b) => {
      if (assigned.has(bookKey(b))) return false
      assigned.add(bookKey(b))
      return true
    })
    if (books.length > 0 && !content.has(c.contentKey || c.key)) {
      content.add(c.contentKey || c.key)
      if (c.size) size += c.size
      else unknownSizes++
    }
    return { key: c.key, books, additionalBooks: Math.max(0, c.representedBooks - books.length) }
  })
  const noun = ledger === 'volume' ? 'volume' : 'chapter'
  const upgrades = plan.books.filter((b) => assigned.has(bookKey(b.book)) && b.ownership === 'upgrade_eligible').length
  const remaining = plan.books.filter((b) => !assigned.has(bookKey(b.book)))
  const locked = busy || sending || outcomes !== null
  const toggle = (c: ReleaseCandidate) => setSelected((before) => {
    const next = new Set(before)
    if (next.has(c.key)) next.delete(c.key)
    else next.add(c.key)
    return next
  })
  const numbers = (refs: BookRef[]) => refs.map((b) => {
    const known = plan.books.find((p) => bookKey(p.book) === bookKey(b))
    return known?.label || fmtNum(b.number)
  }).join(', ')
  const submit = async () => {
    setSending(true)
    onBusyChange(true)
    setError('')
    try {
      const response = await api.search(seriesID, { ledger, scope, picks: picks.filter((p) => p.books.length > 0) })
      setOutcomes(response.outcomes ?? [])
      onChanged()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally { setSending(false); onBusyChange(false) }
  }
  return (
    <Stack gap="md">
      <Paper withBorder p="md" data-testid="acquisition-plan">
        <Stack gap="sm">
          <Group justify="space-between" align="flex-start">
            <div>
              <Text fw={600}>{assigned.size} {noun}{assigned.size === 1 ? '' : 's'} selected in {content.size} release{content.size === 1 ? '' : 's'}</Text>
              <Text size="sm" c="dimmed">
                {size > 0 ? fmtSize(size) : 'Size unknown'}{size > 0 && unknownSizes > 0 ? ' + unknown sizes' : ''}
                {upgrades > 0 && ` · ${upgrades} upgrade${upgrades === 1 ? '' : 's'}`}
              </Text>
            </div>
            <Button variant="subtle" size="xs" onClick={() => setChoosing(!choosing)}>
              {choosing ? 'Hide release choices' : 'Choose releases'}
            </Button>
          </Group>
          {!plan.complete && <Text size="sm" c="orange">Some sources could not be checked. This plan uses the results received.</Text>}
          {chosen.map((c, i) => (
            <Group key={c.key} align="flex-start" wrap="nowrap">
              <Checkbox checked disabled={locked} onChange={() => toggle(c)} aria-label={`Include ${c.title}`} mt={3} />
              <div style={{ minWidth: 0 }}>
                <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>
                  {picks[i].books.length ? `Collects ${noun}${picks[i].books.length === 1 ? '' : 's'} ${numbers(picks[i].books)}` : 'Already covered by another selection; will not download'}
                </Text>
                <Text size="xs" c="dimmed" lineClamp={2} title={c.title} style={{ overflowWrap: 'anywhere' }}>{c.title}</Text>
                <Text size="xs" c="dimmed">
                  {c.site} · {c.size ? fmtSize(c.size) : 'Size unknown'}
                  {picks[i].additionalBooks > 0 && ` · Also contains ${picks[i].additionalBooks} book${picks[i].additionalBooks === 1 ? '' : 's'} outside this job’s selection`}
                </Text>
                {picks[i].books.length > 0 && picks[i].books.length * 2 < c.representedBooks && (
                  <Text size="xs" c="orange">
                    Less than half of this pack is needed, so it is only downloaded when you choose it.
                  </Text>
                )}
              </div>
            </Group>
          ))}
          {remaining.length > 0 && (
            <Stack gap={4}>
              {Object.entries(availabilityLabels).map(([state, label]) => {
                const books = remaining.filter((b) => b.availability === state)
                if (!books.length) return null
                return <Text key={state} size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                  {label}: {books.slice(0, 20).map((b) => b.label || fmtNum(b.book.number)).join(', ')}
                  {books.length > 20 && ` and ${books.length - 20} more`}
                </Text>
              })}
            </Stack>
          )}
          {error && <Alert color="red" role="alert" styles={{ message: { overflowWrap: 'anywhere' } }}>{error}</Alert>}
          {outcomes ? (
            <Stack gap={4} aria-live="polite">
              {outcomes.map((o) => <Text key={o.key} size="sm" c={o.status === 'failed' ? 'red' : undefined} style={{ overflowWrap: 'anywhere' }}>
                {o.message} — {o.title || 'Selected release'}
              </Text>)}
              <Text size="xs" c="dimmed">Search again to see current availability.</Text>
            </Stack>
          ) : <Group justify="space-between">
            <Text size="xs" c="dimmed">Checked {new Date(plan.searchedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}. Choices are checked again before download.</Text>
            <Button size="sm" disabled={locked || assigned.size === 0 || picks.length > 200} loading={sending} onClick={() => void submit()}>
              Download {assigned.size} {noun}{assigned.size === 1 ? '' : 's'}
            </Button>
          </Group>}
        </Stack>
      </Paper>
      {(choosing || chosen.length === 0) && <CandidateList candidates={candidates} onGrab={onGrab} busy={locked} editionWide selected={selected} onToggle={toggle} />}
    </Stack>
  )
}
