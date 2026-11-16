import {
  Pagination,
  Stack,
  Text,
} from '@mantine/core'
import { useLocalStorage } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useRef, useState, type MouseEvent } from 'react'
import { api } from '../api'
import { toHundredths } from '../format'
import { seriesQueries } from '../queries'
import type { BookStatus } from '../types/service'
import type {
  AssetCoverage,
  Book,
  CollectedVersion,
  Edition,
  File as SeriesFile,
} from '../types/store'
import type { BookSelection } from './BookDialog'
import { bookSelection } from './bookSelection'
import { BookGrid } from './books/BookGrid'
import { BookRow } from './books/BookRow'
import { filters } from './books/bookStatus'
import { BooksToolbar } from './books/BooksToolbar'
import { SplitBoxDialog } from './books/SplitBoxDialog'
import { useBookSelection } from './books/useBookSelection'
import classes from './EditionBooks.module.css'
import { QueryFeedback } from './QueryFeedback'
import { usePaged } from './usePaged'

// Editions longer than this open as the overview grid; shorter ones (most
// volume runs) open as the list, where every book shows its details.
const GRID_FROM = 50
const PAGE_SIZE = 100

export default function EditionBooks({
  seriesID,
  edition: c,
  version: t,
  label,
  files,
  coverage,
  onBook,
  onEdit,
  onFile,
  run,
  busy,
  explained,
}: {
  seriesID: number
  edition: Edition
  version: CollectedVersion
  // The card's name in a search opened from it.
  label: string
  files: SeriesFile[]
  coverage: AssetCoverage[]
  onBook: (s: BookSelection) => void
  onEdit: (w: Book) => void
  onFile: (f: SeriesFile) => void
  run: (fn: () => Promise<unknown>) => Promise<void>
  busy: boolean
  // A reason the card already states above its books: rows keep it on
  // hover instead of repeating it on every line.
  explained?: string
}) {
  const noun = c.type === 'chapter' ? 'chapter' : 'volume'
  const [filter, setFilter] = useState('all')
  const [viewChoice, setViewChoice] = useState<'grid' | 'list' | null>(null)
  const [selecting, setSelecting] = useState(false)
  const [flash, setFlash] = useState<number | null>(null)
  const [split, setSplit] = useState<{ book: Book; count: string | number } | null>(null)
  const container = useRef<HTMLDivElement>(null)

  const q = useQuery(seriesQueries.books(seriesID, t.id))
  // Newest first by default: new and wanted books are what is looked for.
  // Remembered per ledger, volumes and chapters apart.
  const [order, setOrder] = useLocalStorage<'newest' | 'oldest'>({
    key: `sento.bookOrder.${noun}`,
    defaultValue: 'newest',
  })
  const all = useMemo(
    () => (order === 'newest' ? [...(q.data ?? [])].reverse() : (q.data ?? [])),
    [q.data, order],
  )
  const active = filters.find((f) => f.value === filter) ?? filters[0]
  const books = useMemo(() => all.filter(active.match), [all, active])
  const view = viewChoice ?? (all.length > GRID_FROM ? 'grid' : 'list')
  const { page, setPage, pages, slice } = usePaged(books, PAGE_SIZE)
  const shown = view === 'list' ? slice : books
  const selection = useBookSelection(all, books)
  const { selected, clearSelection, pick } = selection
  const monitor = (list: BookStatus[], monitored: boolean) =>
    void run(() =>
      api.bulkMonitor(
        seriesID,
        t.id,
        list.map((s) => s.book.id),
        monitored,
      ),
    ).then(clearSelection)

  const open = (e: MouseEvent, s: BookStatus) => {
    if ((selecting && view === 'grid') || e.shiftKey || e.metaKey || e.ctrlKey) {
      e.preventDefault()
      pick(s, e.shiftKey)
      return
    }
    onBook(bookSelection(c, t, label, s))
  }

  const jump = (goTo: string | number) => {
    const wantedNumber = Number(goTo)
    if (goTo === '' || Number.isNaN(wantedNumber)) return
    // The first book numbered at least that, in either order.
    const at = (list: BookStatus[]) => {
      const target = list
        .filter((s) => s.book.numberKnown && s.book.sequenceNum >= toHundredths(wantedNumber))
        .reduce<BookStatus | null>((low, s) => (!low || s.book.sequenceNum < low.book.sequenceNum ? s : low), null)
      return target ? list.indexOf(target) : -1
    }
    let list = books
    let index = at(list)
    if (index < 0 && filter !== 'all') {
      list = all
      index = at(list)
      if (index >= 0) setFilter('all')
    }
    if (index < 0) return
    const id = list[index].book.id
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
      <BooksToolbar
        filter={filter}
        onFilter={(v) => {
          setFilter(v)
          setPage(1)
        }}
        all={all}
        books={books}
        view={view}
        setViewChoice={setViewChoice}
        selecting={selecting}
        setSelecting={setSelecting}
        selection={selection}
        noun={noun}
        jump={jump}
        order={order}
        setOrder={setOrder}
        busy={busy}
        monitor={monitor}
      />

      {!books.length && (
        <Text size="sm" c="dimmed">
          No books match this filter.
        </Text>
      )}

      {view === 'grid' && books.length > 0 && (
        <BookGrid shown={shown} selected={selected} flash={flash} noun={noun} open={open} />
      )}

      {view === 'list' && books.length > 0 && (
        <div className={classes.rows}>
          {shown.map((s) => <BookRow key={s.book.id} s={s} coverage={coverage} t={t} c={c} selected={selected} flash={flash} noun={noun} pick={pick} open={open} explained={explained} busy={busy} run={run} seriesID={seriesID} onEdit={onEdit} setSplit={setSplit} files={files} onFile={onFile} />)}
        </div>
      )}
      {view === 'list' && pages > 1 && (
        <Pagination size="sm" total={pages} value={page} onChange={setPage} />
      )}
      {split && (
        <SplitBoxDialog split={split} setSplit={setSplit} busy={busy} run={run} seriesID={seriesID} />
      )}
    </Stack>
  )
}
