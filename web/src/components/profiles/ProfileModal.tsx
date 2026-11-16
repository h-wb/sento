import { useState } from 'react'
import {
  Button,
  Divider,
  Group,
  Modal,
  NumberInput,
  Select,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
} from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { api, type Format, type Profile } from '../../api'
import { MinScoreOff } from '../../types/profile'

// --- profile editor ---

export function ProfileModal({
  profile,
  formats,
  onClose,
  onSaved,
  fail,
}: {
  profile: Profile | null
  formats: Format[]
  onClose: () => void
  onSaved: () => void
  fail: (e: Error) => void
}) {
  const [name, setName] = useState(profile?.name ?? '')
  const [upgradesEnabled, setUpgradesEnabled] = useState(profile?.upgradesEnabled ?? false)
  const [upgradeUntilScore, setUpgradeUntilScore] = useState(profile?.upgradeUntilScore ?? 0)
  const [minUpgradeScore, setMinUpgradeScore] = useState(profile?.minUpgradeScore ?? 1)
  const [minScore, setMinScore] = useState<number>(profile?.minFormatScore ?? MinScoreOff)
  const [scores, setScores] = useState<Record<number, number>>({
    ...(profile?.formatScores ?? {}),
  })
  const [required, setRequired] = useState<number[]>(profile?.required ?? [])
  const [refused, setRefused] = useState<number[]>(profile?.refused ?? [])
  const [volume, setVolume] = useState(
    profile?.volume ?? {
      sizeMinMB: 20,
      sizeMaxMB: 1500,
      minSeeders: 1,
      preferredProtocol: 'torrent',
    },
  )
  const [chapter, setChapter] = useState(
    profile?.chapter ?? {
      sizeMinMB: 2,
      sizeMaxMB: 200,
      minSeeders: 1,
      preferredProtocol: 'source',
    },
  )

  const save = useMutation({
    mutationFn: () => {
      const body = {
        name,
        minFormatScore: minScore,
        upgradesEnabled,
        upgradeUntilScore,
        minUpgradeScore,
        required,
        refused,
        formatScores: Object.fromEntries(Object.entries(scores).filter(([, v]) => v !== 0)),
        volume,
        chapter,
      }
      return profile ? api.updateProfile(profile.id, body) : api.createProfile(body).then(() => {})
    },
    onSuccess: () => {
      onSaved()
      onClose()
    },
    onError: fail,
  })

  return (
    <Modal
      opened
      onClose={onClose}
      title={profile ? `Edit ${profile.name}` : 'Add profile'}
      size="lg"
    >
      <Stack gap="sm">
        <TextInput
          label="Name"
          value={name}
          onChange={(e) => setName(e.currentTarget.value)}
          required
        />
        <Switch
          label="Require a minimum format score"
          checked={minScore > MinScoreOff}
          onChange={(e) => setMinScore(e.currentTarget.checked ? 0 : MinScoreOff)}
        />
        {minScore > MinScoreOff && (
          <NumberInput
            label="Minimum format score"
            description="Releases with a lower score won't download automatically"
            value={minScore}
            onChange={(v) => setMinScore(Number(v) || 0)}
          />
        )}

        <Switch
          label="Download better releases automatically"
          description="Previous files stay on disk. Pinned files are never upgraded."
          checked={upgradesEnabled}
          onChange={(e) => setUpgradesEnabled(e.currentTarget.checked)}
        />
        {upgradesEnabled && (
          <Group grow>
            <NumberInput
              label="Minimum score improvement"
              min={1}
              value={minUpgradeScore}
              onChange={(v) => setMinUpgradeScore(Number(v) || 1)}
            />
            <NumberInput
              label="Stop upgrading at score"
              description="0 keeps looking for improvements"
              value={upgradeUntilScore}
              onChange={(v) => setUpgradeUntilScore(Number(v) || 0)}
            />
          </Group>
        )}
        <Divider label="Formats" />
        <Text size="sm" c="dimmed">
          Scores rank releases within this profile. Require every selected format, or refuse any match.
          These rules also apply to files on disk and cannot be overridden when downloading.
        </Text>
        <Table.ScrollContainer minWidth={460}>
        <Table verticalSpacing={4}>
          <Table.Thead>
            <Table.Tr><Table.Th>Format</Table.Th><Table.Th>Rule</Table.Th><Table.Th>Score</Table.Th></Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {formats.map((f) => (
              <Table.Tr key={f.id}>
                <Table.Td>
                  <Text size="sm">{f.name}</Text>
                </Table.Td>
                <Table.Td w={170}>
                  <Select
                    size="xs"
                    aria-label={`Rule for ${f.name}`}
                    allowDeselect={false}
                    data={[
                      { value: 'score', label: 'Score only' },
                      { value: 'required', label: 'Required' },
                      { value: 'refused', label: 'Refused' },
                    ]}
                    value={required.includes(f.id) ? 'required' : refused.includes(f.id) ? 'refused' : 'score'}
                    onChange={(rule) => {
                      setRequired((ids) => [...ids.filter((id) => id !== f.id), ...(rule === 'required' ? [f.id] : [])])
                      setRefused((ids) => [...ids.filter((id) => id !== f.id), ...(rule === 'refused' ? [f.id] : [])])
                    }}
                  />
                </Table.Td>
                <Table.Td w={120}>
                  <NumberInput
                    size="xs"
                    aria-label={`Score for ${f.name}`}
                    value={scores[f.id] ?? 0}
                    onChange={(v) => setScores((s) => ({ ...s, [f.id]: Number(v) || 0 }))}
                  />
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
        </Table.ScrollContainer>

        <Divider label="Download rules" />
        <Group grow align="flex-start">
          <RulesFields label="Volumes" rules={volume} onChange={setVolume} />
          <RulesFields label="Chapters" rules={chapter} onChange={setChapter} />
        </Group>

        <Group justify="flex-end" mt="xs">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!name.trim()}>
            Save
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

function RulesFields({
  label,
  rules,
  onChange,
}: {
  label: string
  rules: Profile['volume']
  onChange: (r: Profile['volume']) => void
}) {
  return (
    <Stack gap={6}>
      <Text size="sm" fw={600}>
        {label}
      </Text>
      <Select
        size="xs"
        label="Preferred download type"
        description="Adds 200 points to matching releases"
        data={[
          { value: 'torrent', label: 'Torrent' },
          { value: 'source', label: 'Source' },
        ]}
        value={rules.preferredProtocol}
        onChange={(v) => v && onChange({ ...rules, preferredProtocol: v })}
      />
      <Group grow gap={6}>
        <NumberInput
          size="xs"
          label="Min size (MB)"
          min={0}
          value={rules.sizeMinMB}
          onChange={(v) => onChange({ ...rules, sizeMinMB: Number(v) || 0 })}
        />
        <NumberInput
          size="xs"
          label="Max size (MB)"
          min={0}
          value={rules.sizeMaxMB}
          onChange={(v) => onChange({ ...rules, sizeMaxMB: Number(v) || 0 })}
        />
      </Group>
      <NumberInput
        size="xs"
        label="Min seeders (torrents)"
        min={0}
        value={rules.minSeeders}
        onChange={(v) => onChange({ ...rules, minSeeders: Number(v) || 0 })}
      />
    </Stack>
  )
}
