import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  ActionIcon,
  Badge,
  Button,
  Divider,
  Group,
  Stack,
  Table,
  Text,
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
import { api, type Condition, type Format, type Profile } from '../api'
import { defaultProfileQuery, formatsQuery, invalidateProfiles, profilesQuery } from '../queries'
import { notify, usePageTitle } from '../ui'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { useConfirmAction } from '../components/ConfirmAction'
import { PageHeader } from '../components/PageHeader'
import { ProfileModal } from '../components/profiles/ProfileModal'
import { FormatModal } from '../components/profiles/FormatModal'
import { ImportProfileModal } from '../components/profiles/ImportProfileModal'
import { MinScoreOff } from '../types/profile'

// condText renders a condition compactly: "title ~ \bLQ\b", "NOT flag:raw".
function condText(c: Condition): string {
  const body =
    c.type === 'flag' || c.type === 'protocol' ? `${c.type}:${c.value}` : `${c.type} ~ ${c.value}`
  return `${c.negate ? 'NOT ' : ''}${body}${c.required ? ' (required)' : ''}`
}

function downloadJSON(name: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.json`
  a.click()
  URL.revokeObjectURL(url)
}

export default function ProfilesPage() {
  usePageTitle('Profiles')
  const { confirm, confirmation } = useConfirmAction()
  const qc = useQueryClient()
  const profiles = useQuery(profilesQuery)
  const formats = useQuery(formatsQuery)
  const formatName = (id: number) => formats.data?.find((f) => f.id === id)?.name ?? `format ${id}`
  const defaultProfile = useQuery(defaultProfileQuery)

  const [editProfile, setEditProfile] = useState<Profile | 'new' | null>(null)
  // ?profile=<id> opens that profile (a search's "Open profile" link).
  const [searchParams, setSearchParams] = useSearchParams()
  const linked = profiles.data?.find((p) => String(p.id) === searchParams.get('profile'))
  const openProfile = editProfile ?? linked ?? null
  const closeProfile = () => {
    setEditProfile(null)
    if (searchParams.has('profile')) setSearchParams({}, { replace: true })
  }
  const [editFormat, setEditFormat] = useState<Format | 'new' | null>(null)
  const [importOpen, setImportOpen] = useState(false)

  const invalidate = () => {
    void invalidateProfiles(qc)
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
        description="Which releases Sento downloads, and which it prefers."
      />

      <Group justify="space-between">
        <div>
          <Title order={3}>Release profiles</Title>
          <Text c="dimmed" size="sm">
            A series or edition uses the starred profile unless it chooses another.
          </Text>
        </div>
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
      </Group>

      <QueryFeedback
        pending={profiles.isPending}
        error={profiles.error || defaultProfile.error}
        onRetry={() => {
          void profiles.refetch()
          void defaultProfile.refetch()
        }}
      />
      <Table.ScrollContainer minWidth={560}>
        <Table verticalSpacing="sm" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th w={32} />
              <Table.Th>Name</Table.Th>
              <Table.Th>Min format score</Table.Th>
              <Table.Th>Scored formats</Table.Th>
              <Table.Th>Rules</Table.Th>
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
                        aria-label={
                          isDefault ? `${p.name} is the default profile` : `Make ${p.name} default`
                        }
                        disabled={setDefault.isPending}
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
                      {p.minFormatScore <= MinScoreOff ? 'No minimum' : p.minFormatScore}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed">
                      {Object.keys(p.formatScores ?? {}).length}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed">
                      {[
                        ...(p.required ?? []).map((id) => `Requires ${formatName(id)}`),
                        ...(p.refused ?? []).map((id) => `Refuses ${formatName(id)}`),
                      ].join(' · ') || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} justify="flex-end" wrap="nowrap">
                      <Tooltip label="Export JSON">
                        <ActionIcon
                          variant="subtle"
                          aria-label={`Export ${p.name}`}
                          onClick={() => void exportOne(p).catch(notify.err)}
                        >
                          <IconDownload size={16} />
                        </ActionIcon>
                      </Tooltip>
                      <ActionIcon
                        variant="subtle"
                        aria-label={`Edit ${p.name}`}
                        onClick={() => setEditProfile(p)}
                      >
                        <IconPencil size={16} />
                      </ActionIcon>
                      <ActionIcon
                        variant="subtle"
                        color="red"
                        disabled={isDefault}
                        aria-label={`Delete ${p.name}`}
                        onClick={() =>
                          confirm({
                            title: 'Delete profile?',
                            message: `Delete “${p.name}”? Only unused profiles can be deleted. Reassign any series or editions using it first.`,
                            label: 'Delete profile',
                            onConfirm: () => deleteProfile.mutateAsync(p.id),
                          })
                        }
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
      </Table.ScrollContainer>
      {profiles.data?.length === 0 && (
        <EmptyState
          title="No profiles"
          description="Create a profile to choose which releases Sento prefers."
        />
      )}

      <Divider />

      <Group justify="space-between">
        <div>
          <Title order={3}>Custom formats</Title>
          <Text c="dimmed" size="sm">
            Match release details such as language, quality, or group. Profiles assign a score to
            each match.
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

      <QueryFeedback
        pending={formats.isPending}
        error={formats.error}
        onRetry={() => void formats.refetch()}
      />
      <Table.ScrollContainer minWidth={520}>
        <Table verticalSpacing="sm" highlightOnHover>
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
                    <ActionIcon
                      variant="subtle"
                      aria-label={`Edit format ${f.name}`}
                      onClick={() => setEditFormat(f)}
                    >
                      <IconPencil size={16} />
                    </ActionIcon>
                    <ActionIcon
                      variant="subtle"
                      color="red"
                      aria-label={`Delete format ${f.name}`}
                      onClick={() =>
                        confirm({
                          title: 'Delete custom format?',
                          message: `Delete “${f.name}”? Its scores and rules will be removed from every profile.`,
                          label: 'Delete format',
                          onConfirm: () => deleteFormat.mutateAsync(f.id),
                        })
                      }
                    >
                      <IconTrash size={16} />
                    </ActionIcon>
                  </Group>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      {formats.data?.length === 0 && (
        <Text size="sm" c="dimmed">
          No custom formats. Add one to prefer or avoid specific release details.
        </Text>
      )}
      {confirmation}

      {openProfile && (
        <ProfileModal
          profile={openProfile === 'new' ? null : openProfile}
          formats={formats.data ?? []}
          onClose={closeProfile}
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
      {importOpen && <ImportProfileModal onClose={() => setImportOpen(false)} onSaved={invalidate} />}
    </Stack>
  )
}
