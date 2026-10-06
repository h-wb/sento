import { useState } from 'react'
import {
  Anchor,
  Badge,
  Button,
  Group,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import { useConfirmAction } from './ConfirmAction'
import { notify } from '../ui'
import { IconSearch } from '@tabler/icons-react'
import { fmtNum, fmtSize, type ReleaseCandidate } from '../api'
import type { Range } from '../types/parse'

// Rejections force-grab cannot bypass (mirror of the backend's hard set).
const hardRejections = new Set([
  'rejected.language',
  'rejected.language_unknown',
  'rejected.unit_ambiguous',
  'rejected.target_group',
  'rejected.no_upgrade',
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
  'rejected.edition_routing': 'no matching collection',
  'rejected.no_coverage': 'no wanted volumes or chapters',
  'rejected.blocklisted': 'blocked',
  'rejected.language': 'wrong language',
  'rejected.unit_ambiguous': 'Several edition units share this number; select a specific unit',
  'rejected.language_unknown': 'language needs verification',
  'rejected.target_group': 'different release group',
  'rejected.no_upgrade': 'no improvement over owned copy',
  'rejected.format_score': 'below minimum score',
  'rejected.pack_threshold': 'pack mostly already owned',
  'rejected.size': 'outside size limits',
  'rejected.seeders': 'not enough seeders',
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
  onGrab?: (c: ReleaseCandidate, force: boolean) => void | Promise<void>
  busy?: boolean
}) {
  const { confirm, confirmation } = useConfirmAction()
  const [queued, setQueued] = useState<Set<string>>(new Set())
  const download = async (c: ReleaseCandidate, force: boolean) => {
    if (!onGrab || busy || queued.has(c.guid)) return
    await onGrab(c, force)
    setQueued((prev) => new Set([...prev, c.guid]))
  }
  const requestDownload = (c: ReleaseCandidate, force: boolean) => {
    if (force)
      confirm({
        title: 'Download rejected release?',
        message: `“${c.title}” doesn't meet your rules: ${(c.rejections ?? []).map(friendly).join(', ')}. Download it anyway?`,
        label: 'Download anyway',
        onConfirm: () => download(c, true),
      })
    else void download(c, false).catch(notify.err)
  }
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
    .sort((a, b) =>
      asc ? sortVal(a, sort) - sortVal(b, sort) : sortVal(b, sort) - sortVal(a, sort),
    )

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
            available · {rejected.length} rejected
            {dropped > 0 && ` · ${dropped} excluded from this search`}
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
              <Table.Th
                w={90}
                style={sortableTh}
                aria-sort={sort === 'size' ? (asc ? 'ascending' : 'descending') : 'none'}
              >
                <UnstyledButton onClick={() => sortBy('size')}>Size{arrow('size')}</UnstyledButton>
              </Table.Th>
              <Table.Th
                w={80}
                style={sortableTh}
                aria-sort={sort === 'seeders' ? (asc ? 'ascending' : 'descending') : 'none'}
              >
                <UnstyledButton onClick={() => sortBy('seeders')}>
                  Seeders{arrow('seeders')}
                </UnstyledButton>
              </Table.Th>
              <Table.Th
                w={90}
                style={sortableTh}
                aria-sort={sort === 'score' ? (asc ? 'ascending' : 'descending') : 'none'}
              >
                <UnstyledButton onClick={() => sortBy('score')}>
                  Score{arrow('score')}
                </UnstyledButton>
              </Table.Th>
              {onGrab && <Table.Th w={70} />}
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {shown.map((c) => (
              <CandidateRow
                key={c.guid}
                c={c}
                onGrab={onGrab ? requestDownload : undefined}
                busy={busy}
                queued={queued.has(c.guid)}
              />
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      {shown.length === 0 && filter && (
        <Text size="xs" c="dimmed">
          Nothing matches “{filter}”.
        </Text>
      )}
      {confirmation}
    </Stack>
  )
}

function CandidateRow({
  c,
  onGrab,
  busy,
  queued,
}: {
  c: ReleaseCandidate
  onGrab?: (c: ReleaseCandidate, force: boolean) => void | Promise<void>
  busy: boolean
  queued: boolean
}) {
  const rejections = c.rejections ?? []
  const rejected = rejections.length > 0
  const hard = rejections.some((r) => hardRejections.has(r))
  const coverage = fmtRanges('v', c.parse.volumes) ?? fmtRanges('c', c.parse.chapters) ?? null
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
          <Tooltip label="Higher scores are preferred for automatic downloads">
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
                ? 'This release cannot be downloaded'
                : rejected
                  ? 'Review the rejection before downloading'
                  : 'Download release'
            }
          >
            <Button
              size="compact-xs"
              color={rejected ? 'orange' : 'brand'}
              variant={rejected ? 'light' : 'filled'}
              disabled={hard || busy || queued}
              onClick={() => onGrab(c, rejected)}
            >
              {queued ? 'Queued' : rejected ? 'Download anyway' : 'Download'}
            </Button>
          </Tooltip>
        </Table.Td>
      )}
    </Table.Tr>
  )
}
