import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'

const statusKey = ['library', 'organize', 'status']

// The background organize run over the given series (none: the whole
// library): its live status, and the action that starts it. Only one run
// is held at a time, so a run started elsewhere shows here as busy.
export function useOrganizeRun(seriesIds?: number[]) {
  const queryClient = useQueryClient()
  const [started, setStarted] = useState(false)
  const status = useQuery({
    queryKey: statusKey,
    queryFn: api.organizeLibraryStatus,
    refetchInterval: (q) => (q.state.data?.running || q.state.data?.queued ? 1500 : false),
  })
  const busy = !!(status.data?.running || status.data?.queued)
  const apply = useMutation({
    mutationFn: (changes: { rename: boolean; retag: boolean }) =>
      api.organizeLibrary({ seriesIds: seriesIds ?? [], ...changes }),
    onSuccess: (s) => {
      setStarted(true)
      queryClient.setQueryData(statusKey, s)
    },
  })
  const finished = started && !!status.data && !busy
  useEffect(() => {
    if (finished) void queryClient.invalidateQueries({ queryKey: ['series'] })
  }, [finished, queryClient])
  return { status, busy, started, finished, apply }
}
