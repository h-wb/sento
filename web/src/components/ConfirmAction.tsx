import { useState, type ReactNode } from 'react'
import { Alert, Button, Group, Modal, Stack, Text } from '@mantine/core'

type Action = { title: string; message: ReactNode; label: string; onConfirm: () => Promise<unknown> }

// Keep the dialog open on failure so the user can retry the same action.
export function useConfirmAction() {
  const [action, setAction] = useState<Action | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const confirm = (next: Action) => { setError(null); setAction(next) }
  const close = () => { if (!pending) setAction(null) }
  const submit = async () => {
    if (!action || pending) return
    setPending(true)
    setError(null)
    try { await action.onConfirm(); setAction(null) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setPending(false) }
  }
  const confirmation = (
    <Modal opened={!!action} onClose={close} title={action?.title} centered
      closeOnClickOutside={!pending} closeOnEscape={!pending} withCloseButton={!pending}>
      <Stack gap="md">
        <Text size="sm" c="dimmed">{action?.message}</Text>
        {error && <Alert color="red">{error}</Alert>}
        <Group justify="flex-end">
          <Button variant="default" onClick={close} disabled={pending} data-autofocus>Cancel</Button>
          <Button color="red" loading={pending} onClick={() => void submit()}>{action?.label}</Button>
        </Group>
      </Stack>
    </Modal>
  )
  return { confirm, confirmation }
}
