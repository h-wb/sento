import { useMemo, useRef, useState, type MouseEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ActionIcon,
  Badge,
  Button,
  Checkbox,
  Chip,
  Group,
  Menu,
  Modal,
  NumberInput,
  Pagination,
  Popover,
  SegmentedControl,
  Stack,
  Text,
} from '@mantine/core'
import { IconDots } from '@tabler/icons-react'
import { api } from '../api'
import type {
  Collection,
  DownloadTarget,
  Work,
  File as SeriesFile,
  AssetCoverage,
} from '../types/store'
import type { UnitSlot } from '../types/service'
import type { SlotSelection } from './SlotModal'
import { QueryFeedback } from './QueryFeedback'
import classes from './EditionBooks.module.css'

const labels: Record<string, string> = {
  owned: 'Owned',
  pending: 'Pending',
  missing: 'Missing',
  not_released: 'Upcoming',
  unmonitored: 'Paused',
  unknown: 'Date unknown',
  upgrade_eligible: 'Upgrade wanted',
  unsatisfactory: 'Different copy on disk',
  needs_review: 'Needs review',
  storage_unavailable: 'Storage unavailable',
}

// Editions longer than this open as the overview grid; shorter ones (most
// volume runs) open as the list, where every book shows its details.
const GRID_FROM = 50
const PAGE_SIZE = 100

type Tone = 'disk' | 'wanted' | 'pending' | 'review' | 'upcoming' | 'paused'

// tone is the one colour a book gets in the grid and the status badge.
function tone(s: UnitSlot): Tone {
  switch (s.state) {
    case 'pending':
      return 'pending'
    case 'needs_review':
    case 'storage_unavailable':
    case 'unsatisfactory':
      return 'review'
    case 'owned':
    case 'upgrade_eligible':
      return 'disk'
    case 'missing':
      return 'wanted'
    case 'unmonitored':
      return 'paused'
    default:
      return 'upcoming'
  }
}

const badgeColor: Record<Tone, string> = {
  disk: 'green',
  wanted: 'red',
  pending: 'blue',
  review: 'yellow',
  upcoming: 'gray',
  paused: 'gray',
}

type Filter = {
  value: string
  label: string
  tone?: Tone
  always?: boolean
  match: (s: UnitSlot) => boolean
}

const filters: Filter[] = [
  { value: 'all', label: 'All', always: true, match: () => true },
  {
    value: 'on_disk',
    label: 'On disk',
    tone: 'disk',
    always: true,
    match: (s) => (s.onDiskFileIds ?? []).length > 0,
  },
  { value: 'wanted', label: 'Wanted', tone: 'wanted', always: true, match: (s) => s.wanted },
  { value: 'pending', label: 'Downloading', tone: 'pending', match: (s) => s.state === 'pending' },
  { value: 'upcoming', label: 'Upcoming', tone: 'upcoming', match: (s) => tone(s) === 'upcoming' },
  {
    value: 'unmonitored',
    label: 'Paused',
    tone: 'paused',
    match: (s) => s.state === 'unmonitored',
  },
  { value: 'review', label: 'Needs attention', tone: 'review', match: (s) => tone(s) === 'review' },
]

function bookSelection(c: Collection, t: DownloadTarget, s: UnitSlot): SlotSelection {
  return {
    ledger: c.type === 'chapter' ? 'chapter' : 'volume',
    collectionId: c.id,
    targetId: t.id,
    workId: s.work.id,
    number: s.work.sequenceNum,
    label: s.work.sequenceStr,
    collectionName: `${c.editionName} · ${t.name}`,
    fileIds: s.fileIds,
    monitored: s.monitored,
    wanted: s.wanted,
  }
}

