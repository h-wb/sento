import { useState, type Dispatch, type SetStateAction } from 'react'
import {
  ActionIcon,
  Button,
  Chip,
  Group,
  NumberInput,
  Popover,
  SegmentedControl,
  Text,
  Tooltip,
} from '@mantine/core'
import { IconSortAscendingNumbers, IconSortDescendingNumbers } from '@tabler/icons-react'
import type { BookStatus } from '../../types/service'
import { filters } from './bookStatus'
import type { BookRange, BookSelectionState } from './useBookSelection'
import classes from '../EditionBooks.module.css'

export function BooksToolbar({
  filter,
  onFilter,
  all,
  books,
  view,
  setViewChoice,
  selecting,
  setSelecting,
  selection,
  noun,
  jump,
  order,
  setOrder,
  busy,
  monitor,
}: {
  filter: string
  onFilter: (filter: string) => void
  all: BookStatus[]
  books: BookStatus[]
  view: 'grid' | 'list'
  setViewChoice: Dispatch<SetStateAction<'grid' | 'list' | null>>
  selecting: boolean
  setSelecting: Dispatch<SetStateAction<boolean>>
  selection: BookSelectionState
  noun: string
  jump: (goTo: string | number) => void
  order: 'newest' | 'oldest'
  setOrder: (order: 'newest' | 'oldest') => void
  busy: boolean
  monitor: (list: BookStatus[], monitored: boolean) => void
}) {
  const { picked, clearSelection } = selection
  const [goTo, setGoTo] = useState<string | number>('')
  const [range, setRange] = useState<BookRange>({ from: '', to: '' })
  const [rangeOpen, setRangeOpen] = useState(false)
  const selectRange = () => {
    if (selection.selectRange(range)) setRangeOpen(false)
  }
  return (
    <>
      <Group justify="space-between" gap="xs">
        <Chip.Group value={filter} onChange={(v) => onFilter(typeof v === 'string' ? v : 'all')}>
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
          {/* The list has a checkbox per book; tiles need a mode to select. */}
          {view === 'grid' && (
            <Button
              size="compact-xs"
              variant={selecting ? 'light' : 'subtle'}
              aria-pressed={selecting}
              title="Click books to select them. Shift-click selects a range; Ctrl/Cmd-click works without this mode."
              onClick={() => setSelecting(!selecting)}
            >
              {selecting ? 'Done selecting' : 'Select'}
            </Button>
          )}
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
            onKeyDown={(e) => e.key === 'Enter' && jump(goTo)}
          />
          <Tooltip label={order === 'newest' ? 'Newest first' : 'Oldest first'}>
            <ActionIcon
              variant="default"
              size="md"
              aria-label={
                order === 'newest'
                  ? 'Newest first; show oldest first'
                  : 'Oldest first; show newest first'
              }
              onClick={() => setOrder(order === 'newest' ? 'oldest' : 'newest')}
            >
              {order === 'newest' ? (
                <IconSortDescendingNumbers size={16} />
              ) : (
                <IconSortAscendingNumbers size={16} />
              )}
            </ActionIcon>
          </Tooltip>
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
      {((selecting && view === 'grid') || picked.length > 0 || filter !== 'all') && (
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
    </>
  )
}
