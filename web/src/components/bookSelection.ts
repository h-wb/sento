import type { BookStatus } from '../types/service'
import type { BookRef, CollectedVersion, Edition, QueueItem } from '../types/store'
import type { BookSelection } from './BookDialog'
import { cardLabel } from './editionText'

// The single book a row or grid cell opens the search for.
export function bookSelection(c: Edition, t: CollectedVersion, label: string, s: BookStatus): BookSelection {
  return {
    ledger: c.type === 'chapter' ? 'chapter' : 'volume',
    editionId: c.id,
    versionId: t.id,
    bookId: s.book.id,
    number: s.book.sequenceNum,
    label: s.book.sequenceStr,
    editionLabel: label,
    fileIds: s.fileIds,
    unusedFileIds: s.onDiskFileIds.filter((id) => !s.fileIds.includes(id)),
    monitored: s.monitored,
    wanted: s.wanted,
  }
}

// Search every wanted book of one edition card at once.
export function editionSearch(c: Edition, t: CollectedVersion, allVersions: CollectedVersion[]): BookSelection {
  return {
    ledger: c.type === 'chapter' ? 'chapter' : 'volume',
    editionId: c.id,
    versionId: t.id,
    all: true,
    number: 0,
    editionLabel: cardLabel(c, t, allVersions),
    monitored: t.monitored,
    wanted: true,
    fileIds: [],
    autoSearch: true,
  }
}

// Search a replacement for a download that failed or stalled; null when
// the download names no version to search.
export function replacementSearch(book: BookRef | undefined, job: QueueItem): BookSelection | null {
  if (!book?.versionId) return null
  return {
    versionId: book.versionId,
    editionId: book.editionId,
    ledger: book.ledger as 'volume' | 'chapter',
    all: true,
    number: 0,
    monitored: true,
    wanted: true,
    fileIds: [],
    autoSearch: true,
    editionLabel: 'Replacement',
    replaceQueueId: job.id,
    replaceTitle: job.title,
  }
}
