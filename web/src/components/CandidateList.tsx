import { useRef, useState } from 'react'
import {
  Alert,
  Anchor,
  Badge,
  Button,
  Chip,
  Group,
  Pagination,
  SegmentedControl,
  Select,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  UnstyledButton,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight, IconSearch } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import classes from './CandidateList.module.css'
import { useConfirmAction } from './ConfirmAction'
import { fmtNum, fmtSize, type ReleaseCandidate } from '../api'

const releaseKey = (c: ReleaseCandidate) => c.key || `${c.provider}/${c.siteId}/${c.guid}/${c.versionId ?? 0}`
const rangeLabel = (label: string, ranges?: [number, number][]) =>
  ranges?.length
    ? `${label} ${ranges.map(([from, to]) => (from === to ? from : `${from}–${to}`)).join(', ')}`
    : ''
const status = (c: ReleaseCandidate) =>
  c.decision?.status ?? (c.rejections?.length ? 'blocked' : 'ready')
const rank = { ready: 0, review: 1, blocked: 2 }

const PAGE_SIZE = 25

// A pack carries several books: by what it would collect here, or by the
// span its name states.
const spans = (ranges?: [number, number][]) =>
  (ranges ?? []).reduce((n, [from, to]) => n + Math.max(1, Math.floor(to - from) + 1), 0)
const isPack = (c: ReleaseCandidate) =>
  (c.coverage?.length ?? 0) > 1 ||
  !!c.parse.complete ||
  spans(c.parse.volumes) > 1 ||
  (!c.parse.volumes?.length && spans(c.parse.chapters) > 1)
const firstNumber = (c: ReleaseCandidate) =>
  c.parse.volumes?.[0]?.[0] ?? c.parse.chapters?.[0]?.[0] ?? Number.MAX_SAFE_INTEGER

// Short names for why a release was ruled out, used to group the blocked
// results. The full sentence stays on each row.
const reasonNames: Record<string, string> = {
  'rejected.series_match': 'Different series',
  'rejected.edition_routing': 'Different edition',
  'rejected.edition_ambiguous': 'Edition to confirm',
  'rejected.language': 'Wrong language',
  'rejected.content_type': 'Not manga',
  'rejected.numbers_unknown': 'No book number',
  'rejected.pack_unverified': 'Contents not confirmed',
  'rejected.pack_threshold': 'Mostly books you have',
  'rejected.previous_download': 'Download being replaced',
  'rejected.no_coverage': 'Not needed',
  'rejected.book_ambiguous': 'Ambiguous book',
  'rejected.blocklisted': 'On your blocklist',
  'rejected.target_group': 'Other release group',
}
const reasonCode = (c: ReleaseCandidate) => c.decision?.reasons[0]?.code ?? c.rejections?.[0] ?? ''
// Who decides a rejection, in the order the reasons are grouped.
const owners = [
  ['book', 'Not this book'],
  ['profile', 'Your profile'],
  ['confirm', 'To confirm'],
  ['other', 'Other'],
] as const
const reasonOwner = (c: ReleaseCandidate) => c.decision?.reasons[0]?.owner ?? 'other'
const reasonName = (code: string) => {
  const fallback = code.replace(/^rejected\./, '').replace(/_/g, ' ')
  return reasonNames[code] ?? fallback.charAt(0).toUpperCase() + fallback.slice(1)
}

