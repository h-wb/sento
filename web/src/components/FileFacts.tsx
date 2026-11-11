import { Chip, Group, Stack, Text, TextInput } from '@mantine/core'
import { IconMinus } from '@tabler/icons-react'
import type { File as SeriesFile } from '../types/store'
import { factFlags, hasFlag, manualFlag } from './factFlags'

// What files are, as release profiles judge them: language, group and
// flags. Sento reads them from a file's name and its release; here a person
// corrects what it read wrong, for one file or for several at once.
// `flags` holds only the flags changed in this form, and an empty language
// or group leaves each file's as it was read.
export function FileFacts({
  files,
  language,
  group,
  flags,
  disabled,
  onLanguage,
  onGroup,
  onFlags,
}: {
  files: SeriesFile[]
  language: string
  group: string
  flags: Record<string, boolean>
  disabled?: boolean
  onLanguage: (v: string) => void
  onGroup: (v: string) => void
  onFlags: (v: Record<string, boolean>) => void
}) {
  const several = files.length > 1
  // Read on every file, on none, or on some of them.
  const read = (key: string) => {
    const n = files.filter((f) => hasFlag(f, key)).length
    return n === files.length ? true : n === 0 ? false : undefined
  }
  const mixed = (key: string) => !(key in flags) && read(key) === undefined
  const toggle = (key: string) => {
    const next = { ...flags }
    const was = read(key)
    const now = flags[key] ?? was
    // On, off, then back to what was read, which is no change at all.
    const to = now === undefined ? true : was === undefined && now === false ? undefined : !now
    if (to === undefined || to === was) delete next[key]
    else next[key] = to
    onFlags(next)
  }
  const common = (key: 'language' | 'group') => {
    const values = new Set(files.map((f) => f.facts?.[key] || f.claims?.[key] || ''))
    return values.size > 1 ? 'Differs between files' : [...values][0] || 'Not known'
  }
  const changes = factFlags
    .filter((f) => f.key in flags)
    .map((f) => (flags[f.key] ? f.label : `not ${f.label}`))
  const yours = several
    ? []
    : factFlags
        .filter((f) => !(f.key in flags) && manualFlag(files[0], f.key))
        .map((f) => (hasFlag(files[0], f.key) ? f.label : `not ${f.label}`))
  return (
    <Stack gap="sm">
      <Group grow align="flex-start">
        <TextInput
          label="Language"
          placeholder={several ? common('language') : undefined}
          disabled={disabled}
          value={language}
          onChange={(e) => onLanguage(e.currentTarget.value)}
        />
        <TextInput
          label="Release group"
          placeholder={several ? common('group') : undefined}
          disabled={disabled}
          value={group}
          onChange={(e) => onGroup(e.currentTarget.value)}
        />
      </Group>
      <div>
        <Group gap={6} role="group" aria-label="Flags">
          {factFlags.map((f) => {
            const some = mixed(f.key)
            return (
              <Chip
                key={f.key}
                size="xs"
                variant="outline"
                disabled={disabled}
                checked={some || (flags[f.key] ?? read(f.key) ?? false)}
                icon={some ? <IconMinus size={12} /> : undefined}
                title={some ? 'Some of these files' : undefined}
                onChange={() => toggle(f.key)}
              >
                {f.label}
              </Chip>
            )
          })}
        </Group>
        <Text size="xs" c="dimmed" mt={8}>
          {changes.length > 0
            ? `${several ? 'These files become' : 'This file becomes'}: ${changes.join(', ')}.`
            : several
              ? 'Tick what every file is, untick what none is.'
              : 'Tick what the file is.'}
          {factFlags.some((f) => mixed(f.key)) && ' A dash marks what only some of them are.'}
          {yours.length > 0 && ` Set by you: ${yours.join(', ')}.`}
          {(changes.length > 0 || yours.length > 0) && ' A rescan keeps it.'}
        </Text>
      </div>
    </Stack>
  )
}
