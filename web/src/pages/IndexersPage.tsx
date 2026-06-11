import { useState } from 'react'
import { ActionIcon, Alert, Badge, NumberInput, Switch, Table, Text, Title, Tooltip } from '@mantine/core'
import { IconSettings } from '@tabler/icons-react'
import SourcePrefsModal from '../components/SourcePrefsModal'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type SiteView } from '../api'
import { notify, usePageTitle } from '../ui'

// Global indexer configuration: every site Sento can search, uniformly,
// with per-ledger enablement.
export default function IndexersPage() {
  usePageTitle('Indexers')
  const queryClient = useQueryClient()
  const { data: sites, error } = useQuery({
    queryKey: ['indexers'],
    queryFn: api.indexerSettings,
  })
  const connections = useQuery({ queryKey: ['connections'], queryFn: api.connections })
  const [prefsFor, setPrefsFor] = useState<SiteView | null>(null)
  const patch = useMutation({
    mutationFn: ({ s, p }: { s: SiteView; p: Parameters<typeof api.updateIndexerSetting>[2] }) =>
      api.updateIndexerSetting(s.provider, s.siteId, p),
    onError: notify.err,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['indexers'] }),
  })

  return (
    <>
      <Title order={2} mb="xs">
        Indexers
      </Title>
      <Text size="sm" c="dimmed" mb="md">
        Every site Sento can search — torrent indexers and extension sources alike. Configure
        which ledgers each site serves; series can override these defaults individually.
      </Text>
      {(error || patch.error) && (
        <Alert color="red" mb="md">
          {String(error ?? patch.error)}
        </Alert>
      )}
      <Table striped maw={760}>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Site</Table.Th>
            <Table.Th>Protocol</Table.Th>
            <Table.Th>Lang</Table.Th>
            <Table.Th>Enabled</Table.Th>
            <Table.Th>Volumes</Table.Th>
            <Table.Th>Chapters</Table.Th>
            <Table.Th>
              <Tooltip label="Tie-breaker between equally good releases: lower number wins">
                <span>Priority</span>
              </Tooltip>
            </Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {sites?.map((s) => (
            <Table.Tr key={`${s.provider}-${s.siteId}`} opacity={s.reachable ? 1 : 0.5}>
              <Table.Td>
                <Text size="sm">
                  {s.name}
                  {s.protocol === 'source' && connections.data?.sourceBackend === 'host' && (
                    <Tooltip label="Source settings (login, mirrors…)">
                      <ActionIcon
                        size="xs"
                        variant="subtle"
                        color="gray"
                        ml={4}
                        onClick={() => setPrefsFor(s)}
                      >
                        <IconSettings size={12} />
                      </ActionIcon>
                    </Tooltip>
                  )}
                </Text>
                <Text size="xs" c="dimmed">
                  {s.provider}/{s.siteId}
                </Text>
              </Table.Td>
              <Table.Td>
                <Badge size="xs" variant="outline" color={s.protocol === 'torrent' ? 'blue' : 'teal'}>
                  {s.protocol}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Text size="xs" c="dimmed">
                  {s.lang ?? '—'}
                </Text>
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  checked={s.enabled}
                  onChange={(e) => patch.mutate({ s, p: { enabled: e.currentTarget.checked } })}
                />
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  checked={s.useVolumes}
                  disabled={!s.enabled}
                  onChange={(e) => patch.mutate({ s, p: { useVolumes: e.currentTarget.checked } })}
                />
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  checked={s.useChapters}
                  disabled={!s.enabled}
                  onChange={(e) => patch.mutate({ s, p: { useChapters: e.currentTarget.checked } })}
                />
              </Table.Td>
              <Table.Td>
                <NumberInput
                  size="xs"
                  w={70}
                  min={0}
                  max={99}
                  defaultValue={s.priority}
                  disabled={!s.enabled}
                  onBlur={(e) => {
                    const v = Number(e.currentTarget.value)
                    if (!isNaN(v) && v !== s.priority) patch.mutate({ s, p: { priority: v } })
                  }}
                />
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
      {prefsFor && (
        <SourcePrefsModal
          sourceId={prefsFor.siteId}
          sourceName={prefsFor.name}
          onClose={() => setPrefsFor(null)}
        />
      )}
      {sites && sites.length === 0 && (
        <Text c="dimmed" size="sm">
          No providers configured — set Suwayomi and/or Prowlarr in the config.
        </Text>
      )}
    </>
  )
}