export default function CandidateList({
  candidates,
  dropped = 0,
  onGrab,
  busy = false,
  searching = false,
  editionWide = false,
  selected,
  onToggle,
  showAll = false,
  allBooks = false,
}: {
  candidates: ReleaseCandidate[]
  dropped?: number
  onGrab?: (c: ReleaseCandidate, force: boolean) => void | Promise<void>
  busy?: boolean
  searching?: boolean
  // A whole-edition search mixes packs with single books: offer to
  // separate them and show what each release covers up front.
  editionWide?: boolean
  selected?: Set<string>
  onToggle?: (c: ReleaseCandidate) => void
  // Open on every release, ruled-out ones included: the question being
  // answered is why something is not on offer.
  showAll?: boolean
  // The search covers every book, not only the wanted ones.
  allBooks?: boolean
}) {
  const { confirm, confirmation } = useConfirmAction()
  const [queued, setQueued] = useState<Set<string>>(new Set())
  const downloading = useRef(false)
  const [active, setActive] = useState('')
  const [error, setError] = useState('')
  const [filter, setFilter] = useState('')
  // Matches are what the user came for; ruled-out results are one click away.
  const [state, setState] = useState<'matches' | 'blocked' | 'all'>(showAll ? 'all' : 'matches')
  const [reason, setReason] = useState<string | null>(null)
  const [kind, setKind] = useState<'any' | 'packs' | 'singles'>('any')
  const [site, setSite] = useState<string | null>(null)
  const [sort, setSort] = useState('best')
  // Filter and sort tools open on demand, and stay open while one is set.
  const [toolsOpen, setToolsOpen] = useState(false)
  const activeTools = [filter !== '', site !== null, sort !== 'best', kind !== 'any'].filter(Boolean).length
  const tools = toolsOpen || activeTools > 0
  const [page, setPage] = useState(1)
  const unique = [...new Map(candidates.map((c) => [releaseKey(c), c])).values()]
  const matches = unique.filter((c) => status(c) !== 'blocked')
  const blocked = unique.filter((c) => status(c) === 'blocked')
  const reasonCounts = new Map<string, number>()
  for (const c of blocked)
    reasonCounts.set(reasonCode(c), (reasonCounts.get(reasonCode(c)) ?? 0) + 1)
  const ownerOf = new Map(blocked.map((c) => [reasonCode(c), reasonOwner(c)]))
  const reasons = [...reasonCounts]
    .map(([code, count]) => ({ code, count, owner: ownerOf.get(code) ?? 'other' }))
    .sort((a, b) => b.count - a.count)
  // The reasons grouped by who decides them, empty groups left out.
  const reasonGroups = owners
    .map(([owner, label]) => ({ label, reasons: reasons.filter((r) => r.owner === owner) }))
    .filter((g) => g.reasons.length > 0)
  const resetPage = (fn: () => void) => {
    fn()
    setPage(1)
  }
  const showBlocked = (code: string | null) =>
    resetPage(() => {
      setState('blocked')
      setReason(code)
    })
  const shown = unique
    .filter((c) => {
      const text = [
        c.title,
        c.site,
        c.parse.group,
        c.scanlator,
        c.parse.language,
        c.editionSnapshot?.editionName,
        c.versionSnapshot?.name,
      ]
        .join(' ')
        .toLowerCase()
      return (
        (!filter.trim() || text.includes(filter.trim().toLowerCase())) &&
        (state === 'all' || (state === 'blocked') === (status(c) === 'blocked')) &&
        (state !== 'blocked' || !reason || reasonCode(c) === reason) &&
        (!site || c.site === site) &&
        (kind === 'any' || (kind === 'packs') === isPack(c))
      )
    })
    .sort((a, b) => {
      // Preserve arrival order while streaming so a result cannot move under a click.
      if (searching) return 0
      if (sort === 'number') return firstNumber(a) - firstNumber(b) || b.score - a.score
      if (sort === 'size') return (a.size ?? 0) - (b.size ?? 0)
      if (sort === 'seeders') return (b.seeders ?? -1) - (a.seeders ?? -1)
      if (sort === 'quality') return (b.formatScore ?? 0) - (a.formatScore ?? 0)
      return (
        rank[status(a)] - rank[status(b)] ||
        b.score - a.score ||
        // Indexer priority only breaks ties (lower wins), as the server does.
        (a.indexerPriority ?? 0) - (b.indexerPriority ?? 0) ||
        a.title.localeCompare(b.title) ||
        releaseKey(a).localeCompare(releaseKey(b))
      )
    })
  const pages = Math.max(1, Math.ceil(shown.length / PAGE_SIZE))
  const currentPage = Math.min(page, pages)
  const filtered = !!filter.trim() || !!site || kind !== 'any' || (state === 'blocked' && !!reason)
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
        // Name what is being overridden: the reasons themselves, whatever
        // they are (pack contents, edition, profile limits).
        title: 'Download this release anyway?',
        message: `It was held for review: ${(c.decision?.reasons ?? []).map((r) => r.message).join(' ')} Its books still have to match when it is imported.`,
        label: 'Download anyway',
        onConfirm: () => download(c, true),
      })
    else void download(c, false).catch(() => {}) // The result list retains the actionable error.
  }
  return (
    <Stack gap="sm">
      <Group justify="space-between" gap="xs">
        <SegmentedControl
          size="xs"
          aria-label="Release status"
          value={state}
          onChange={(v) =>
            resetPage(() => {
              setState(v as typeof state)
              setReason(null)
            })
          }
          data={[
            { value: 'matches', label: `Matches ${matches.length}` },
            { value: 'blocked', label: `Ruled out ${blocked.length}` },
            { value: 'all', label: `All ${unique.length}` },
          ]}
        />
        <Button
          size="compact-xs"
          variant="subtle"
          aria-expanded={tools}
          disabled={activeTools > 0}
          onClick={() => setToolsOpen(!toolsOpen)}
        >
          {activeTools > 0 ? `Filter and sort (${activeTools})` : 'Filter and sort'}
        </Button>
      </Group>
      {tools && (
        <Group gap="xs" className={classes.filters}>
          <TextInput
            aria-label="Filter releases"
            placeholder="Filter by title, group, language…"
            size="xs"
            leftSection={<IconSearch size={14} />}
            value={filter}
            onChange={(e) => resetPage(() => setFilter(e.currentTarget.value))}
            style={{ flex: 1, minWidth: 140, maxWidth: 320 }}
          />
          <Select
            aria-label="Release site"
            placeholder="All sites"
            size="xs"
            w={150}
            clearable
            value={site}
            onChange={(v) => resetPage(() => setSite(v))}
            data={[...new Set(unique.map((c) => c.site))].sort()}
          />
          <Select
            aria-label="Sort releases"
            size="xs"
            w={150}
            value={sort}
            onChange={(v) => resetPage(() => setSort(v ?? 'best'))}
            disabled={searching}
            allowDeselect={false}
            data={[
              { value: 'best', label: 'Best match' },
              ...(editionWide ? [{ value: 'number', label: 'Book number' }] : []),
              { value: 'quality', label: 'Format score' },
              { value: 'seeders', label: 'Most seeders' },
              { value: 'size', label: 'Smallest' },
            ]}
          />
        </Group>
      )}
      {tools && editionWide && state !== 'blocked' && (
        <Chip.Group value={kind} onChange={(v) => resetPage(() => setKind(v as typeof kind))}>
          <Group gap={6}>
            <Chip value="any" size="xs" variant="light">
              Packs and singles
            </Chip>
            <Chip value="packs" size="xs" variant="light">
              Packs {matches.filter(isPack).length}
            </Chip>
            <Chip value="singles" size="xs" variant="light">
              Single books {matches.filter((c) => !isPack(c)).length}
            </Chip>
          </Group>
        </Chip.Group>
      )}
      {state === 'blocked' && reasons.length > 1 && (
        <Chip.Group
          value={reason ?? 'any'}
          onChange={(v) =>
            resetPage(() => setReason(v === 'any' || typeof v !== 'string' ? null : v))
          }
        >
          <Group gap={6}>
            <Chip value="any" size="xs" variant="light">
              Any reason {blocked.length}
            </Chip>
            {reasonGroups.map((g) => (
              <Group key={g.label} gap={6}>
                <Text size="xs" c="dimmed">
                  {g.label}
                </Text>
                {g.reasons.map((r) => (
                  <Chip key={r.code} value={r.code} size="xs" variant="light">
                    {reasonName(r.code)} {r.count}
                  </Chip>
                ))}
              </Group>
            ))}
          </Group>
        </Chip.Group>
      )}
      {error && (
        <Alert
          color="red"
          title="Download could not be queued"
          style={{ overflowWrap: 'anywhere' }}
        >
          {error}
        </Alert>
      )}
      {shown.length === 0 && state === 'matches' && !filtered && (
        <div className={classes.empty}>
          <Text size="sm" fw={600}>
            {searching ? 'No matching release yet…' : 'No release matches this selection.'}
          </Text>
          {blocked.length > 0 && (
            <>
              <Text size="sm" c="dimmed" mt={4}>
                {blocked.length} {blocked.length === 1 ? 'result was' : 'results were'} ruled out:
              </Text>
              <Stack gap={4} mt="xs">
                {reasonGroups.map((g) => (
                  <Group key={g.label} gap={6}>
                    <Text size="xs" c="dimmed" w={90}>
                      {g.label}
                    </Text>
                    {g.reasons.map((r) => (
                      <Button
                        key={r.code}
                        size="compact-xs"
                        variant="default"
                        onClick={() => showBlocked(r.code)}
                      >
                        {reasonName(r.code)} · {r.count}
                      </Button>
                    ))}
                  </Group>
                ))}
              </Stack>
            </>
          )}
        </div>
      )}
      {shown.length === 0 && (state !== 'matches' || filtered) && (
        <div className={classes.empty}>
          <Text size="sm">No releases match these filters.</Text>
          <Button
            variant="subtle"
            size="compact-xs"
            mt="xs"
            onClick={() => {
              setFilter('')
              setSite(null)
              setKind('any')
              setReason(null)
              setPage(1)
            }}
          >
            Clear filters
          </Button>
        </div>
      )}
      {shown.length > 0 && (
        <div className={classes.rows}>
          {shown.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE).map((c) => (
            <ReleaseRow
              key={releaseKey(c)}
              c={c}
              queued={queued.has(releaseKey(c))}
              busy={busy || !!active}
              showBooks={editionWide}
              allBooks={allBooks}
              loading={active === releaseKey(c)}
              onGrab={onGrab ? requestDownload : undefined}
              selected={selected?.has(c.key)}
              onToggle={onToggle ? () => onToggle(c) : undefined}
            />
          ))}
        </div>
      )}
      <Group justify="space-between">
        <Text size="xs" c="dimmed">
          {shown.length > 0 && `${shown.length} ${shown.length === 1 ? 'release' : 'releases'}`}
          {shown.length > 0 && dropped > 0 && ' · '}
          {dropped > 0 && `${dropped} results for the other book type not shown`}
        </Text>
        {pages > 1 && <Pagination size="sm" total={pages} value={currentPage} onChange={setPage} />}
      </Group>
      {confirmation}
    </Stack>
  )
}

