import type { ReleaseCandidate } from '../../api'
import { factFlags } from '../factFlags'

// This screen words the flags its own way ("Color"); the keys are the
// shared ones.
const ownFlagLabels: Record<string, string> = { colored: 'Color' }
export const flagLabel = (key: string) =>
  ownFlagLabels[key] ?? factFlags.find((f) => f.key === key)?.label ?? key

export const releaseKey = (c: ReleaseCandidate) =>
  c.key || `${c.provider}/${c.siteId}/${c.guid}/${c.versionId ?? 0}`
export const rangeLabel = (label: string, ranges?: [number, number][]) =>
  ranges?.length
    ? `${label} ${ranges.map(([from, to]) => (from === to ? from : `${from}–${to}`)).join(', ')}`
    : ''
export const status = (c: ReleaseCandidate) =>
  c.decision?.status ?? (c.rejections?.length ? 'blocked' : 'ready')
export const rank = { ready: 0, review: 1, blocked: 2 }

// A pack carries several books: by what it would collect here, or by the
// span its name states.
const spans = (ranges?: [number, number][]) =>
  (ranges ?? []).reduce((n, [from, to]) => n + Math.max(1, Math.floor(to - from) + 1), 0)
export const isPack = (c: ReleaseCandidate) =>
  (c.coverage?.length ?? 0) > 1 ||
  !!c.parse.complete ||
  spans(c.parse.volumes) > 1 ||
  (!c.parse.volumes?.length && spans(c.parse.chapters) > 1)
export const firstNumber = (c: ReleaseCandidate) =>
  c.parse.volumes?.[0]?.[0] ?? c.parse.chapters?.[0]?.[0] ?? Number.MAX_SAFE_INTEGER

// Short names for why a release was ruled out, used to group the blocked
// results. The full sentence stays on each row.
const reasonNames: Record<string, string> = {
  'rejected.series_match': 'Different series',
  'rejected.edition_routing': 'Different edition',
  'rejected.edition_ambiguous': 'Edition to confirm',
  'rejected.language': 'Wrong language',
  'rejected.content_type': 'Not manga',
  'rejected.numbers_unknown': 'No book number',
  'rejected.pack_unverified': 'Contents not confirmed',
  'rejected.pack_threshold': 'Mostly books you have',
  'rejected.previous_download': 'Download being replaced',
  'rejected.no_coverage': 'Not needed',
  'rejected.book_ambiguous': 'Ambiguous book',
  'rejected.blocklisted': 'On your blocklist',
  'rejected.target_group': 'Other release group',
}
export const reasonCode = (c: ReleaseCandidate) =>
  c.decision?.reasons[0]?.code ?? c.rejections?.[0] ?? ''
// Who decides a rejection, in the order the reasons are grouped.
export const owners = [
  ['book', 'Not this book'],
  ['profile', 'Your profile'],
  ['confirm', 'To confirm'],
  ['other', 'Other'],
] as const
export const reasonOwner = (c: ReleaseCandidate) => c.decision?.reasons[0]?.owner ?? 'other'
export const reasonName = (code: string) => {
  const fallback = code.replace(/^rejected\./, '').replace(/_/g, ' ')
  return reasonNames[code] ?? fallback.charAt(0).toUpperCase() + fallback.slice(1)
}