export default function EditionBooks({
  seriesID,
  edition: c,
  target: t,
  files,
  coverage,
  onSlot,
  onEdit,
  onFile,
  run,
  busy,
}: {
  seriesID: number
  edition: Collection
  target: DownloadTarget
  files: SeriesFile[]
  coverage: AssetCoverage[]
  onSlot: (s: SlotSelection) => void
  onEdit: (w: Work) => void
  onFile: (f: SeriesFile) => void
  run: (fn: () => Promise<unknown>) => Promise<void>
  busy: boolean
}) {
  const noun = c.type === 'chapter' ? 'chapter' : 'volume'
  const [filter, setFilter] = useState('all')
  const [viewChoice, setViewChoice] = useState<'grid' | 'list' | null>(null)
  const [page, setPage] = useState(1)
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [anchor, setAnchor] = useState<number | null>(null)
  const [flash, setFlash] = useState<number | null>(null)
  const [goTo, setGoTo] = useState<string | number>('')
  const [range, setRange] = useState<{ from: string | number; to: string | number }>({
    from: '',
    to: '',
  })
  const [rangeOpen, setRangeOpen] = useState(false)
  const [split, setSplit] = useState<{ work: Work; count: string | number } | null>(null)
  const container = useRef<HTMLDivElement>(null)

  const q = useQuery({
    queryKey: ['series', seriesID, 'books', t.id],
    queryFn: () => api.targetBooks(seriesID, t.id),
  })
  const all = useMemo(() => q.data ?? [], [q.data])
  const active = filters.find((f) => f.value === filter) ?? filters[0]
  const books = useMemo(() => all.filter(active.match), [all, active])
  const view = viewChoice ?? (all.length > GRID_FROM ? 'grid' : 'list')
  const pages = Math.max(1, Math.ceil(books.length / PAGE_SIZE))
  const current = Math.min(page, pages)
  const shown =
    view === 'list' ? books.slice((current - 1) * PAGE_SIZE, current * PAGE_SIZE) : books
  // A selection only ever acts on books that still exist.
  const picked = all.filter((s) => selected.has(s.work.id))

  const clearSelection = () => {
    setSelected(new Set())
    setAnchor(null)
  }
  const monitor = (slots: UnitSlot[], monitored: boolean) =>
    void run(() =>
      api.bulkMonitor(
        seriesID,
        t.id,
        slots.map((s) => s.work.id),
        monitored,
      ),
    ).then(clearSelection)

  // Shift extends from the last picked book across the visible order;
  // Ctrl/Cmd (or Select mode) toggles one book.
  const pick = (s: UnitSlot, shift: boolean) => {
    const next = new Set(selected)
    const from = books.findIndex((b) => b.work.id === anchor)
    const to = books.findIndex((b) => b.work.id === s.work.id)
    if (shift && from >= 0 && to >= 0) {
      for (const b of books.slice(Math.min(from, to), Math.max(from, to) + 1)) next.add(b.work.id)
    } else if (next.has(s.work.id)) {
      next.delete(s.work.id)
    } else {
      next.add(s.work.id)
    }
    setSelected(next)
    setAnchor(s.work.id)
  }
  const open = (e: MouseEvent, s: UnitSlot) => {
    if (selecting || e.shiftKey || e.metaKey || e.ctrlKey) {
      e.preventDefault()
      pick(s, e.shiftKey)
      return
    }
    onSlot(bookSelection(c, t, s))
  }

  const jump = () => {
    const wantedNumber = Number(goTo)
    if (goTo === '' || Number.isNaN(wantedNumber)) return
    const at = (list: UnitSlot[]) =>
      list.findIndex(
        (s) => s.work.numberKnown && s.work.sequenceNum >= Math.round(wantedNumber * 100),
      )
    let list = books
    let index = at(list)
    if (index < 0 && filter !== 'all') {
      list = all
      index = at(list)
      if (index >= 0) setFilter('all')
    }
    if (index < 0) return
    const id = list[index].work.id
    setPage(Math.floor(index / PAGE_SIZE) + 1)
    setFlash(id)
    window.setTimeout(() => setFlash((f) => (f === id ? null : f)), 2500)
    // Wait for the page/filter change to render before scrolling to it.
    window.setTimeout(
      () =>
        container.current
          ?.querySelector(`[data-book="${id}"]`)
          ?.scrollIntoView({ block: 'center', behavior: 'smooth' }),
      50,
    )
  }

  const selectRange = () => {
    const from = Number(range.from)
    const to = Number(range.to)
    if (range.from === '' || range.to === '' || Number.isNaN(from) || Number.isNaN(to)) return
    const low = Math.round(Math.min(from, to) * 100)
    const high = Math.round(Math.max(from, to) * 100)
    setSelected(
      new Set(
        all
          .filter(
            (s) => s.work.numberKnown && s.work.sequenceNum >= low && s.work.sequenceNum <= high,
          )
          .map((s) => s.work.id),
      ),
    )
    setAnchor(null)
    setRangeOpen(false)
  }

  const title = (s: UnitSlot) =>
    `${noun[0].toUpperCase()}${noun.slice(1)} ${s.work.sequenceStr} · ${labels[s.state] ?? s.state}${s.reason ? ` · ${s.reason}` : ''}`

  if (q.isPending || q.error) {
    return <QueryFeedback pending={q.isPending} error={q.error} onRetry={() => void q.refetch()} />
  }
  if (!all.length) {
    return (
      <Text size="sm" c="dimmed">
        No {noun}s added yet.
      </Text>
    )
  }
  return (
    <Stack gap="xs" ref={container}>
      <Group justify="space-between" gap="xs">
        <Chip.Group
          value={filter}
          onChange={(v) => {
            setFilter(typeof v === 'string' ? v : 'all')
            setPage(1)
          }}
        >
          <Group gap={6}>
            {filters.map((f) => {
              const count = all.filter(f.match).length
              if (!f.always && count === 0 && f.value !== filter) return null
              return (
                <Chip key={f.value} value={f.value} size="xs" variant="light">
                  {f.tone && <span className={classes.dot} data-tone={f.tone} />}
                  {f.label} {count}
                </Chip>
              )
            })}
          </Group>
        </Chip.Group>

        <Group gap="xs">
          <Button
            size="compact-xs"
            variant={selecting ? 'light' : 'subtle'}
            aria-pressed={selecting}
            title="Click books to select them. Shift-click selects a range; Ctrl/Cmd-click works without this mode."
            onClick={() => setSelecting(!selecting)}
          >
            {selecting ? 'Done selecting' : 'Select'}
          </Button>
          <Popover
            opened={rangeOpen}
            onChange={setRangeOpen}
            position="bottom-start"
            withArrow
            trapFocus
          >
            <Popover.Target>
              <Button size="compact-xs" variant="subtle" onClick={() => setRangeOpen(!rangeOpen)}>
                Select range
              </Button>
            </Popover.Target>
            <Popover.Dropdown>
              <Group gap="xs" align="flex-end">
                <NumberInput
                  label="From"
                  size="xs"
                  w={90}
                  min={0}
                  hideControls
                  value={range.from}
                  onChange={(from) => setRange({ ...range, from })}
                />
                <NumberInput
                  label="To"
                  size="xs"
                  w={90}
                  min={0}
                  hideControls
                  value={range.to}
                  onChange={(to) => setRange({ ...range, to })}
                  onKeyDown={(e) => e.key === 'Enter' && selectRange()}
                />
                <Button size="xs" onClick={selectRange}>
                  Select
                </Button>
              </Group>
            </Popover.Dropdown>
          </Popover>
          <NumberInput
            aria-label={`Go to ${noun} number`}
            placeholder={`Go to ${noun}…`}
            size="xs"
            w={130}
            min={0}
            hideControls
            value={goTo}
            onChange={setGoTo}
            onKeyDown={(e) => e.key === 'Enter' && jump()}
          />
          <SegmentedControl
            size="xs"
            aria-label="Book layout"
            value={view}
            onChange={(v) => setViewChoice(v as 'grid' | 'list')}
            data={[
              { value: 'grid', label: 'Grid' },
              { value: 'list', label: 'List' },
            ]}
          />
        </Group>
      </Group>

      {/* Bulk actions stay out of the way until a filter or a selection gives them a subject. */}
      {(selecting || picked.length > 0 || filter !== 'all') && (
        <Group gap="xs">
          {picked.length > 0 ? (
            <>
              <Text size="xs" fw={600}>
                {picked.length} selected
              </Text>
              <Button
                size="compact-xs"
                variant="light"
                disabled={busy}
                onClick={() => monitor(picked, true)}
              >
                Collect
              </Button>
              <Button
                size="compact-xs"
                variant="light"
                disabled={busy}
                onClick={() => monitor(picked, false)}
              >
                Pause
              </Button>
              <Button size="compact-xs" variant="subtle" onClick={clearSelection}>
                Clear
              </Button>
            </>
          ) : (
            <>
              <Button
                size="compact-xs"
                variant="subtle"
                disabled={busy || !books.length}
                onClick={() => monitor(books, true)}
              >
                Collect {filter === 'all' ? 'all' : 'these'} {books.length}
              </Button>
              <Button
                size="compact-xs"
                variant="subtle"
                disabled={busy || !books.length}
                onClick={() => monitor(books, false)}
              >
                Pause {filter === 'all' ? 'all' : 'these'} {books.length}
              </Button>
            </>
          )}
        </Group>
      )}

      {!books.length && (
        <Text size="sm" c="dimmed">
          No books match this filter.
        </Text>
      )}

      {view === 'grid' && books.length > 0 && (
        <div className={classes.grid}>
          {shown.map((s) => (
            <button
              key={s.work.id}
              type="button"
              className={classes.cell}
              data-book={s.work.id}
              data-tone={tone(s)}
              data-selected={selected.has(s.work.id)}
              data-flash={flash === s.work.id}
              title={title(s)}
              onClick={(e) => open(e, s)}
            >
              {s.work.sequenceStr}
            </button>
          ))}
        </div>
      )}

      {view === 'list' && books.length > 0 && (
        <div className={classes.rows}>
          {shown.map((s) => {
            const onDisk = s.onDiskFileIds ?? s.fileIds
            const pins = s.fileIds
              .map((fid) =>
                coverage.find(
                  (a) => a.fileId === fid && a.targetId === t.id && a.workId === s.work.id,
                ),
              )
              .filter((a): a is AssetCoverage => !!a)
            return (
              <div
                key={s.work.id}
                className={classes.row}
                data-book={s.work.id}
                data-selected={selected.has(s.work.id)}
                data-flash={flash === s.work.id}
              >
                <Checkbox
                  size="xs"
                  aria-label={`Collect ${c.type} ${s.work.sequenceStr}`}
                  checked={s.monitored}
                  disabled={busy}
                  onChange={(e) =>
                    void run(() =>
                      api.unitMonitor(seriesID, t.id, s.work.id, e.currentTarget.checked),
                    )
                  }
                />
                <Button size="compact-sm" variant="subtle" miw={44} onClick={(e) => open(e, s)}>
                  {s.work.sequenceStr}
                </Button>
                <Text size="xs" c="dimmed" className={classes.rowDetail}>
                  {s.work.countType !== 'main' &&
                    (s.work.countType === 'extra' ? 'Bonus / special · ' : 'Other · ')}
                  {s.reason || s.work.releaseDate || 'Publication date unknown'}
                </Text>
                <Badge size="xs" variant="light" color={badgeColor[tone(s)]}>
                  {labels[s.state] ?? s.state}
                </Badge>
                <Menu position="bottom-end" withinPortal>
                  <Menu.Target>
                    <ActionIcon
                      size="sm"
                      variant="subtle"
                      color="gray"
                      aria-label={`Actions for ${noun} ${s.work.sequenceStr}`}
                    >
                      <IconDots size={14} />
                    </ActionIcon>
                  </Menu.Target>
                  <Menu.Dropdown>
                    <Menu.Item onClick={() => onEdit(s.work)}>Edit</Menu.Item>
                    {c.type === 'volume' && (
                      <Menu.Item onClick={() => setSplit({ work: s.work, count: 2 })}>
                        This is a box of several volumes…
                      </Menu.Item>
                    )}
                    {onDisk.length > 0 && (
                      <Menu.Item
                        onClick={() => {
                          const f = files.find((f) => f.id === onDisk[0])
                          if (f) onFile(f)
                        }}
                      >
                        View files
                      </Menu.Item>
                    )}
                    {pins.map((a) => (
                      <Menu.Item
                        key={a.fileId}
                        disabled={busy}
                        onClick={() =>
                          void run(() => api.pinCoverage(seriesID, { ...a, pinned: !a.pinned }))
                        }
                      >
                        {a.pinned ? 'Unpin copy' : 'Pin copy'}
                      </Menu.Item>
                    ))}
                  </Menu.Dropdown>
                </Menu>
              </div>
            )
          })}
        </div>
      )}
      {view === 'list' && pages > 1 && (
        <Pagination size="sm" total={pages} value={current} onChange={setPage} />
      )}
      {split && (
        <Modal
          opened
          onClose={() => setSplit(null)}
          title={`Split "${split.work.sequenceStr}" into volumes`}
        >
          <Stack gap="sm">
            <Text size="sm" c="dimmed">
              Catalogs list a box set as one item, but releases are named after the books inside it.
              This adds volumes 1 to N to this edition, with the box's ISBN and date, and stops
              collecting the box itself.
            </Text>
            <NumberInput
              label="Volumes in the box"
              min={2}
              max={100}
              value={split.count}
              onChange={(count) => setSplit({ ...split, count })}
            />
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setSplit(null)}>
                Cancel
              </Button>
              <Button
                disabled={busy || Number(split.count) < 2}
                onClick={() =>
                  void run(() => api.splitBox(seriesID, split.work.id, Number(split.count))).then(
                    () => setSplit(null),
                  )
                }
              >
                Add volumes
              </Button>
            </Group>
          </Stack>
        </Modal>
      )}
    </Stack>
  )
}
