// Shared UI helpers: toast notifications, error text, page titles.

import { useEffect } from 'react'
import { notifications } from '@mantine/notifications'

export const notify = {
  ok: (message: string) =>
    notifications.show({ message, color: 'green', withBorder: true }),
  err: (e: unknown) =>
    notifications.show({
      title: 'Something went wrong',
      message: errorMessage(e),
      color: 'red',
      withBorder: true,
      autoClose: 8000,
    }),
}

// An error's own message, without the "Error: " prefix String(e) adds.
export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function usePageTitle(title: string) {
  useEffect(() => {
    document.title = title ? `${title} — sento` : 'sento'
    return () => {
      document.title = 'sento'
    }
  }, [title])
}

// A release is "v0.7.0"; any other build says which commit it is.
export const versionLabel = (s: { version: string; commit?: string }) =>
  /^\d/.test(s.version)
    ? `v${s.version}`
    : [s.version, s.commit?.slice(0, 7)].filter(Boolean).join(' · ')
