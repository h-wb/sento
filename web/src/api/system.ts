import type { CalendarEntry } from '../types/store'
import type { HealthReport, WantedRow } from '../types/service'
import type { StatusResponse } from '../types/server'
import type { TaskStatus } from '../types/scheduler'
import type { SeriesRef } from '../types/metadata'
import { request } from './request'

export type SystemStatus = StatusResponse
export type LookupResult = SeriesRef

export const systemApi = {
  status: () => request<SystemStatus>('/system/status'),
  healthChecks: () => request<HealthReport>('/system/checks'),
  runHealthChecks: () => request<HealthReport>('/system/checks', { method: 'POST' }),
  systemTasks: () => request<TaskStatus[]>('/system/tasks'),
  runTask: (slug: string) => request<void>(`/system/tasks/${slug}/run`, { method: 'POST' }),
  lookup: (q: string) => request<LookupResult[]>(`/lookup?q=${encodeURIComponent(q)}`),
  calendar: (days = 60) => request<CalendarEntry[]>(`/calendar?days=${days}`),
  wanted: () => request<WantedRow[]>('/wanted'),
}
