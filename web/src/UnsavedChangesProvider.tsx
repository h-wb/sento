import { useCallback, useState, type ReactNode } from 'react'
import { Button, Group, Modal, Text } from '@mantine/core'
import { useBeforeUnload, useBlocker } from 'react-router-dom'

import { ChangesContext } from './useUnsavedChanges'

// One blocker for the whole page, including pages with independently saved forms.
export function UnsavedChangesProvider({ children }: { children: ReactNode }) {
  const [forms, setForms] = useState<Set<string>>(new Set())
  const register = useCallback((id: string, dirty: boolean) => {
    setForms((prev) => {
      if (prev.has(id) === dirty) return prev
      const next = new Set(prev)
      if (dirty) next.add(id)
      else next.delete(id)
      return next
    })
  }, [])
  const dirty = forms.size > 0
  const blocker = useBlocker(({ currentLocation, nextLocation }) => dirty && currentLocation.pathname !== nextLocation.pathname)
  useBeforeUnload(useCallback((event) => {
    if (dirty) { event.preventDefault(); event.returnValue = '' }
  }, [dirty]))
  return (
    <ChangesContext.Provider value={register}>
      {children}
      <Modal opened={blocker.state === 'blocked'} onClose={() => blocker.reset?.()} title="Discard unsaved changes?" centered>
        <Text size="sm" c="dimmed">Your edits haven't been saved. Leaving this page will discard them.</Text>
        <Group mt="lg" justify="flex-end">
          <Button variant="default" onClick={() => blocker.reset?.()} data-autofocus>Keep editing</Button>
          <Button color="red" onClick={() => blocker.proceed?.()}>Discard changes</Button>
        </Group>
      </Modal>
    </ChangesContext.Provider>
  )
}
