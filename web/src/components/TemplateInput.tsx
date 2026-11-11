import { Text, TextInput } from '@mantine/core'

// A naming-template field with its live rendered preview (and validation
// error) underneath — the same input on the Media settings page and in the
// edition editor.
export function TemplateInput({
  id,
  label,
  description,
  value,
  preview,
  error,
  disabled,
  placeholder,
  onChange,
  onFocus,
}: {
  id?: string
  label: string
  description?: string
  value: string
  preview?: string
  error?: string
  disabled?: boolean
  placeholder?: string
  onChange: (v: string) => void
  // The input, when it takes focus: a token picked elsewhere goes into it.
  onFocus?: (input: HTMLInputElement) => void
}) {
  return (
    <div>
      <TextInput
        id={id}
        label={label}
        description={description}
        value={value}
        placeholder={placeholder}
        error={error}
        disabled={disabled}
        onChange={(e) => onChange(e.currentTarget.value)}
        onFocus={(e) => onFocus?.(e.currentTarget)}
        // The template and its preview in one font, so they read alike.
        styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12.5 } }}
      />
      {!error && (
        <Text size="xs" c="dimmed" mt={4} style={{ fontFamily: 'var(--mantine-font-family-monospace)', overflowWrap: 'anywhere' }}>
          Preview: {preview ?? 'Loading…'}
        </Text>
      )}
    </div>
  )
}
