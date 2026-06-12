import { useState } from 'react'
import { Anchor, Badge, Button, Group, Stack, Switch, Table, Text, TextInput, Tooltip } from '@mantine/core'
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
  'rejected.format_score': 'below format score floor',
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

type SortKey = 'score' | 'size' | 'seeders'

const sortVal = (c: ReleaseCandidate, k: SortKey) =>
  k === 'score' ? c.score : k === 'size' ? (c.size ?? 0) : (c.seeders ?? 0)

// CandidateList renders funnel results as a sortable table: grabbable
// releases up front with the parse facts as chips, rejected ones behind
// a toggle with their reasons in plain words.
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
  const [sort, setSort] = useState<SortKey>('score')
  const [asc, setAsc] = useState(false)

  const ok = candidates.filter((c) => !c.rejections?.length)
  const rejected = candidates.filter((c) => c.rejections?.length)
  // An all-rejected result must not look empty — show the reasons.
  const revealRejected = showRejected || ok.length === 0

  const match = (c: ReleaseCandidate) =>
    !filter.trim() || c.title.toLowerCase().includes(filter.trim().toLowerCase())
  const shown = [...ok, ...(revealRejected ? rejected : [])]
    .filter(match)
    .sort((a, b) => (asc ? sortVal(a, sort) - sortVal(b, sort) : sortVal(b, sort) - sortVal(a, sort)))

  const sortBy = (k: SortKey) => {
    if (sort === k) setAsc(!asc)
    else {
      setSort(k)
      setAsc(false)
    }
  }
  const arrow = (k: SortKey) => (sort === k ? (asc ? ' ↑' : ' ↓') : '')
  const sortableTh = { cursor: 'pointer', userSelect: 'none', whiteSpace: 'nowrap' } as const

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

      <Table.ScrollContainer minWidth={640}>
        <Table highlightOnHover verticalSpacing={6}>
          <Table.Thead>
            <Table.Tr>
              <Table.Th w={130}>Site</Table.Th>
              <Table.Th>Release</Table.Th>
              <Table.Th w={90} style={sortableTh} onClick={() => sortBy('size')}>
                Size{arrow('size')}
              </Table.Th>
              <Table.Th w={80} style={sortableTh} onClick={() => sortBy('seeders')}>
                Seeders{arrow('seeders')}
              </Table.Th>
              <Table.Th w={90} style={sortableTh} onClick={() => sortBy('score')}>
                Score{arrow('score')}
              </Table.Th>
              {onGrab && <Table.Th w={70} />}
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {shown.map((c) => (
              <CandidateRow key={c.guid} c={c} onGrab={onGrab} busy={busy} />
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
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
    <Table.Tr opacity={rejected ? 0.65 : 1}>
      <Table.Td>
        <Badge
          size="xs"
          variant="dot"
          color={c.protocol === 'torrent' ? 'blue' : 'teal'}
          style={{ textTransform: 'none' }}
        >
          {c.site}
        </Badge>
      </Table.Td>
      <Table.Td>
        {c.url ? (
          <Anchor
            href={c.url}
            target="_blank"
            rel="noopener noreferrer"
            size="xs"
            c="inherit"
            underline="hover"
            lineClamp={1}
            title={c.title}
          >
            {c.title}
          </Anchor>
        ) : (
          <Text size="xs" lineClamp={1} title={c.title}>
            {c.title}
          </Text>
        )}
        <Group gap={4} mt={2} wrap="wrap">
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
          {(c.formats ?? []).map((f) => (
            <Tooltip key={f.name} label={`Custom format · ${f.score >= 0 ? '+' : ''}${f.score}`}>
              <Badge
                size="xs"
                variant="light"
                color={f.score > 0 ? 'indigo' : f.score < 0 ? 'red' : 'gray'}
                style={{ textTransform: 'none' }}
              >
                {f.name}
              </Badge>
            </Tooltip>
          ))}
          {(c.covered?.length ?? 0) > 1 && (
            <Tooltip label={`Fills ${c.covered!.length} wanted slots`}>
              <Badge size="xs" variant="light" color="cyan">
                fills {c.covered!.length}
              </Badge>
            </Tooltip>
          )}
        </Group>
      </Table.Td>
      <Table.Td>
        <Text size="xs" c="dimmed">
          {c.protocol === 'torrent' ? fmtSize(c.size ?? 0) : '—'}
        </Text>
      </Table.Td>
      <Table.Td>
        <Text size="xs" c="dimmed">
          {c.protocol === 'torrent' ? (c.seeders ?? 0) : '—'}
        </Text>
      </Table.Td>
      <Table.Td>
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
      </Table.Td>
      {onGrab && (
        <Table.Td>
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
        </Table.Td>
      )}
    </Table.Tr>
  )
}
