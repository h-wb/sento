import type { BookStatus } from '../../types/service'

export const labels: Record<string, string> = {
  owned: 'Owned',
  pending: 'Pending',
  missing: 'Missing',
  not_released: 'Upcoming',
  unmonitored: 'Paused',
  unknown: 'Date unknown',
  upgrade_eligible: 'Upgrade wanted',
  unsatisfactory: 'On disk, not accepted',
  needs_review: 'Needs review',
  storage_unavailable: 'Storage unavailable',
}

export type Tone = 'disk' | 'wanted' | 'pending' | 'review' | 'upcoming' | 'paused'

// tone is the one colour a book gets in the grid and the status badge.
export function tone(s: BookStatus): Tone {
  switch (s.state) {
    case 'pending':
      return 'pending'
    case 'needs_review':
    case 'storage_unavailable':
    case 'unsatisfactory':
      return 'review'
    case 'owned':
    case 'upgrade_eligible':
      return 'disk'
    case 'missing':
      return 'wanted'
    case 'unmonitored':
      return 'paused'
    default:
      return 'upcoming'
  }
}

export const badgeColor: Record<Tone, string> = {
  disk: 'green',
  wanted: 'red',
  pending: 'blue',
  review: 'yellow',
  upcoming: 'gray',
  paused: 'gray',
}

export type Filter = {
  value: string
  label: string
  tone?: Tone
  always?: boolean
  match: (s: BookStatus) => boolean
}

export const filters: Filter[] = [
  { value: 'all', label: 'All', always: true, match: () => true },
  {
    value: 'on_disk',
    label: 'On disk',
    tone: 'disk',
    always: true,
    match: (s) => (s.onDiskFileIds ?? []).length > 0,
  },
  { value: 'wanted', label: 'Wanted', tone: 'wanted', always: true, match: (s) => s.wanted },
  { value: 'pending', label: 'Downloading', tone: 'pending', match: (s) => s.state === 'pending' },
  { value: 'upcoming', label: 'Upcoming', tone: 'upcoming', match: (s) => tone(s) === 'upcoming' },
  {
    value: 'unmonitored',
    label: 'Paused',
    tone: 'paused',
    match: (s) => s.state === 'unmonitored',
  },
  { value: 'review', label: 'Needs attention', tone: 'review', match: (s) => tone(s) === 'review' },
]

