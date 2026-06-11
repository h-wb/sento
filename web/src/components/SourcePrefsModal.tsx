import { useState } from 'react'
import {
  Alert,
  Loader,
  Modal,
  MultiSelect,
  PasswordInput,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Preference } from '../types/sourcehost'
import { notify } from '../ui'

// Renders a source's ConfigurableSource preference screen (logins,
// mirrors, quality…). Values save on change: Sento pushes them to the
// host and stores them for replay onto fresh hosts.
export default function SourcePrefsModal({
  sourceId,
  sourceName,
  onClose,
}: {
  sourceId: string
  sourceName: string
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const prefs = useQuery({
    queryKey: ['source-prefs', sourceId],
    queryFn: () => api.sourcePreferences(sourceId),
  })
  const save = useMutation({
    mutationFn: ({ key, value }: { key: string; value: unknown }) =>
      api.setSourcePreference(sourceId, key, value),
    onSuccess: () => {
      notify.ok('Preference saved')
      queryClient.invalidateQueries({ queryKey: ['source-prefs', sourceId] })
    },
    onError: notify.err,
  })

  return (
    <Modal opened onClose={onClose} title={`${sourceName} settings`} size="lg">
      {prefs.isPending && <Loader size="sm" />}
      {prefs.error && <Alert color="red">{String(prefs.error)}</Alert>}
      {prefs.data && prefs.data.length === 0 && (
        <Text size="sm" c="dimmed">
          This source has no settings.
        </Text>
      )}
      <Stack gap="sm">
        {prefs.data?.map((p) => (
          <PrefField key={p.key} pref={p} onSave={(value) => save.mutate({ key: p.key, value })} />
        ))}
      </Stack>
    </Modal>
  )
}

const isSecret = (p: Preference) =>
  /pass|secret|token|api.?key/i.test(`${p.key} ${p.title ?? ''}`)

function PrefField({ pref, onSave }: { pref: Preference; onSave: (value: unknown) => void }) {
  const current = pref.current ?? pref.default

  if (pref.valueType === 'Boolean') {
    return (
      <Switch
        label={pref.title ?? pref.key}
        description={pref.summary}
        checked={current === true}
        onChange={(e) => onSave(e.currentTarget.checked)}
      />
    )
  }

  if (pref.type === 'ListPreference') {
    const options = (pref.entryValues ?? []).map((v, i) => ({
      value: v,
      label: pref.entries?.[i] ?? v,
    }))
    return (
      <Select
        label={pref.title ?? pref.key}
        description={pref.summary}
        data={options}
        value={typeof current === 'string' ? current : null}
        onChange={(v) => v !== null && onSave(v)}
        allowDeselect={false}
      />
    )
  }

  if (pref.type === 'MultiSelectListPreference') {
    const options = (pref.entryValues ?? []).map((v, i) => ({
      value: v,
      label: pref.entries?.[i] ?? v,
    }))
    return (
      <MultiSelect
        label={pref.title ?? pref.key}
        description={pref.summary}
        data={options}
        value={Array.isArray(current) ? (current as string[]) : []}
        onChange={(v) => onSave(v)}
      />
    )
  }

  // EditTextPreference (and anything else string-typed): edit locally,
  // save on blur/Enter.
  return <TextPref pref={pref} value={typeof current === 'string' ? current : ''} onSave={onSave} />
}

function TextPref({
  pref,
  value,
  onSave,
}: {
  pref: Preference
  value: string
  onSave: (value: unknown) => void
}) {
  const [text, setText] = useState(value)
  const Input = isSecret(pref) ? PasswordInput : TextInput
  return (
    <Input
      label={pref.title ?? pref.key}
      description={pref.summary}
      value={text}
      onChange={(e) => setText(e.currentTarget.value)}
      onBlur={() => text !== value && onSave(text)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && text !== value) onSave(text)
      }}
    />
  )
}
