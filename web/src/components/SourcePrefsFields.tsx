import {
  Alert,
  Loader,
  MultiSelect,
  PasswordInput,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import type { Preference } from '../types/sourcehost'

// Embeds a source's ConfigurableSource preference screen (logins,
// mirrors, quality…) as form fields. Controlled: edits land in `values`
// as an overlay over the host's current values, and the owning form
// pushes them on save — Sento stores pushed values for replay onto
// fresh hosts.
export default function SourcePrefsFields({
  sourceId,
  values,
  onChange,
}: {
  sourceId: string
  values: Record<string, unknown>
  onChange: (key: string, value: unknown) => void
}) {
  const prefs = useQuery({
    queryKey: ['source-prefs', sourceId],
    queryFn: () => api.sourcePreferences(sourceId),
  })

  if (prefs.isPending) return <Loader size="sm" />
  if (prefs.error) return <Alert color="red">{String(prefs.error)}</Alert>
  if (!prefs.data || prefs.data.length === 0) {
    return (
      <Text size="sm" c="dimmed">
        This source has no settings of its own.
      </Text>
    )
  }
  return (
    <Stack gap="sm">
      {prefs.data.map((p) => (
        <PrefField
          key={p.key}
          pref={p}
          value={p.key in values ? values[p.key] : (p.current ?? p.default)}
          onChange={(v) => onChange(p.key, v)}
        />
      ))}
    </Stack>
  )
}

const isSecret = (p: Preference) =>
  /pass|secret|token|api.?key/i.test(`${p.key} ${p.title ?? ''}`)

function PrefField({
  pref,
  value,
  onChange,
}: {
  pref: Preference
  value: unknown
  onChange: (value: unknown) => void
}) {
  if (pref.valueType === 'Boolean') {
    return (
      <Switch
        label={pref.title ?? pref.key}
        description={pref.summary}
        checked={value === true}
        onChange={(e) => onChange(e.currentTarget.checked)}
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
        value={typeof value === 'string' ? value : null}
        onChange={(v) => v !== null && onChange(v)}
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
        value={Array.isArray(value) ? (value as string[]) : []}
        onChange={(v) => onChange(v)}
      />
    )
  }

  // EditTextPreference (and anything else string-typed).
  const Input = isSecret(pref) ? PasswordInput : TextInput
  return (
    <Input
      label={pref.title ?? pref.key}
      description={pref.summary}
      value={typeof value === 'string' ? value : ''}
      onChange={(e) => onChange(e.currentTarget.value)}
    />
  )
}
