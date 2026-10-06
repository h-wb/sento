import { useRef, useState } from 'react'
import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Pagination,
  Select,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
} from '@mantine/core'
import { IconChevronDown, IconChevronUp, IconSearch } from '@tabler/icons-react'
import { useConfirmAction } from './ConfirmAction'
import { fmtNum, fmtSize, type ReleaseCandidate } from '../api'

const releaseKey = (c: ReleaseCandidate) => `${c.provider}/${c.guid}/${c.targetId ?? 0}`
const rangeLabel = (label: string, ranges?: [number, number][]) =>
  ranges?.length
    ? `${label} ${ranges.map(([from, to]) => (from === to ? from : `${from}–${to}`)).join(', ')}`
    : ''
const status = (c: ReleaseCandidate) =>
  c.decision?.status ?? (c.rejections?.length ? 'blocked' : 'ready')
const rank = { ready: 0, review: 1, blocked: 2 }

export default function CandidateList({
  candidates,
  dropped = 0,
  onGrab,
  busy = false,
  searching = false,
}: {
  candidates: ReleaseCandidate[]
  dropped?: number
  onGrab?: (c: ReleaseCandidate, force: boolean) => void | Promise<void>
  busy?: boolean
  searching?: boolean
}) {
  const { confirm, confirmation } = useConfirmAction()
  const [queued, setQueued] = useState<Set<string>>(new Set())
  const downloading = useRef(false)
  const [active, setActive] = useState('')
  const [error, setError] = useState('')
  const [filter, setFilter] = useState('')
  const [state, setState] = useState('all')
  const [site, setSite] = useState<string | null>(null)
  const [protocol, setProtocol] = useState<string | null>(null)
  const [sort, setSort] = useState('best')
  const [page, setPage] = useState(1)
  const unique = [...new Map(candidates.map((c) => [releaseKey(c), c])).values()]
  const ready = unique.filter((c) => status(c) === 'ready').length
  const review = unique.filter((c) => status(c) === 'review').length
  const blocked = unique.length - ready - review
  const resetPage = (fn: () => void) => {
    fn()
    setPage(1)
  }
  const shown = unique
    .filter((c) => {
      const text = [
        c.title,
        c.site,
        c.parse.group,
        c.scanlator,
        c.parse.language,
        c.editionSnapshot?.editionName,
        c.targetSnapshot?.name,
      ]
        .join(' ')
        .toLowerCase()
      return (
        (!filter.trim() || text.includes(filter.trim().toLowerCase())) &&
        (state === 'all' || status(c) === state) &&
        (!site || c.site === site) &&
        (!protocol || c.protocol === protocol)
      )
    })
    .sort((a, b) => {
      // Preserve arrival order while streaming so a result cannot move under a click.
      if (searching) return 0
      if (sort === 'size') return (a.size ?? 0) - (b.size ?? 0)
      if (sort === 'seeders') return (b.seeders ?? -1) - (a.seeders ?? -1)
      if (sort === 'quality') return (b.formatScore ?? 0) - (a.formatScore ?? 0)
      return (
        rank[status(a)] - rank[status(b)] ||
        b.score - a.score ||
        a.title.localeCompare(b.title) ||
        releaseKey(a).localeCompare(releaseKey(b))
      )
    })
  const pages = Math.max(1, Math.ceil(shown.length / 12))
  const currentPage = Math.min(page, pages)
  const download = async (c: ReleaseCandidate, force: boolean) => {
    if (!onGrab || busy || downloading.current || queued.has(releaseKey(c))) return
    downloading.current = true
    setActive(releaseKey(c))
    setError('')
    try {
      await onGrab(c, force)
      setQueued((prev) => new Set([...prev, releaseKey(c)]))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      throw e
    } finally {
      downloading.current = false
      setActive('')
    }
  }
  const requestDownload = (c: ReleaseCandidate, force: boolean) => {
    if (force)
      confirm({
        title: 'Download despite profile limits?',
        message: `“${c.title}” needs review: ${(c.decision?.reasons ?? []).map((r) => r.message).join(' ')} Its edition and book matches still have to pass.`,
        label: 'Download anyway',
        onConfirm: () => download(c, true),
      })
    else void download(c, false).catch(() => {}) // The result list retains the actionable error.
  }
  return (
    <Stack gap="sm">
      <Group justify="space-between" gap="xs">
        <Text size="sm" role="status">
          {ready} ready · {review} need review · {blocked} blocked
        </Text>
        {dropped > 0 && (
          <Text size="xs" c="dimmed">
            {dropped} results for the other book type excluded
          </Text>
        )}
      </Group>
      <TextInput
        aria-label="Filter releases"
        placeholder="Filter by title, group, language, or site"
        leftSection={<IconSearch size={15} />}
        value={filter}
        onChange={(e) => resetPage(() => setFilter(e.currentTarget.value))}
      />
      <SimpleGrid cols={{ base: 2, sm: 4 }} spacing="xs">
        <Select
          aria-label="Release status"
          value={state}
          onChange={(v) => resetPage(() => setState(v ?? 'all'))}
          data={[
            { value: 'all', label: 'All results' },
            { value: 'ready', label: `Ready (${ready})` },
            { value: 'review', label: `Needs review (${review})` },
            { value: 'blocked', label: `Blocked (${blocked})` },
          ]}
          allowDeselect={false}
        />
        <Select
          aria-label="Release site"
          placeholder="All sites"
          clearable
          value={site}
          onChange={(v) => resetPage(() => setSite(v))}
          data={[...new Set(unique.map((c) => c.site))].sort()}
        />
        <Select
          aria-label="Release protocol"
          placeholder="All download types"
          clearable
          value={protocol}
          onChange={(v) => resetPage(() => setProtocol(v))}
          data={[
            { value: 'torrent', label: 'Torrent' },
            { value: 'source', label: 'Direct download' },
          ]}
        />
        <Select
          aria-label="Sort releases"
          value={sort}
          onChange={(v) => resetPage(() => setSort(v ?? 'best'))}
          disabled={searching}
          allowDeselect={false}
          data={[
            { value: 'best', label: 'Best match first' },
            { value: 'quality', label: 'Format score' },
            { value: 'seeders', label: 'Most seeders' },
            { value: 'size', label: 'Smallest download' },
          ]}
        />
      </SimpleGrid>
      {error && (
        <Alert
          color="red"
          title="Download could not be queued"
          style={{ overflowWrap: 'anywhere' }}
        >
          {error}
        </Alert>
      )}
      {shown.length === 0 && (
        <Card withBorder>
          <Stack gap="xs">
            <Text size="sm">No releases match these filters.</Text>
            <Button
              variant="subtle"
              size="xs"
              onClick={() => {
                setFilter('')
                setState('all')
                setSite(null)
                setProtocol(null)
                setPage(1)
              }}
              style={{ alignSelf: 'flex-start' }}
            >
              Clear filters
            </Button>
          </Stack>
        </Card>
      )}
      {shown.slice((currentPage - 1) * 12, currentPage * 12).map((c) => (
        <ReleaseRow
          key={releaseKey(c)}
          c={c}
          queued={queued.has(releaseKey(c))}
          busy={busy || !!active}
          loading={active === releaseKey(c)}
          onGrab={onGrab ? requestDownload : undefined}
        />
      ))}
      {pages > 1 && (
        <Group justify="space-between">
          <Text size="xs" c="dimmed">
            {shown.length} releases
          </Text>
          <Pagination size="sm" total={pages} value={currentPage} onChange={setPage} />
        </Group>
      )}
      {confirmation}
    </Stack>
  )
}

