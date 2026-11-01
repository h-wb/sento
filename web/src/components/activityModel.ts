import type { ActivityData, ActivityDownload } from '../types/service'
import type { ReviewItem } from '../types/store'

export type ActivityFilter = 'progress' | 'attention' | 'history'
export type AttentionGroup = {
  key: string
  seriesID: number
  title: string
  downloads: ActivityDownload[]
  reviews: ReviewItem[]
}

export function inProgress(d: ActivityDownload) {
  return !d.history && ['queued', 'downloading', 'importing', 'retry_wait'].includes(d.job.status)
}

// A folder failure or a pack with several unmatched files is one problem.
// Both navigation badges and the feed use these same groups.
export function attentionGroups(data?: ActivityData): AttentionGroup[] {
  if (!data) return []
  const groups = new Map<string, AttentionGroup>()
  const add = (key: string, seriesID: number, title: string) => {
    let group = groups.get(key)
    if (!group) {
      group = { key, seriesID, title, downloads: [], reviews: [] }
      groups.set(key, group)
    }
    return group
  }
  for (const download of data.downloads) {
    if (!download.attention || download.history) continue
    const job = download.job
    const sharedFailure = job.protocol === 'source' && job.canTryAnother && !download.files.length
    const key = sharedFailure
      ? `${job.seriesId}/source/${download.listingId || download.sourceName || job.providerId || 'source'}/${download.issue}`
      : `${job.seriesId}/download/${job.id}`
    add(
      key,
      job.seriesId,
      job.seriesTitle || data.seriesTitles[job.seriesId] || `Series ${job.seriesId}`,
    ).downloads.push(download)
  }
  for (const review of data.reviews) {
    const key = ['file', 'chapter'].includes(review.kind)
      ? `${review.seriesId}/${review.kind}`
      : `${review.seriesId}/${review.id}`
    add(key, review.seriesId, review.seriesTitle).reviews.push(review)
  }
  return [...groups.values()].sort((a, b) => a.title.localeCompare(b.title))
}

export function activityCounts(data?: ActivityData) {
  return {
    progress: data?.downloads.filter(inProgress).length ?? 0,
    attention: attentionGroups(data).length,
    history: data?.downloads.filter((d) => d.history).length ?? 0,
  }
}

export function downloadLabel(d: ActivityDownload) {
  const job = d.job
  if (job.status === 'retry_wait') return 'Retry scheduled'
  if (job.status === 'dispatch_unknown') return 'Confirm download'
  if (d.files.length)
    return `${d.files.length} ${d.files.length === 1 ? 'file needs' : 'files need'} ${d.files.some((f) => f.status === 'unavailable' || f.status === 'unknown') ? 'checking' : 'matching'}`
  if (job.status === 'needs_review') return 'Needs attention'
  if (job.status === 'failed') return d.history ? 'Previous attempt' : 'Failed'
  if (job.status === 'cancelled') return 'Cancelled'
  if (job.status === 'imported') return 'Imported'
  if (job.status === 'importing') return 'Importing'
  if (job.status === 'queued') return 'Waiting'
  if (job.stalled) return 'Stalled'
  const states: Record<string, string> = {
    pausedDL: 'Paused in client',
    stoppedDL: 'Paused in client',
    queuedDL: 'Waiting in client',
    checkingDL: 'Checking files',
    checkingResumeData: 'Checking files',
    moving: 'Moving files',
    missingFiles: 'Files missing',
    error: 'Client error',
    'waiting for source cooldown': 'Waiting for source',
    'waiting for download slot': 'Waiting for download slot',
  }
  return states[job.downloadState] ?? 'Downloading'
}

export function activityTime(value: string) {
  return Date.parse(value.includes('T') ? value : `${value.replace(' ', 'T')}Z`) || 0
}
