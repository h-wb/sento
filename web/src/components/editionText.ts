import type { Edition } from '../types/store'

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
