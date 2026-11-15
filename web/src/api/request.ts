// The HTTP layer of the typed Sento client: one request wrapper, the error
// text every endpoint reads the same way, and the review-batching limit.

export const base = '/api/v1'

// The message an error response carries, falling back to its status.
export async function readError(res: Response): Promise<string> {
  let msg = `HTTP ${res.status}`
  try {
    const body = await res.json()
    if (body.error) msg = body.error
  } catch {
    /* keep status message */
  }
  return msg
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(base + path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (!res.ok) throw new Error(await readError(res))
  // Empty bodies (204, 202, …) are valid void responses.
  const text = await res.text()
  return text ? (JSON.parse(text) as T) : (undefined as T)
}

// A release can contain more files than one bounded review request accepts.
// Keep batching here so previews and both bulk actions share the limit.
export const reviewBatchSize = 500

// Slice a long id list into review-sized batches.
export function* batched<T>(items: T[]): Generator<T[]> {
  for (let offset = 0; offset < items.length; offset += reviewBatchSize) {
    yield items.slice(offset, offset + reviewBatchSize)
  }
}

export type Ledger = 'volume' | 'chapter'
