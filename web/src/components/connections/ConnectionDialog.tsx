import { useState, type ReactNode } from 'react'
import { Alert, Badge, Button, Group, Stack, Text, Tooltip } from '@mantine/core'
import { IconLock } from '@tabler/icons-react'
import { useMutation } from '@tanstack/react-query'
import { api, type ConnectionSettings, type ConnectionTest } from '../../api'
import { notify } from '../../ui'
import { EditorModal } from '../EditorModal'

// StatusBadge renders one probe result. `draft` marks a test of unsaved
// values, so it can't be mistaken for the state of what is running.
export function StatusBadge({
  result,
  configured,
  probing,
  draft,
}: {
  result?: ConnectionTest
  configured: boolean
  probing?: boolean
  draft?: boolean
}) {
  if (!configured) {
    return (
      <Badge color="gray" variant="light">
        Not configured
      </Badge>
    )
  }
  if (!result) {
    return (
      <Badge color="gray" variant="light">
        {probing ? 'Checking…' : 'Unknown'}
      </Badge>
    )
  }
  const label = result.ok
    ? `${draft ? 'Reachable' : 'Connected'}${result.detail ? ` · ${result.detail}` : ''}`
    : 'Unreachable'
  return (
    <Tooltip label={result.error || 'The service answered'} multiline maw={360}>
      <Badge color={result.ok ? 'green' : 'red'} variant="light">
        {label}
      </Badge>
    </Tooltip>
  )
}

// ConnectionDialog is one connection's settings: its form, a Test of the
// values as typed, and Save. Saving writes only this connection; the rest
// stays as it is on the server.
export function ConnectionDialog({
  title,
  blurb,
  service,
  saved,
  live,
  onSaved,
  onClose,
  alwaysSavable = false,
  readOnly = false,
  isSet,
  same,
  only,
  merge,
  fields,
  extra,
  below,
}: {
  title: string
  blurb?: string
  service: string
  saved: ConnectionSettings
  live?: ConnectionTest
  onSaved: () => void
  onClose: () => void
  // A connection being added has nothing saved to differ from.
  alwaysSavable?: boolean
  // Fixed by the deployment's configuration: the values are shown and can
  // be tested, but there is nothing to save.
  readOnly?: boolean
  isSet: (c: ConnectionSettings) => boolean
  same: (a: ConnectionSettings, b: ConnectionSettings) => boolean
  only: (c: ConnectionSettings) => ConnectionSettings
  merge: (base: ConnectionSettings, form: ConnectionSettings) => ConnectionSettings
  fields: (form: ConnectionSettings, set: (patch: Partial<ConnectionSettings>) => void) => ReactNode
  extra?: ReactNode
  // Shown under the buttons: things that act on the saved connection
  // right away, apart from the form.
  below?: ReactNode
}) {
  const [form, setForm] = useState<ConnectionSettings>(saved)
  const [draftTest, setDraftTest] = useState<ConnectionTest | null>(null)
  const set = (patch: Partial<ConnectionSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setDraftTest(null)
  }
  const dirty = !same(form, saved)

  const save = useMutation({
    mutationFn: () => api.saveConnections(merge(saved, form)),
    onSuccess: () => {
      notify.ok(`${title} saved`)
      onSaved()
    },
    onError: notify.err,
  })
  const test = useMutation({
    mutationFn: () => api.testConnections(only(form)),
    onSuccess: (results) => {
      const mine = results.find((t) => t.service === service)
      if (mine) setDraftTest(mine)
      else notify.err('Nothing to test — enter a URL first')
    },
    onError: notify.err,
  })
  const busy = save.isPending || test.isPending

  return (
    <EditorModal title={title} dirty={dirty} busy={busy} onClose={onClose}>
      <Stack gap="sm">
        {blurb && (
          <Text size="sm" c="dimmed">
            {blurb}
          </Text>
        )}
        {readOnly && (
          <Alert color="gray" py="xs" icon={<IconLock size={16} />} title="Read-only">
            <Text size="xs">
              Its address is set by the deployment&apos;s configuration (
              <code>SENTO_EXTENSIONS_URL</code>). Change it there and restart Sento.
            </Text>
          </Alert>
        )}
        {live && !live.ok && (
          <Alert color="red" py="xs" title="Not reachable right now">
            <Text size="xs" className="wrapAnywhere">
              {live.error}
            </Text>
          </Alert>
        )}
        <fieldset disabled={busy} className="formFields">
          {fields(form, set)}
        </fieldset>
        <Group justify="space-between" mt="xs">
          <Group gap="xs">
            <Button
              variant="default"
              size="xs"
              onClick={() => test.mutate()}
              loading={test.isPending}
              disabled={!isSet(form) || save.isPending}
            >
              Test
            </Button>
            {draftTest && <StatusBadge result={draftTest} configured draft />}
          </Group>
          <Group gap="xs">
            {extra}
            <Button variant="default" size="xs" onClick={onClose} disabled={busy}>
              {readOnly ? 'Close' : 'Cancel'}
            </Button>
            {!readOnly && (
              <Button
                size="xs"
                onClick={() => save.mutate()}
                loading={save.isPending}
                disabled={test.isPending || (alwaysSavable ? !isSet(form) : !dirty)}
              >
                Save
              </Button>
            )}
          </Group>
        </Group>
        {below}
      </Stack>
    </EditorModal>
  )
}
