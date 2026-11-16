import { toHundredths } from '../format'
import type { BookStatus } from '../types/service'

// "1–3, 5, 7.5": consecutive whole numbers collapse into a range. Numbers
// are in hundredths, as the API sends them.
export function bookRanges<T>(items: T[], number: (t: T) => number, name: (t: T) => string): string {
  const sorted = [...items].sort((a, b) => number(a) - number(b))
  const next = (a: T, b: T) => number(a) % 100 === 0 && number(b) - number(a) === 100
  const out: string[] = []
  let start = 0
  for (let i = 1; i <= sorted.length; i++) {
    if (i < sorted.length && next(sorted[i - 1], sorted[i])) continue
    out.push(start === i - 1 ? name(sorted[start]) : `${name(sorted[start])}–${name(sorted[i - 1])}`)
    start = i
  }
  return out.join(', ')
}

export function booksInRange(list: BookStatus[], from: number, to: number) {
  const low = toHundredths(Math.min(from, to))
  const high = toHundredths(Math.max(from, to))
  return list.filter((s) => s.book.numberKnown && s.book.sequenceNum >= low && s.book.sequenceNum <= high)
}
