import type { File as SeriesFile } from '../types/store'

// The flags a file's facts carry, as release profiles test them and a
// person can correct them (the backend's store.FactFlags, without the
// release-level "complete").
export const factFlags: { key: string; label: string }[] = [
  { key: 'colored', label: 'Colored' },
  { key: 'digital', label: 'Digital' },
  { key: 'scan', label: 'Scan' },
  { key: 'official', label: 'Official' },
  { key: 'premium', label: 'Premium' },
  { key: 'web', label: 'Web' },
  { key: 'raw', label: 'Raw' },
  { key: 'omnibus', label: 'Omnibus' },
  { key: 'deluxe', label: 'Deluxe' },
  { key: 'boxset', label: 'Box set' },
]

// Whether a person set the flag, either way.
export const manualFlag = (f: SeriesFile, key: string) =>
  f.facts?.sources?.[`flag:${key}`] === 'manual'

// What the file is taken to be: its own flag, or its release's unless a
// person turned it off. The same rule profiles judge it by.
export const hasFlag = (f: SeriesFile, key: string) =>
  !!f.facts?.flags?.[key] || (!!f.claims?.flags?.[key] && !manualFlag(f, key))

export const flagLabels = (f: SeriesFile) =>
  factFlags.filter((x) => hasFlag(f, x.key)).map((x) => x.label)

// Where a file came from, in words.
export const fileOrigin = (f: SeriesFile) =>
  f.claims?.title
    ? `From “${f.claims.title}”${f.facts?.site || f.claims.site ? ` · ${f.facts?.site || f.claims.site}` : ''}`
    : f.provenance === 'library-import'
      ? 'Found by a scan'
      : f.provenance === 'source'
        ? 'Downloaded from a source'
        : 'Downloaded'
