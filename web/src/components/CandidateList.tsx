import { useState } from 'react'
import { Badge, Button, Group, Stack, Switch, Text, TextInput, Tooltip } from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { fmtNum, fmtSize, type ReleaseCandidate } from '../api'
import type { Range } from '../types/parse'

// Rejections force-grab cannot bypass (mirror of the backend's hard set).
const hardRejections = new Set([
  'rejected.content_type',
  'rejected.series_match',
  'rejected.edition_routing',
  'rejected.no_coverage',
  'rejected.blocklisted',
])

// Human labels for funnel rejection codes.
const rejectionLabel: Record<string, string> = {
  'rejected.content_type': 'not manga',
  'rejected.series_match': 'different series',
  'rejected.edition_routing': 'no matching edition',
  'rejected.no_coverage': 'covers nothing wanted',
  'rejected.blocklisted': 'blocklisted',
  'rejected.language': 'wrong language',
  'rejected.pack_threshold': 'pack mostly unwanted',
  'rejected.size': 'implausible size',
  'rejected.seeders': 'no seeders',
}
const friendly = (code: string) => rejectionLabel[code] ?? code.replace('rejected.', '')

// fmtRanges renders parsed number ranges compactly: "v1–112", "c1102.5".
function fmtRanges(prefix: string, ranges?: Range[]): string | null {
  if (!ranges?.length) return null
  const parts = ranges
    .slice(0, 3)
    .map((r) => (r.From === r.To ? fmtNum(r.From) : `${fmtNum(r.From)}–${fmtNum(r.To)}`))
  return prefix + parts.join(',') + (ranges.length > 3 ? ',…' : '')
}

// CandidateList renders funnel results: grabbable releases up front with
// the parse facts as chips, rejected ones behind a toggle with their
// reasons in plain words. Shared by the slot modal and the ledger-wide
// dry-run views.
export default function CandidateList({
  candidates,
  dropped = 0,
  onGrab,
  busy = false,
}: {
  candidates: ReleaseCandidate[]
  dropped?: number
  onGrab?: (c: ReleaseCandidate, force: boolean) => void
  busy?: boolean
}) {
  const [showRejected, setShowRejected] = useState(false)
  const [filter, setFilter] = useState('')

  const ok = candidates.filter((c) => !c.rejections?.length)
  const rejected = candidates.filter((c) => c.rejections?.length)
  // An all-rejected result must not look empty — show the reasons.
  const revealRejected = showRejected || ok.length === 0

  const match = (c: ReleaseCandidate) =>
    !filter.trim() || c.title.toLowerCase().includes(filter.trim().toLowerCase())
  const shown = [...ok, ...(revealRejected ? rejected : [])].filter(match)

  return (
    <Stack gap={6}>
      <Group justify="space-between" wrap="wrap" gap="xs">
        <Group gap="xs">
          <Text size="xs" c="dimmed">
            <Text span c="green" fw={600} size="xs">
              {ok.length}
            </Text>{' '}
            grabbable · {rejected.length} rejected
            {dropped > 0 && ` · ${dropped} for the other ledger`}
          </Text>
          {rejected.length > 0 && ok.length > 0 && (
            <Switch
              size="xs"
              label="Show rejected"
              checked={showRejected}
              onChange={(e) => setShowRejected(e.currentTarget.checked)}
            />
          )}
        </Group>
        {candidates.length > 15 && (
          <TextInput
            size="xs"
            w={180}
            placeholder="Filter results…"
            leftSection={<IconSearch size={12} />}
            value={filter}
            onChange={(e) => setFilter(e.currentTarget.value)}
          />
        )}
      </Group>

      {shown.map((c) => (
        <CandidateRow key={c.guid} c={c} onGrab={onGrab} busy={busy} />
      ))}
      {shown.length === 0 && filter && (
        <Text size="xs" c="dimmed">
          Nothing matches “{filter}”.
        </Text>
      )}
    </Stack>
  )
}

function CandidateRow({
  c,
  onGrab,
  busy,
}: {
  c: ReleaseCandidate
  onGrab?: (c: ReleaseCandidate, force: boolean) => void
  busy: boolean
}) {
  const rejections = c.rejections ?? []
  const rejected = rejections.length > 0
  const hard = rejections.some((r) => hardRejections.has(r))
  const coverage =
    fmtRanges('v', c.parse.volumes) ?? fmtRanges('c', c.parse.chapters) ?? null
  const editions = [
    c.parse.edition?.omnibus && 'omnibus',
    c.parse.edition?.deluxe && 'deluxe',
    c.parse.edition?.boxset && 'box set',
    c.parse.edition?.colored && 'colored',
    c.parse.quality?.digital && 'digital',
  ].filter(Boolean) as string[]

  return (
    <div style={{ opacity: rejected ? 0.65 : 1 }}>
      <Group justify="space-between" wrap="nowrap" gap="xs">
        <Text size="xs" lineClamp={1} style={{ flex: 1 }} title={c.title}>
          {c.title}
        </Text>
        <Group gap={6} wrap="nowrap">
          {rejected ? (
            <Tooltip label={rejections.map(friendly).join(' · ')}>
              <Badge size="xs" color={hard ? 'red' : 'orange'} variant="light">
                {friendly(rejections[0])}
                {rejections.length > 1 && ` +${rejections.length - 1}`}
              </Badge>
            </Tooltip>
          ) : (
            <Tooltip label="Decision score — the highest eligible release wins auto-grab">
              <Badge size="xs" color="green" variant="light">
                {c.score}
              </Badge>
            </Tooltip>
          )}
          {onGrab && (
            <Tooltip
              label={
                hard
                  ? 'Hard rejection — cannot grab'
                  : rejected
                    ? 'Force grab (overrides the soft rejection)'
                    : 'Grab'
              }
            >
              <Button
                size="compact-xs"
                color={rejected ? 'orange' : 'indigo'}
                variant={rejected ? 'light' : 'filled'}
                disabled={hard || busy}
                onClick={() => onGrab(c, rejected)}
              >
                {rejected ? 'Force' : 'Grab'}
              </Button>
            </Tooltip>
          )}
        </Group>
      </Group>
      <Group gap={4} mt={2} wrap="wrap">
        <Badge
          size="xs"
          variant="dot"
          color={c.protocol === 'torrent' ? 'blue' : 'teal'}
          style={{ textTransform: 'none' }}
        >
          {c.site}
        </Badge>
        {coverage && (
          <Badge size="xs" variant="outline" color="gray" style={{ textTransform: 'none' }}>
            {coverage}
          </Badge>
        )}
        {c.parse.complete && (
          <Badge size="xs" variant="outline" color="cyan">
            complete
          </Badge>
        )}
        {c.parse.language && (
          <Badge size="xs" variant="outline" color="grape">
            {c.parse.language}
          </Badge>
        )}
        {editions.map((e) => (
          <Badge key={e} size="xs" variant="outline" color="gray">
            {e}
          </Badge>
        ))}
        {c.scanlator && (
          <Badge size="xs" variant="outline" color="gray" style={{ textTransform: 'none' }}>
            {c.scanlator}
          </Badge>
        )}
        {(c.covered?.length ?? 0) > 1 && (
          <Tooltip label={`Fills ${c.covered!.length} wanted slots`}>
            <Badge size="xs" variant="light" color="cyan">
              fills {c.covered!.length}
            </Badge>
          </Tooltip>
        )}
        {c.protocol === 'torrent' && (
          <Text size="xs" c="dimmed">
            {fmtSize(c.size ?? 0)} · {c.seeders ?? 0} seeders
          </Text>
        )}
      </Group>
    </div>
  )
}
