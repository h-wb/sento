import { Text, TextInput } from '@mantine/core'

// A naming-template field with its live rendered preview (and validation
// error) underneath — the same input on the Media settings page and in the
// edition editor.
export function TemplateInput({
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
        label={label}
        description={description}
        value={value}
        placeholder={placeholder}
        error={error}
        disabled={disabled}
        onChange={(e) => onChange(e.currentTarget.value)}
        onFocus={(e) => onFocus?.(e.currentTarget)}
      />
      {!error && (
        <Text size="xs" c="dimmed" mt={2} style={{ fontFamily: 'monospace', overflowWrap: 'anywhere' }}>
          Preview: {preview ?? 'Loading…'}
        </Text>
      )}
    </div>
  )
}
