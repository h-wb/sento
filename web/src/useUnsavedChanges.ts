import { createContext, useContext, useEffect, useId } from 'react'

export const ChangesContext = createContext<((id: string, dirty: boolean) => void) | null>(null)

export function useUnsavedChanges(dirty: boolean) {
  const register = useContext(ChangesContext)
  const id = useId()
  useEffect(() => {
    register?.(id, dirty)
    return () => register?.(id, false)
  }, [register, id, dirty])
}
