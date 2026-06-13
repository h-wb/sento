import { useMemo, useState } from 'react'
import {
  ActionIcon,
  Badge,
  Button,
  Checkbox,
  Divider,
  FileButton,
  Group,
  Modal,
  NumberInput,
  Select,
  Stack,
  Table,
  Text,
  TextInput,
  Textarea,
  Title,
  Tooltip,
} from '@mantine/core'
import {
  IconDownload,
  IconPencil,
  IconPlus,
  IconStar,
  IconStarFilled,
  IconTrash,
  IconUpload,
} from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Condition, type Format, type Profile, type ProfileDoc } from '../api'
import { notify } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { MinScoreOff } from '../types/profile'

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

// condText renders a condition compactly: "title ~ \bLQ\b", "NOT flag:raw".
function condText(c: Condition): string {
  const body = c.type === 'flag' || c.type === 'protocol' ? `${c.type}:${c.value}` : `${c.type} ~ ${c.value}`
  return `${c.negate ? 'NOT ' : ''}${body}${c.required ? ' (required)' : ''}`
}

function downloadJSON(name: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.json`
  a.click()
  URL.revokeObjectURL(url)
}

export default function ProfilesPage() {
  const qc = useQueryClient()
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const formats = useQuery({ queryKey: ['formats'], queryFn: api.listFormats })
  const defaultProfile = useQuery({ queryKey: ['default-profile'], queryFn: api.defaultProfile })

  const [editProfile, setEditProfile] = useState<Profile | 'new' | null>(null)
  const [editFormat, setEditFormat] = useState<Format | 'new' | null>(null)
  const [importOpen, setImportOpen] = useState(false)

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['profiles'] })
    void qc.invalidateQueries({ queryKey: ['formats'] })
    void qc.invalidateQueries({ queryKey: ['default-profile'] })
  }
  const fail = (e: Error) => notify.err(e)

  const setDefault = useMutation({
    mutationFn: (id: number) => api.setDefaultProfile(id),
    onSuccess: invalidate,
    onError: fail,
  })
  const deleteProfile = useMutation({
    mutationFn: (id: number) => api.deleteProfile(id),
    onSuccess: invalidate,
    onError: fail,
  })
  const deleteFormat = useMutation({
    mutationFn: (id: number) => api.deleteFormat(id),
    onSuccess: invalidate,
    onError: fail,
  })

  const exportOne = async (p: Profile) => {
    const doc = await api.exportProfile(p.id)
    downloadJSON(p.name, doc)
  }

  return (
    <Stack gap="lg">
      <PageHeader
        title="Profiles"
        description="Custom formats score releases; profiles decide how much each format is worth per series."
        action={
          <Group gap="xs">
            <Button
              variant="default"
              size="xs"
              leftSection={<IconUpload size={14} />}
              onClick={() => setImportOpen(true)}
            >
              Import
            </Button>
            <Button
              size="xs"
              leftSection={<IconPlus size={14} />}
              onClick={() => setEditProfile('new')}
            >
              Add profile
            </Button>
          </Group>
        }
      />

      <Table verticalSpacing={6} highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th w={32} />
            <Table.Th>Name</Table.Th>
            <Table.Th>Min format score</Table.Th>
            <Table.Th>Scored formats</Table.Th>
            <Table.Th w={120} />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {(profiles.data ?? []).map((p) => {
            const isDefault = defaultProfile.data?.profileId === p.id
            return (
              <Table.Tr key={p.id}>
                <Table.Td>
                  <Tooltip label={isDefault ? 'Default profile' : 'Make default'}>
                    <ActionIcon
                      variant="subtle"
                      color={isDefault ? 'yellow' : 'gray'}
                      onClick={() => !isDefault && setDefault.mutate(p.id)}
                    >
                      {isDefault ? <IconStarFilled size={16} /> : <IconStar size={16} />}
                    </ActionIcon>
                  </Tooltip>
                </Table.Td>
                <Table.Td>
                  <Text size="sm" fw={500}>
                    {p.name}
                  </Text>
                </Table.Td>
                <Table.Td>
                  <Text size="sm" c="dimmed">
                    {p.minFormatScore <= MinScoreOff ? 'disabled' : p.minFormatScore}
                  </Text>
                </Table.Td>
                <Table.Td>
                  <Text size="sm" c="dimmed">
                    {Object.keys(p.formatScores ?? {}).length}
                  </Text>
                </Table.Td>
                <Table.Td>
                  <Group gap={4} justify="flex-end" wrap="nowrap">
                    <Tooltip label="Export JSON">
                      <ActionIcon variant="subtle" onClick={() => void exportOne(p)}>
                        <IconDownload size={16} />
                      </ActionIcon>
                    </Tooltip>
                    <ActionIcon variant="subtle" onClick={() => setEditProfile(p)}>
                      <IconPencil size={16} />
                    </ActionIcon>
                    <ActionIcon
                      variant="subtle"
                      color="red"
                      disabled={isDefault}
                      onClick={() => deleteProfile.mutate(p.id)}
                    >
                      <IconTrash size={16} />
                    </ActionIcon>
                  </Group>
                </Table.Td>
              </Table.Tr>
            )
          })}
        </Table.Tbody>
      </Table>

      <Divider />

      <Group justify="space-between">
        <div>
          <Title order={3}>Custom formats</Title>
          <Text c="dimmed" size="sm">
            A format matches when at least one condition of each type matches; required and negated
            conditions must always hold.
          </Text>
        </div>
        <Button
          size="xs"
          variant="default"
          leftSection={<IconPlus size={14} />}
          onClick={() => setEditFormat('new')}
        >
          Add format
        </Button>
      </Group>

      <Table verticalSpacing={6} highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th w={180}>Name</Table.Th>
            <Table.Th>Conditions</Table.Th>
            <Table.Th w={80} />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {(formats.data ?? []).map((f) => (
            <Table.Tr key={f.id}>
              <Table.Td>
                <Text size="sm" fw={500}>
                  {f.name}
                </Text>
              </Table.Td>
              <Table.Td>
                <Group gap={4} wrap="wrap">
                  {f.conditions.map((c, i) => (
                    <Badge
                      key={i}
                      size="xs"
                      variant="outline"
                      color={c.negate ? 'red' : 'gray'}
                      style={{ textTransform: 'none' }}
                    >
                      {condText(c)}
                    </Badge>
                  ))}
                </Group>
              </Table.Td>
              <Table.Td>
                <Group gap={4} justify="flex-end" wrap="nowrap">
                  <ActionIcon variant="subtle" onClick={() => setEditFormat(f)}>
                    <IconPencil size={16} />
                  </ActionIcon>
                  <ActionIcon variant="subtle" color="red" onClick={() => deleteFormat.mutate(f.id)}>
                    <IconTrash size={16} />
                  </ActionIcon>
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>

      {editProfile && (
        <ProfileModal
          profile={editProfile === 'new' ? null : editProfile}
          formats={formats.data ?? []}
          onClose={() => setEditProfile(null)}
          onSaved={invalidate}
        />
      )}
      {editFormat && (
        <FormatModal
          format={editFormat === 'new' ? null : editFormat}
          onClose={() => setEditFormat(null)}
          onSaved={invalidate}
        />
      )}
      {importOpen && <ImportModal onClose={() => setImportOpen(false)} onSaved={invalidate} />}
    </Stack>
  )
}

// --- profile editor ---

function ProfileModal({
  profile,
  formats,
  onClose,
  onSaved,
}: {
  profile: Profile | null
  formats: Format[]
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(profile?.name ?? '')
  const [minScore, setMinScore] = useState<number>(profile?.minFormatScore ?? MinScoreOff)
  const [scores, setScores] = useState<Record<number, number>>({ ...(profile?.formatScores ?? {}) })
  const [volume, setVolume] = useState(
    profile?.volume ?? { sizeMinMB: 20, sizeMaxMB: 1500, minSeeders: 1, preferredProtocol: 'torrent' },
  )
  const [chapter, setChapter] = useState(
    profile?.chapter ?? { sizeMinMB: 2, sizeMaxMB: 200, minSeeders: 1, preferredProtocol: 'source' },
  )

  const save = useMutation({
    mutationFn: () => {
      const body = {
        name,
        minFormatScore: minScore,
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
    onError: (e: Error) => notify.err(e),
  })

  return (
    <Modal opened onClose={onClose} title={profile ? `Edit ${profile.name}` : 'Add profile'} size="lg">
      <Stack gap="sm">
        <TextInput label="Name" value={name} onChange={(e) => setName(e.currentTarget.value)} required />
        <NumberInput
          label="Minimum format score"
          description={`Releases scoring below this are rejected (force-grab still works). ${MinScoreOff} disables the floor.`}
          value={minScore}
          onChange={(v) => setMinScore(Number(v) || 0)}
        />

        <Divider label="Format scores" />
        <Table verticalSpacing={4}>
          <Table.Tbody>
            {formats.map((f) => (
              <Table.Tr key={f.id}>
                <Table.Td>
                  <Text size="sm">{f.name}</Text>
                </Table.Td>
                <Table.Td w={120}>
                  <NumberInput
                    size="xs"
                    value={scores[f.id] ?? 0}
                    onChange={(v) => setScores((s) => ({ ...s, [f.id]: Number(v) || 0 }))}
                  />
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>

        <Divider label="Ledger rules" />
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
        label="Preferred protocol (+200)"
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

// --- format editor ---

function FormatModal({
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
    format?.conditions?.length ? format.conditions.map((c) => ({ ...c })) : [{ type: 'title', value: '' }],
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
    <Modal opened onClose={onClose} title={format ? `Edit ${format.name}` : 'Add custom format'} size="lg">
      <Stack gap="sm">
        <TextInput label="Name" value={name} onChange={(e) => setName(e.currentTarget.value)} required />
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
                placeholder={c.type === 'resolution' ? '1600' : c.type === 'language' ? 'fr' : 'regex…'}
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

// --- import ---

function ImportModal({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const [text, setText] = useState('')

  const doc = useMemo(() => {
    try {
      return JSON.parse(text) as ProfileDoc
    } catch {
      return null
    }
  }, [text])

  const importMut = useMutation({
    mutationFn: (d: ProfileDoc) => api.importProfile(d),
    onSuccess: (p) => {
      notify.ok(`Imported profile “${p.name}”`)
      onSaved()
      onClose()
    },
    onError: (e: Error) => notify.err(e),
  })

  return (
    <Modal opened onClose={onClose} title="Import profile" size="lg">
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          Paste an exported profile document, or pick a file. Formats are matched by name and
          updated; the profile is created or replaced by name.
        </Text>
        <FileButton
          accept="application/json"
          onChange={(f) => {
            if (f) void f.text().then(setText)
          }}
        >
          {(props) => (
            <Button {...props} variant="default" size="xs" style={{ alignSelf: 'flex-start' }}>
              Choose file…
            </Button>
          )}
        </FileButton>
        <Textarea
          autosize
          minRows={8}
          maxRows={20}
          placeholder='{"name": "…", "minFormatScore": -10000, "formats": [...]}'
          value={text}
          onChange={(e) => setText(e.currentTarget.value)}
          styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!doc} loading={importMut.isPending} onClick={() => doc && importMut.mutate(doc)}>
            Import
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
