import { useState } from 'react'
import {
  ActionIcon,
  Button,
  Checkbox,
  Group,
  Modal,
  Select,
  Stack,
  Text,
  TextInput,
} from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import { useMutation } from '@tanstack/react-query'
import { api, type Condition, type Format } from '../../api'
import { notify } from '../../ui'

const conditionTypes = [
  { value: 'title', label: 'Release title (regex)' },
  { value: 'group', label: 'Group / scanlator (regex)' },
  { value: 'language', label: 'Language' },
  { value: 'flag', label: 'Parsed fact' },
  { value: 'protocol', label: 'Protocol' },
  { value: 'site', label: 'Site (regex)' },
  { value: 'resolution', label: 'Min resolution (px)' },
]

const flagValues = [
  'digital',
  'scan',
  'web',
  'premium',
  'colored',
  'omnibus',
  'deluxe',
  'boxset',
  'official',
  'raw',
  'complete',
]

export function FormatModal({
  format,
  onClose,
  onSaved,
}: {
  format: Format | null
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(format?.name ?? '')
  const [conditions, setConditions] = useState<Condition[]>(
    format?.conditions?.length
      ? format.conditions.map((c) => ({ ...c }))
      : [{ type: 'title', value: '' }],
  )

  const setCond = (i: number, patch: Partial<Condition>) =>
    setConditions((cs) => cs.map((c, j) => (j === i ? { ...c, ...patch } : c)))

  const save = useMutation({
    mutationFn: () => {
      const body = { name, conditions }
      return format ? api.updateFormat(format.id, body) : api.createFormat(body).then(() => {})
    },
    onSuccess: () => {
      onSaved()
      onClose()
    },
    onError: (e: Error) => notify.err(e),
  })

  return (
    <Modal
      opened
      onClose={onClose}
      title={format ? `Edit ${format.name}` : 'Add custom format'}
      size="lg"
    >
      <Stack gap="sm">
        <TextInput
          label="Name"
          value={name}
          onChange={(e) => setName(e.currentTarget.value)}
          required
        />
        <Text size="sm" fw={600}>
          Conditions
        </Text>
        {conditions.map((c, i) => (
          <Group key={i} gap={6} align="flex-end" wrap="nowrap">
            <Select
              size="xs"
              w={190}
              data={conditionTypes}
              value={c.type}
              onChange={(v) => v && setCond(i, { type: v, value: '' })}
            />
            {c.type === 'flag' ? (
              <Select
                size="xs"
                style={{ flex: 1 }}
                data={flagValues}
                value={c.value || null}
                onChange={(v) => v && setCond(i, { value: v })}
              />
            ) : c.type === 'protocol' ? (
              <Select
                size="xs"
                style={{ flex: 1 }}
                data={['torrent', 'source']}
                value={c.value || null}
                onChange={(v) => v && setCond(i, { value: v })}
              />
            ) : (
              <TextInput
                size="xs"
                style={{ flex: 1 }}
                placeholder={
                  c.type === 'resolution'
                    ? '1600'
                    : c.type === 'language'
                      ? 'fr, or * for any stated language'
                      : 'regex…'
                }
                value={c.value}
                onChange={(e) => setCond(i, { value: e.currentTarget.value })}
              />
            )}
            <Checkbox
              size="xs"
              label="Negate"
              checked={c.negate ?? false}
              onChange={(e) => setCond(i, { negate: e.currentTarget.checked })}
            />
            <Checkbox
              size="xs"
              label="Required"
              checked={c.required ?? false}
              onChange={(e) => setCond(i, { required: e.currentTarget.checked })}
            />
            <ActionIcon
              variant="subtle"
              color="red"
              disabled={conditions.length === 1}
              onClick={() => setConditions((cs) => cs.filter((_, j) => j !== i))}
            >
              <IconTrash size={14} />
            </ActionIcon>
          </Group>
        ))}
        <Button
          size="compact-xs"
          variant="default"
          leftSection={<IconPlus size={12} />}
          onClick={() => setConditions((cs) => [...cs, { type: 'title', value: '' }])}
          style={{ alignSelf: 'flex-start' }}
        >
          Add condition
        </Button>
        <Group justify="flex-end" mt="xs">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={() => save.mutate()}
            loading={save.isPending}
            disabled={!name.trim() || conditions.some((c) => !c.value.trim())}
          >
            Save
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
