import { useQuery } from '@tanstack/react-query'
import { activityQuery } from '../queries'

export function useActivity(seriesID = 0) {
  return useQuery(activityQuery(seriesID))
}
