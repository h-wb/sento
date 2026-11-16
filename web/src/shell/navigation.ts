import {
IconAdjustmentsHorizontal,
IconBooks,
IconCalendar,
IconDownload,
IconFiles,
IconPlug,
IconServerBolt,
IconWorldSearch,
IconZoomCheck
} from '@tabler/icons-react';
export type NavItem = { to: string; label: string; icon: typeof IconBooks; key: string }

// Day-to-day pages, always on top. `key` is the page's "g <key>" shortcut.
export const primaryLinks: NavItem[] = [
  { to: '/', label: 'Library', icon: IconBooks, key: 'l' },
  { to: '/calendar', label: 'Calendar', icon: IconCalendar, key: 'c' },
  { to: '/queue', label: 'Activity', icon: IconDownload, key: 'a' },
  { to: '/wanted', label: 'Wanted', icon: IconZoomCheck, key: 'w' },
]
// Set-up-once config, ordered to mirror the pipeline: search → download →
// quality decision → import.
export const configLinks: NavItem[] = [
  { to: '/indexers', label: 'Indexers', icon: IconWorldSearch, key: 'i' },
  { to: '/connections', label: 'Connections', icon: IconPlug, key: 'o' },
  { to: '/profiles', label: 'Profiles', icon: IconAdjustmentsHorizontal, key: 'p' },
  { to: '/media', label: 'Media', icon: IconFiles, key: 'm' },
]
export const systemLink: NavItem = { to: '/system', label: 'System', icon: IconServerBolt, key: 's' }
export const allLinks = [...primaryLinks, ...configLinks, systemLink]

export const modKey = /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘' : 'Ctrl '

// "g" then a page's key jumps to it (Gmail-style sequence); "g n" adds a series.
export const SHORTCUTS: [string[], string][] = [
  [[`${modKey}K`, '/'], 'Search pages and series'],
  ...allLinks.map((l): [string[], string] => [[`g ${l.key}`], `Go to ${l.label}`]),
  [['g n'], 'Add a series'],
  [['?'], 'Show this help'],
]

