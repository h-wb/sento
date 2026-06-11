// Shared UI helpers: toast notifications, time formatting, page titles.

import { useEffect } from 'react'
import { notifications } from '@mantine/notifications'

export const notify = {
  ok: (message: string) =>
    notifications.show({ message, color: 'green', withBorder: true }),
  err: (e: unknown) =>
    notifications.show({
      title: 'Something went wrong',
      message: e instanceof Error ? e.message : String(e),
      color: 'red',
      withBorder: true,
      autoClose: 8000,
    }),
}

// fmtWhen renders a server timestamp ("2026-06-11 01:23:45", UTC, or
// RFC3339) as a relative time, falling back to the date for older events.
export function fmtWhen(ts: string): string {
  if (!ts) return ''
  const iso = ts.includes('T') ? ts : ts.replace(' ', 'T') + 'Z'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ts
  const secs = Math.round((Date.now() - d.getTime()) / 1000)
  if (secs < 60) return 'just now'
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  if (secs < 7 * 86400) return `${Math.floor(secs / 86400)}d ago`
  return d.toLocaleDateString()
}

// fmtDay renders a YYYY-MM-DD date with its weekday ("Mon, Jul 7").
export function fmtDay(date: string): string {
  const d = new Date(date + 'T00:00:00')
  if (isNaN(d.getTime())) return date
  return d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
}

export function usePageTitle(title: string) {
  useEffect(() => {
    document.title = title ? `${title} — sento` : 'sento'
    return () => {
      document.title = 'sento'
    }
  }, [title])
}