function ReleaseRow({
  c,
  queued,
  showBooks = false,
  allBooks = false,
  busy,
  loading,
  onGrab,
  selected,
  onToggle,
}: {
  c: ReleaseCandidate
  queued: boolean
  showBooks?: boolean
  allBooks?: boolean
  busy: boolean
  loading: boolean
  onGrab?: (c: ReleaseCandidate, force: boolean) => void
  selected?: boolean
  onToggle?: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const decision = c.decision
  const state = status(c)
  const volume = rangeLabel('Vol.', c.parse.volumes)
  const chapter = rangeLabel('Ch.', c.parse.chapters)
  // Edition-wide, what a release covers is the first thing to compare.
  const books = showBooks ? [volume, chapter].filter(Boolean).join(' · ') : ''
  const facts = [
    !books && volume,
    !books && chapter,
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
  const more = (decision?.reasons.length ?? 1) - 1
  const matched = c.coverage?.length ?? 0
  return (
    <div className={classes.row} data-testid="release-row" data-status={queued ? 'queued' : state}>
      <Group gap="sm" align="flex-start" className={classes.rowMain}>
        <UnstyledButton
          className={classes.summary}
          aria-expanded={expanded}
          onClick={() => setExpanded(!expanded)}
        >
          <Group gap={6} wrap="nowrap" align="flex-start">
            {expanded ? (
              <IconChevronDown size={14} className={classes.chevron} />
            ) : (
              <IconChevronRight size={14} className={classes.chevron} />
            )}
            <div style={{ minWidth: 0, flex: 1 }}>
              <Group gap={6} wrap="nowrap" align="baseline">
                {books && (
                  <Badge variant="light" color="gray" className={classes.books}>
                    {books}
                  </Badge>
                )}
                <Text
                  fw={600}
                  size="sm"
                  lineClamp={expanded ? undefined : 1}
                  style={{ overflowWrap: 'anywhere' }}
                >
                  {c.title}
                </Text>
              </Group>
              <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                {[
                  c.site,
                  c.protocol === 'torrent' ? 'Torrent' : 'Direct download',
                  c.protocol === 'torrent' && (c.size ? fmtSize(c.size) : 'Size unknown'),
                  c.protocol === 'torrent' && `${c.seeders ?? 0} seeders`,
                  ...facts,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
              {state === 'blocked' ? (
                <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                  {reason ?? 'Ruled out'}
                  {more > 0 && ` (+${more} more)`}
                </Text>
              ) : (
                <Text size="xs" c={state === 'review' ? 'orange' : 'dimmed'}>
                  {state === 'review' && `${reason}${more > 0 ? ` (+${more} more)` : ''} · `}
                  {matched > 0
                    ? `${matched}${allBooks ? '' : ' wanted'} ${matched === 1 ? 'book' : 'books'}`
                    : decision?.kind || 'Unclassified'}
                  {state === 'ready' && ` · Rank score ${c.score}`}
                  {c.editionSnapshot && ` · ${c.editionSnapshot.editionName}`}
                  {c.versionSnapshot && !c.versionSnapshot.isDefault && ` · ${c.versionSnapshot.name}`}
                </Text>
              )}
            </div>
          </Group>
        </UnstyledButton>
        {queued ? (
          <Badge variant="light" color="blue">
            Queued
          </Badge>
        ) : onToggle && state === 'ready' ? (
          // Inside a download plan a ready release joins the plan instead
          // of starting its own download.
          <Button size="xs" variant={selected ? 'light' : 'default'} disabled={busy} aria-pressed={selected ?? false} onClick={onToggle}>
            {selected ? 'In plan' : 'Add to plan'}
          </Button>
        ) : onGrab && state !== 'blocked' ? (
          <Button
            size="xs"
            variant={state === 'review' ? 'light' : 'filled'}
            color={state === 'review' ? 'orange' : 'brand'}
            disabled={busy || !(decision?.canDownload || decision?.canForce)}
            loading={loading}
            onClick={() => onGrab(c, state === 'review')}
          >
            {state === 'review' ? 'Review & download' : 'Download'}
          </Button>
        ) : (
          state === 'blocked' && (
            <Badge variant="light" color="gray" className={classes.reasonBadge}>
              {reasonName(reasonCode(c))}
            </Badge>
          )
        )}
      </Group>
      {expanded && (
        <Stack gap="sm" className={classes.details}>
          {!!decision?.reasons.length && (
            <Stack gap={4}>
              {decision.reasons.map((r) => (
                <Text key={r.code} size="sm" style={{ overflowWrap: 'anywhere' }}>
                  {r.message}{' '}
                  <Text span size="xs" c="dimmed">
                    {r.owner === 'book'
                      ? 'Not this book: nothing to change.'
                      : r.override
                        ? 'Can be overridden for this download.'
                        : 'Cannot be overridden.'}
                  </Text>
                  {r.owner === 'profile' && !!c.profileId && (
                    <>
                      {' '}
                      <Anchor component={Link} to={`/profiles?profile=${c.profileId}`} size="xs">
                        Open profile
                      </Anchor>
                    </>
                  )}
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
              {c.formats.map((f) => `${f.name} (${f.score > 0 ? '+' : ''}${f.score})`).join(' · ')}
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
    </div>
  )
}
