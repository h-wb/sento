import { type ReactNode } from 'react'
import { useUnsavedChanges } from '../useUnsavedChanges'
import { Modal } from '@mantine/core'
import { useConfirmAction } from './ConfirmAction'

export function EditorModal({
  dirty,
  busy,
  onClose,
  title,
  children,
  size,
}: {
  dirty: boolean
  busy: boolean
  onClose: () => void
  title: string
  children: ReactNode
  size?: string
}) {
  useUnsavedChanges(dirty)
  const { confirm, confirmation } = useConfirmAction()
  const close = () => {
    if (busy) return
    if (!dirty) {
      onClose()
      return
    }
    confirm({
      title: 'Discard unsaved changes?',
      message: 'Your edits have not been saved.',
      label: 'Discard changes',
      onConfirm: async () => onClose(),
    })
  }
  return (
    <>
      <Modal
        opened
        title={title}
        size={size}
        onClose={close}
        closeOnEscape={!busy}
        closeOnClickOutside={!busy}
        withCloseButton={!busy}
      >
        {children}
      </Modal>
      {confirmation}
    </>
  )
}
