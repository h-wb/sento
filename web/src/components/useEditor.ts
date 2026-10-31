import { useState } from 'react'

// An editor dialog's save: busy while it runs, its error kept to show.
export function useEditor() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  return {
    busy,
    error,
    run: async (fn: () => Promise<void>) => {
      setBusy(true)
      setError('')
      try {
        await fn()
      } catch (e) {
        setError(String(e))
      } finally {
        setBusy(false)
      }
    },
  }
}
