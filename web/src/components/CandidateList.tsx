import {
  Alert,
  Button,
  Chip,
  Group,
  Pagination,
  SegmentedControl,
  Select,
  Stack,
  Text,
  TextInput,
} from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { useRef, useState } from 'react'
import type { ReleaseCandidate } from '../api'
import { plural } from '../format'
import { errorMessage } from '../ui'
import classes from './CandidateList.module.css'
import { useConfirmAction } from './ConfirmAction'
import { firstNumber, isPack, owners, rank, reasonCode, reasonName, reasonOwner, releaseKey, status } from './search/releaseModel'
import { ReleaseRow } from './search/ReleaseRow'
import { usePaged } from './usePaged'

const PAGE_SIZE = 25

export default function CandidateList({
  candidates,
  dropped = 0,
  onGrab,
  onUnblock,
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
  onUnblock?: (c: ReleaseCandidate) => Promise<void>
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
  const { page, setPage, pages, slice } = usePaged(shown, PAGE_SIZE)
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
      setError(errorMessage(e))
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
  const unblock = async (c: ReleaseCandidate) => {
    if (!onUnblock || busy || searching || downloading.current) return
    downloading.current = true
    setActive(releaseKey(c))
    setError('')
    try { await onUnblock(c) }
    catch (e) { setError(errorMessage(e)) }
    finally { downloading.current = false; setActive('') }
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
          title="Release action failed"
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
                {blocked.length} {plural(blocked.length, 'result was', 'results were')} ruled out:
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
          {slice.map((c) => (
            <ReleaseRow
              key={releaseKey(c)}
              c={c}
              queued={queued.has(releaseKey(c))}
              busy={busy || !!active}
              showBooks={editionWide}
              allBooks={allBooks}
              loading={active === releaseKey(c)}
              onGrab={onGrab ? requestDownload : undefined}
              onUnblock={onUnblock ? () => void unblock(c) : undefined}
              unblockDisabled={searching}
              selected={selected?.has(c.key)}
              onToggle={onToggle ? () => onToggle(c) : undefined}
            />
          ))}
        </div>
      )}
      <Group justify="space-between">
        <Text size="xs" c="dimmed">
          {shown.length > 0 && `${shown.length} ${plural(shown.length, 'release')}`}
          {shown.length > 0 && dropped > 0 && ' · '}
          {dropped > 0 && `${dropped} results for the other book type not shown`}
        </Text>
        {pages > 1 && <Pagination size="sm" total={pages} value={page} onChange={setPage} />}
      </Group>
      {confirmation}
    </Stack>
  )
}
