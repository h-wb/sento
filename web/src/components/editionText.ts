import type { CollectedVersion, Edition } from '../types/store'

const unique = (values: string[]) => [...new Set(values.filter(Boolean))]

// An edition can exist in several media, each a catalog collection of its
// own; a local edition has the one it was given.
export const editionMedia = (c: Edition) =>
  c.collections.length ? unique(c.collections.map((x) => x.medium)) : unique([c.medium])

export const editionPublishers = (c: Edition) =>
  c.collections.length ? unique(c.collections.map((x) => x.publisher)) : unique([c.publisher])

// "en · Shonen Jump · digital + paperback"
export const editionDetails = (c: Edition, ...more: (string | false | null | undefined)[]) =>
  [c.language, editionPublishers(c).join(' / '), editionMedia(c).join(' + '), ...more]
    .filter(Boolean)
    .join(' · ')

// A card is one way of collecting an edition. It is titled by the edition,
// and by its own release profile too when the edition has several cards:
// "Standard Edition · Colored".
export const cardTitle = (c: Edition, v: CollectedVersion, versions: CollectedVersion[]) =>
  v.profileName && versions.filter((t) => t.editionId === c.id).length > 1
    ? `${c.editionName} · ${v.profileName}`
    : c.editionName

// The card named away from the edition list (a search, a file's mapping):
// "en · Standard Edition · Colored".
export const cardLabel = (c: Edition, v: CollectedVersion, versions: CollectedVersion[]) =>
  [c.language, cardTitle(c, v, versions)].filter(Boolean).join(' · ')

// A stored file's name as people know it: no folders, no content hash.
export const fileName = (path: string) =>
  (path.split(/[\\/]/).pop() ?? path).replace(/ \[[0-9a-f]{64}\](?=\.\w+$)/, '')
