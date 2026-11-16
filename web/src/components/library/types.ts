export type SortField =
  'sortTitle' | 'type' | 'status' | 'year' | 'finalVolume' | 'totalChapters' | 'addedAt'

// A series' books owned of those it collects that are out, null when it collects none.
export type Owned = { complete: boolean; share: number; label: string; title: string } | null
