import { useState } from 'react'
import { errorMessage } from '../ui'

// An editor dialog's save: busy while it runs, its error kept to show.
export function useEditor() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  return {
    busy,
    error,
    // settled runs after fn either way, once its error shows and before busy clears.
    run: async (fn: () => Promise<void>, settled?: () => Promise<void>) => {
      setBusy(true)
      setError('')
      try {
        await fn()
      } catch (e) {
        setError(errorMessage(e))
      } finally {
        await settled?.()
        setBusy(false)
      }
    },
  }
}
