import type { ReviewBookPlan, ReviewFilePlan, FileScore } from '../src/types/service'
import type { BookRef } from '../src/types/store'

export const reviewScore: FileScore = { profileId: 1, profileName: 'Standard', score: 100, formats: [{ name: 'Digital', score: 100 }], factsKnown: true, reason: '' }
export const reviewBook = (book: BookRef, label: string): ReviewBookPlan => ({ book, label, card: 'Standard Edition', incoming: reviewScore, owners: [], useIncoming: true, currentSelected: false, requestedReplacement: false, reason: '' })
export const reviewFile = (fileId: number, books: ReviewBookPlan[], reason = ''): ReviewFilePlan => ({ fileId, path: `/library/Volume ${fileId}.cbz`, group: '1r0n', source: 'torrent', books, reason, trashReason: '', trashWhenUnused: false })

// Tiny archives with distinct page bytes exercise real scanning and fingerprints.
export const reviewArchives: string[] = [
  "UEsDBBQAAAAAADi6Y12z88EzEwAAABMAAAAHAAAAMDAxLmpwZ3VuaXF1ZSBwYWdlIGJ5dGVzIDBQSwECFAMUAAAAAAA4umNds/PBMxMAAAATAAAABwAAAAAAAAAAAAAAgAEAAAAAMDAxLmpwZ1BLBQYAAAAAAQABADUAAAA4AAAAAAA=",
  "UEsDBBQAAAAAADi6Y10lw8ZEEwAAABMAAAAHAAAAMDAxLmpwZ3VuaXF1ZSBwYWdlIGJ5dGVzIDFQSwECFAMUAAAAAAA4umNdJcPGRBMAAAATAAAABwAAAAAAAAAAAAAAgAEAAAAAMDAxLmpwZ1BLBQYAAAAAAQABADUAAAA4AAAAAAA=",
  "UEsDBBQAAAAAADi6Y12fks/dEwAAABMAAAAHAAAAMDAxLmpwZ3VuaXF1ZSBwYWdlIGJ5dGVzIDJQSwECFAMUAAAAAAA4umNdn5LP3RMAAAATAAAABwAAAAAAAAAAAAAAgAEAAAAAMDAxLmpwZ1BLBQYAAAAAAQABADUAAAA4AAAAAAA=",
  "UEsDBBQAAAAAADi6Y10JosiqEwAAABMAAAAHAAAAMDAxLmpwZ3VuaXF1ZSBwYWdlIGJ5dGVzIDNQSwECFAMUAAAAAAA4umNdCaLIqhMAAAATAAAABwAAAAAAAAAAAAAAgAEAAAAAMDAxLmpwZ1BLBQYAAAAAAQABADUAAAA4AAAAAAA=",
  "UEsDBBQAAAAAADi6Y12qN6w0EwAAABMAAAAHAAAAMDAxLmpwZ3VuaXF1ZSBwYWdlIGJ5dGVzIDRQSwECFAMUAAAAAAA4umNdqjesNBMAAAATAAAABwAAAAAAAAAAAAAAgAEAAAAAMDAxLmpwZ1BLBQYAAAAAAQABADUAAAA4AAAAAAA=",
  "UEsDBBQAAAAAADi6Y108B6tDEwAAABMAAAAHAAAAMDAxLmpwZ3VuaXF1ZSBwYWdlIGJ5dGVzIDVQSwECFAMUAAAAAAA4umNdPAerQxMAAAATAAAABwAAAAAAAAAAAAAAgAEAAAAAMDAxLmpwZ1BLBQYAAAAAAQABADUAAAA4AAAAAAA=",
  "UEsDBBQAAAAAADi6Y12GVqLaEwAAABMAAAAHAAAAMDAxLmpwZ3VuaXF1ZSBwYWdlIGJ5dGVzIDZQSwECFAMUAAAAAAA4umNdhlai2hMAAAATAAAABwAAAAAAAAAAAAAAgAEAAAAAMDAxLmpwZ1BLBQYAAAAAAQABADUAAAA4AAAAAAA="
]
