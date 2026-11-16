// Display formatting shared across the app: server timestamps, relative
// times, intervals, numbers and sizes.

// A server timestamp ("2026-06-11 01:23:45", UTC, or RFC3339) as a Date.
export function parseServerTime(ts: string): Date {
  return new Date(ts.includes('T') ? ts : ts.replace(' ', 'T') + 'Z')
}

// fmtWhen renders a server timestamp ("2026-06-11 01:23:45", UTC, or
// RFC3339) as a relative time, falling back to the date for older events.
export function fmtWhen(ts: string): string {
  if (!ts) return ''
  const d = parseServerTime(ts)
  if (isNaN(d.getTime())) return ts
  const secs = Math.round((Date.now() - d.getTime()) / 1000)
  if (secs < 60) return 'just now'
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  if (secs < 7 * 86400) return `${Math.floor(secs / 86400)}d ago`
  return d.toLocaleDateString()
}

// fmtUntil renders a future timestamp as "in 5m" / "in 2h" / "in 3d"
// ("now" once due).
export function fmtUntil(ts: string): string {
  if (!ts) return ''
  const d = new Date(ts)
  if (isNaN(d.getTime())) return ts
  const secs = Math.round((d.getTime() - Date.now()) / 1000)
  if (secs <= 0) return 'now'
  if (secs < 3600) return `in ${Math.max(1, Math.floor(secs / 60))}m`
  if (secs < 86400) return `in ${Math.floor(secs / 3600)}h`
  return `in ${Math.floor(secs / 86400)}d`
}

// fmtDay renders a YYYY-MM-DD date with its weekday ("Mon, Jul 7").
export function fmtDay(date: string): string {
  const d = new Date(date + 'T00:00:00')
  if (isNaN(d.getTime())) return date
  return d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
}

export function fmtInterval(secs: number): string {
  if (secs <= 0) return 'on demand'
  if (secs >= 86400) return `${Math.round(secs / 86400)} day${secs >= 2 * 86400 ? 's' : ''}`
  if (secs >= 3600) return `${Math.round(secs / 3600)} hour${secs >= 2 * 3600 ? 's' : ''}`
  return `${Math.round(secs / 60)} minute${secs >= 120 ? 's' : ''}`
}

export function fmtDuration(ms: number): string {
  if (ms <= 0) return '—'
  if (ms < 1000) return `${ms} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`
}

export function fmtNum(hundredths: number): string {
  const whole = Math.floor(hundredths / 100)
  const frac = hundredths % 100
  if (frac === 0) return String(whole)
  return (hundredths / 100).toString()
}

export function fmtSize(bytes: number): string {
  if (bytes >= 1 << 30) return (bytes / (1 << 30)).toFixed(1) + ' GiB'
  if (bytes >= 1 << 20) return (bytes / (1 << 20)).toFixed(1) + ' MiB'
  return Math.round(bytes / 1024) + ' KiB'
}

// The noun for a count: plural(n, 'file') is "file" or "files".
export const plural = (n: number, one: string, many?: string) =>
  n === 1 ? one : (many ?? one + 's')

// Book numbers are stored in hundredths ("1.5" is 150).
export const toHundredths = (n: number) => Math.round(n * 100)
export const fromHundredths = (h: number) => h / 100

// The host part of a URL for display, or the input itself if it is not one.
export function hostOf(url: string) {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}
