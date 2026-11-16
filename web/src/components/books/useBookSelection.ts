import { useState } from 'react'
import type { BookStatus } from '../../types/service'
import { booksInRange } from '../bookRanges'

export type BookRange = { from: string | number; to: string | number }

export function useBookSelection(all: BookStatus[], books: BookStatus[]) {
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [anchor, setAnchor] = useState<number | null>(null)
  // A selection only ever acts on books that still exist.
  const picked = all.filter((s) => selected.has(s.book.id))

  const clearSelection = () => {
    setSelected(new Set())
    setAnchor(null)
  }
  // Shift extends from the last picked book across the visible order;
  // Ctrl/Cmd (or Select mode) toggles one book.
  const pick = (s: BookStatus, shift: boolean) => {
    const next = new Set(selected)
    const from = books.findIndex((b) => b.book.id === anchor)
    const to = books.findIndex((b) => b.book.id === s.book.id)
    if (shift && from >= 0 && to >= 0) {
      for (const b of books.slice(Math.min(from, to), Math.max(from, to) + 1)) next.add(b.book.id)
    } else if (next.has(s.book.id)) {
      next.delete(s.book.id)
    } else {
      next.add(s.book.id)
    }
    setSelected(next)
    setAnchor(s.book.id)
  }
  // Whether the range was valid and selected.
  const selectRange = (range: BookRange) => {
    const from = Number(range.from)
    const to = Number(range.to)
    if (range.from === '' || range.to === '' || Number.isNaN(from) || Number.isNaN(to)) return false
    setSelected(new Set(booksInRange(all, from, to).map((s) => s.book.id)))
    setAnchor(null)
    return true
  }

  return { selected, picked, clearSelection, pick, selectRange }
}

export type BookSelectionState = ReturnType<typeof useBookSelection>
