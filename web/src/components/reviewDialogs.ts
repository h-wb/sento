import { createContext, useContext } from 'react'
import type { ReviewItem } from '../types/store'

export type ReleaseReview = { seriesID: number; title: string; files: ReviewItem[] }

export const ReviewDialogsContext = createContext<{
  file: (item: ReviewItem) => void
  release: (pack: ReleaseReview) => void
} | null>(null)

export function useReviewDialogs() {
  const dialogs = useContext(ReviewDialogsContext)
  if (!dialogs) throw new Error('Review actions require an activity dialog host')
  return dialogs
}
