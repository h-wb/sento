import type { BookRef, TrashedFile, ReviewResolution } from '../types/store'
import type {
  FileScore,
  ReviewPlan,
  ReviewPlanRequest,
  ReviewApplyRequest,
  ReviewFileOutcome,
  CleanupFile,
  CleanupResult,
  FileMappingResult,
} from '../types/service'
import type { File as SeriesFile } from '../types/store'
import type { FileCleanupBody, FilesFactsBody } from '../types/server'
import { batched, request } from './request'

export const filesApi = {
  files: (id: number) => request<SeriesFile[]>(`/series/${id}/files`),
  fileScore: (id: number, vid: number, fid: number) =>
    request<FileScore>(`/series/${id}/versions/${vid}/files/${fid}/score`),
  previewReviewChoices: (id: number, body: ReviewPlanRequest) =>
    request<ReviewPlan>(`/series/${id}/review/choices/preview`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  applyReviewChoices: (id: number, body: ReviewApplyRequest) =>
    request<ReviewFileOutcome[]>(`/series/${id}/review/choices`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  // One correction of what several files are (language, group, flags).
  correctFilesFacts: (id: number, body: FilesFactsBody) =>
    request<void>(`/series/${id}/files/facts`, { method: 'PATCH', body: JSON.stringify(body) }),
  mapFile: (id: number, fid: number, books: BookRef[], replace = false) =>
    request<FileMappingResult>(`/series/${id}/files/${fid}/mapping`, {
      method: 'PUT',
      body: JSON.stringify({ books, ...(replace ? { replace: true } : {}) }),
    }),
  // Decline a pack of file reviews at once: every file reviewed, unmapped.
  resolveFileReviews: async (id: number, fileIds: number[]) => {
    const result: ReviewResolution = { resolved: [], skipped: [] }
    for (const batch of batched(fileIds)) {
      const part = await request<ReviewResolution>(`/series/${id}/review/resolve`, {
        method: 'POST',
        body: JSON.stringify({ fileIds: batch }),
      })
      result.resolved.push(...part.resolved)
      result.skipped.push(...part.skipped)
    }
    return result
  },
  cleanupPreview: (id: number, editionId?: number) =>
    request<CleanupFile[]>(
      `/series/${id}/files/cleanup${editionId ? `?editionId=${editionId}` : ''}`,
    ),
  cleanupFiles: (id: number, body: FileCleanupBody) =>
    request<CleanupResult>(`/series/${id}/files/cleanup`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  fileTrash: (id: number) => request<TrashedFile[]>(`/series/${id}/files/trash`),
  restoreTrash: (id: number, fid: number) =>
    request<void>(`/series/${id}/files/trash/${fid}/restore`, { method: 'POST' }),
  purgeTrash: (id: number, fid: number) =>
    request<void>(`/series/${id}/files/trash/${fid}`, { method: 'DELETE' }),
  deleteFile: (fid: number) => request<void>(`/files/${fid}`, { method: 'DELETE' }),
}
