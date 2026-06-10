import { Alert, Badge, Switch, Table, Text, Title } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type SiteView } from '../api'

// Global indexer configuration: every site Sento can search, uniformly,
// with per-ledger enablement.
export default function IndexersPage() {
  const queryClient = useQueryClient()
  const { data: sites, error } = useQuery({
    queryKey: ['indexers'],
    queryFn: api.indexerSettings,
  })
  const patch = useMutation({
    mutationFn: ({ s, p }: { s: SiteView; p: Parameters<typeof api.updateIndexerSetting>[2] }) =>
      api.updateIndexerSetting(s.provider, s.siteId, p),
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
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {sites?.map((s) => (
            <Table.Tr key={`${s.provider}-${s.siteId}`} opacity={s.reachable ? 1 : 0.5}>
              <Table.Td>
                <Text size="sm">{s.name}</Text>
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
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
      {sites && sites.length === 0 && (
        <Text c="dimmed" size="sm">
          No providers configured — set Suwayomi and/or Prowlarr in the config.
        </Text>
      )}
    </>
  )
}
