import { useQuery } from '@tanstack/react-query'
import { api } from '../api'

export function useActivity(seriesID = 0) {
  return useQuery({
    queryKey: ['activity', seriesID],
    queryFn: () => api.activity(seriesID),
    refetchInterval: 10000,
  })
}
