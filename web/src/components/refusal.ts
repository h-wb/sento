import type { BookStatus } from '../types/service'

// Books of a card that are on disk in files the card will not use, and no
// other card uses either, with the reason most of them share.
export type Refused = { books: BookStatus[]; fileIds: number[]; reason: string }

export function refusedFiles(versionId: number, all: BookStatus[]): Refused {
  // A file another card holds is not a problem to solve, just a book this
  // card lacks.
  const usedSomewhere = new Set(all.flatMap((b) => b.fileIds))
  const books = all.filter(
    (b) =>
      b.versionId === versionId &&
      b.state === 'unsatisfactory' &&
      b.onDiskFileIds.some((id) => !usedSomewhere.has(id)),
  )
  const reasons = new Map<string, number>()
  for (const b of books) reasons.set(b.reason, (reasons.get(b.reason) ?? 0) + 1)
  return {
    books,
    fileIds: [...new Set(books.flatMap((b) => b.onDiskFileIds))],
    reason: [...reasons].sort((x, y) => y[1] - x[1])[0]?.[0] ?? '',
  }
}
