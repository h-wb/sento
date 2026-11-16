import { useState } from 'react'

// Client-side paging over a list: the current page (clamped to the page
// count), the page count and the current slice. Call setPage(1) when the
// filter changes.
export function usePaged<T>(items: T[], pageSize: number) {
  const [requested, setPage] = useState(1)
  const pages = Math.max(1, Math.ceil(items.length / pageSize))
  const page = Math.min(requested, pages)
  const slice = items.slice((page - 1) * pageSize, page * pageSize)
  return { page, setPage, pages, slice }
}
