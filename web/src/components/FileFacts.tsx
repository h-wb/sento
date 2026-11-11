import { Chip, Group, Stack, Text, TextInput } from '@mantine/core'
import type { File as SeriesFile } from '../types/store'
import { factFlags, hasFlag, manualFlag } from './factFlags'

// What a file is, as release profiles judge it: its language, its group
// and its flags. Sento reads them from the file's name and its release;
// here a person corrects what it read wrong. `flags` holds only the flags
// changed in this form.
export function FileFacts({
  file,
  language,
  group,
  flags,
  disabled,
  onLanguage,
  onGroup,
  onFlags,
}: {
  file: SeriesFile
  language: string
  group: string
  flags: Record<string, boolean>
  disabled?: boolean
  onLanguage: (v: string) => void
  onGroup: (v: string) => void
  onFlags: (v: Record<string, boolean>) => void
}) {
  const on = (key: string) => flags[key] ?? hasFlag(file, key)
  const toggle = (key: string) => {
    const next = { ...flags }
    // Back to what was read is no change at all.
    if (!on(key) === hasFlag(file, key)) delete next[key]
    else next[key] = !on(key)
    onFlags(next)
  }
  const yours = factFlags
    .filter((f) => f.key in flags || manualFlag(file, f.key))
    .map((f) => `${f.label} ${on(f.key) ? 'on' : 'off'}`)
  return (
    <Stack gap="xs">
      <Text size="xs" c="dimmed">
        Read from the file’s name and its release. Release profiles accept or refuse the file on
        these, so correct anything that was read wrong.
      </Text>
      <Group grow>
        <TextInput
          label="Language"
          disabled={disabled}
          value={language}
          onChange={(e) => onLanguage(e.currentTarget.value)}
        />
        <TextInput
          label="Release group"
          disabled={disabled}
          value={group}
          onChange={(e) => onGroup(e.currentTarget.value)}
        />
      </Group>
      <Group gap={6} role="group" aria-label="Flags">
        {factFlags.map((f) => (
          <Chip
            key={f.key}
            size="xs"
            variant="outline"
            disabled={disabled}
            checked={on(f.key)}
            onChange={() => toggle(f.key)}
          >
            {f.label}
          </Chip>
        ))}
      </Group>
      {yours.length > 0 && (
        <Text size="xs" c="dimmed">
          Set by you: {yours.join(', ')}. A rescan keeps these.
        </Text>
      )}
    </Stack>
  )
}
