import type { ActivityData, ActivityDownload } from '../src/types/service'
import type { QueueItem, ReviewItem } from '../src/types/store'

export function activityFixture(
  jobs: Partial<QueueItem>[] = [],
  extra: Record<number, Partial<ActivityDownload>> = {},
  reviews: ReviewItem[] = [],
): ActivityData {
  const downloads = jobs.map((item): ActivityDownload => {
    const job: QueueItem = {
      id: 1,
      seriesId: 901,
      seriesTitle: 'One Piece',
      protocol: 'torrent',
      externalId: '',
      title: 'Release',
      status: 'downloading',
      books: [],
      snapshot: null,
      canTryAnother: item.status === 'failed',
      downloadProgress: 0,
      progressAt: '',
      observedAt: '',
      downloadState: '',
      stalled: false,
      retryCount: 0,
      retryAt: '',
      createdAt: '2026-10-05T12:00:00Z',
      ...item,
    }
    return {
      job,
      files: [],
      attention: ['failed', 'needs_review', 'dispatch_unknown'].includes(job.status) || job.stalled,
      history: ['imported', 'cancelled'].includes(job.status),
      issue: job.error?.includes('404') ? 'gone' : 'download',
      explanation: job.error?.includes('404')
        ? 'This release is no longer available.'
        : job.error
          ? 'The download could not finish.'
          : '',
      ...extra[job.id],
    }
  })
  return {
    downloads,
    reviews,
    events: [],
    seriesTitles: Object.fromEntries(
      downloads.map((d) => [d.job.seriesId, d.job.seriesTitle || `Series ${d.job.seriesId}`]),
    ),
  }
}