function ReleaseRow({
  c,
  queued,
  busy,
  loading,
  onGrab,
}: {
  c: ReleaseCandidate
  queued: boolean
  busy: boolean
  loading: boolean
  onGrab?: (c: ReleaseCandidate, force: boolean) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const decision = c.decision
  const state = status(c)
  const volume = rangeLabel('Vol.', c.parse.volumes)
  const chapter = rangeLabel('Ch.', c.parse.chapters)
  const facts = [
    volume,
    chapter,
    c.parse.languages?.length
      ? c.parse.languages.map((l) => l.toUpperCase()).join('/')
      : c.parse.language?.toUpperCase() || 'Language unknown',
    c.parse.group || c.scanlator,
    c.parse.quality.digital && 'Digital',
    c.parse.quality.scan && 'Scan',
    c.parse.quality.web && 'Web',
    c.parse.edition.omnibus && 'Omnibus',
    c.parse.edition.deluxe && 'Deluxe',
    c.parse.edition.boxset && 'Box set',
    c.parse.edition.colored && 'Color',
    c.parse.extra && 'Bonus / special',
    c.parse.version && `Revision ${c.parse.version}`,
  ].filter(Boolean)
  const reason = decision?.reasons[0]?.message
  const matched = c.coverage?.length ?? c.covered?.length ?? 0
  return (
    <Card withBorder padding="md" data-testid="release-row">
      <Stack gap="xs">
        <Group justify="space-between" align="flex-start">
          <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere', flex: 1, minWidth: 0 }}>
            {c.site} · {c.protocol === 'torrent' ? 'Torrent' : 'Direct download'}
            {c.protocol === 'torrent' &&
              ` · ${c.size ? fmtSize(c.size) : 'Size unknown'} · ${c.seeders ?? 0} seeders`}
          </Text>
          <Badge
            variant="light"
            color={
              queued ? 'blue' : state === 'ready' ? 'green' : state === 'review' ? 'orange' : 'gray'
            }
          >
            {queued
              ? 'Queued'
              : state === 'ready'
                ? 'Ready'
                : state === 'review'
                  ? 'Needs review'
                  : 'Blocked'}
          </Badge>
        </Group>
        <Text
          fw={600}
          size="sm"
          lineClamp={expanded ? undefined : 2}
          style={{ overflowWrap: 'anywhere' }}
        >
          {c.title}
        </Text>
        <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
          {facts.join(' · ')}
        </Text>
        {c.editionSnapshot && (
          <Text size="xs" c="dimmed">
            {c.editionSnapshot.editionName}
            {c.targetSnapshot && !c.targetSnapshot.isDefault ? ` · ${c.targetSnapshot.name}` : ''}
          </Text>
        )}
        {reason && (
          <Text
            size="sm"
            c={state === 'review' ? 'orange' : 'dimmed'}
            style={{ overflowWrap: 'anywhere' }}
          >
            {reason}
            {(decision?.reasons.length ?? 0) > 1 && ` (+${decision!.reasons.length - 1} more)`}
          </Text>
        )}
        <Group justify="space-between" align="center">
          <Group gap="xs">
            <Button
              size="compact-xs"
              variant="subtle"
              aria-expanded={expanded}
              onClick={() => setExpanded(!expanded)}
              rightSection={expanded ? <IconChevronUp size={12} /> : <IconChevronDown size={12} />}
            >
              {expanded ? 'Hide details' : 'Details'}
            </Button>
            <Text size="xs" c="dimmed">
              {matched > 0
                ? `${matched} wanted ${matched === 1 ? 'book' : 'books'}`
                : decision?.kind || 'Unclassified'}
              {state === 'ready' && ` · Rank score ${c.score}`}
            </Text>
          </Group>
          {onGrab && state !== 'blocked' && (
            <Button
              size="xs"
              variant={state === 'review' ? 'light' : 'filled'}
              color={state === 'review' ? 'orange' : 'brand'}
              disabled={busy || queued || !(decision?.canDownload || decision?.canForce)}
              loading={loading}
              onClick={() => onGrab(c, state === 'review')}
            >
              {queued ? 'Queued' : state === 'review' ? 'Review & download' : 'Download'}
            </Button>
          )}
        </Group>
        {expanded && (
          <Stack
            gap="sm"
            pt="sm"
            style={{ borderTop: '1px solid var(--mantine-color-default-border)' }}
          >
            {!!decision?.reasons.length && (
              <Stack gap={4}>
                {decision.reasons.map((r) => (
                  <Text key={r.code} size="sm" style={{ overflowWrap: 'anywhere' }}>
                    {r.message}{' '}
                    <Text span size="xs" c="dimmed">
                      {r.override
                        ? 'Can be overridden for this download.'
                        : 'Cannot be overridden.'}
                    </Text>
                  </Text>
                ))}
              </Stack>
            )}
            <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="xs">
              <Text size="xs">
                Classification: {decision?.kind ?? 'Unknown'}
                {c.parse.complete ? ' · complete pack claim' : ''}
              </Text>
              <Text size="xs">Language evidence: {decision?.languageSource ?? 'unknown'}</Text>
              <Text size="xs">Number evidence: {decision?.numberSource ?? 'unknown'}</Text>
              <Text size="xs">Profile: {c.policySnapshot?.name ?? 'Built-in defaults'}</Text>
            </SimpleGrid>
            {!!c.coverage?.length && (
              <Text size="xs" style={{ overflowWrap: 'anywhere' }}>
                Will collect:{' '}
                {c.coverage
                  .slice(0, 30)
                  .map((s) => `${s.ledger === 'chapter' ? 'Ch.' : 'Vol.'} ${fmtNum(s.number)}`)
                  .join(', ')}
                {c.coverage.length > 30 ? ` and ${c.coverage.length - 30} more` : ''}
              </Text>
            )}
            {!!decision?.warnings.length && (
              <Stack gap={3}>
                {decision.warnings.map((w) => (
                  <Text size="xs" c="dimmed" key={w}>
                    {w}
                  </Text>
                ))}
              </Stack>
            )}
            {!!c.scoreParts?.length && (
              <Stack gap={2}>
                <Text size="xs" fw={600}>
                  Ranking
                </Text>
                {c.scoreParts.map((p, i) => (
                  <Group justify="space-between" key={`${p.label}-${i}`}>
                    <Text size="xs">{p.label}</Text>
                    <Text size="xs">
                      {p.value > 0 ? '+' : ''}
                      {p.value}
                    </Text>
                  </Group>
                ))}
              </Stack>
            )}
            {!!c.formats?.length && (
              <Text size="xs" style={{ overflowWrap: 'anywhere' }}>
                Matched formats:{' '}
                {c.formats
                  .map((f) => `${f.name} (${f.score > 0 ? '+' : ''}${f.score})`)
                  .join(' · ')}
              </Text>
            )}
            {c.url && (
              <Anchor href={c.url} target="_blank" rel="noopener noreferrer" size="xs">
                Open original release
              </Anchor>
            )}
            {!!c.parseTrace?.length && (
              <details>
                <summary style={{ fontSize: 'var(--mantine-font-size-xs)', cursor: 'pointer' }}>
                  Parsing details
                </summary>
                <Stack gap={3} mt="xs">
                  {c.parseTrace.map((p, i) => (
                    <Text key={i} size="xs" style={{ overflowWrap: 'anywhere' }}>
                      {p.rule}
                      {p.text ? `: ${p.text}` : ''}
                    </Text>
                  ))}
                </Stack>
              </details>
            )}
          </Stack>
        )}
      </Stack>
    </Card>
  )
}
