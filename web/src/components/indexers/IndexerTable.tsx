import { ActionIcon, Badge, Group, NumberInput, Switch, Table, Text, Tooltip } from '@mantine/core'
import { IconCopy, IconPencil, IconPlugConnected, IconSearch, IconTrash } from '@tabler/icons-react'
import type { UseMutationResult } from '@tanstack/react-query'
import { type ExtensionView, type IndexerPatch, type IndexerView } from '../../api'
import type { Confirm } from '../ConfirmAction'

export function IndexerTable({
  indexers,
  extByPkg,
  patch,
  test,
  remove,
  confirm,
  onSearchTest,
  onEdit,
  onDuplicate,
}: {
  indexers: IndexerView[]
  extByPkg: Map<string, ExtensionView>
  patch: UseMutationResult<IndexerView, unknown, { id: number; p: IndexerPatch }>
  test: UseMutationResult<void, unknown, IndexerView>
  remove: UseMutationResult<void, unknown, IndexerView>
  confirm: Confirm
  onSearchTest: (ix: IndexerView) => void
  onEdit: (ix: IndexerView) => void
  onDuplicate: (ix: IndexerView) => void
}) {
  return (
    <Table.ScrollContainer minWidth={700}>
      <Table highlightOnHover verticalSpacing="sm" className="dataTable">
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Name</Table.Th>
            <Table.Th>Protocol</Table.Th>
            <Table.Th>Enabled</Table.Th>
            <Table.Th>Volumes</Table.Th>
            <Table.Th>Chapters</Table.Th>
            <Table.Th>
              <Tooltip label="Tie-breaker between equally good releases: lower number wins">
                <span>Priority</span>
              </Tooltip>
            </Table.Th>
            <Table.Th />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {indexers.map((ix) => (
            <Table.Tr key={ix.id} opacity={ix.reachable ? 1 : 0.55}>
              <Table.Td>
                <Text size="sm">{ix.name}</Text>
                <Text size="xs" c="dimmed">
                  {ix.summary}
                  {!ix.reachable && ' — unreachable'}
                </Text>
                {(() => {
                  const pkg =
                    ix.protocol === 'source'
                      ? (ix.settings as { pkgName?: string } | undefined)?.pkgName
                      : undefined
                  const ext = pkg ? extByPkg.get(pkg) : undefined
                  if (!ext) return null
                  return (
                    <Text size="xs" c={ext.hasUpdate ? 'orange' : 'dimmed'}>
                      {ext.name} v{ext.installedVersion ?? '?'}
                      {ext.hasUpdate && ext.availableVersion
                        ? ` — v${ext.availableVersion} available`
                        : ''}
                    </Text>
                  )
                })()}
              </Table.Td>
              <Table.Td>
                <Badge
                  size="xs"
                  variant="outline"
                  color={ix.protocol === 'torznab' ? 'blue' : 'teal'}
                >
                  {ix.protocol}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  aria-label={`Enable ${ix.name}`}
                  disabled={patch.isPending}
                  checked={ix.enabled}
                  onChange={(e) =>
                    patch.mutate({ id: ix.id, p: { enabled: e.currentTarget.checked } })
                  }
                />
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  aria-label={`Search volumes on ${ix.name}`}
                  checked={ix.useVolumes}
                  disabled={!ix.enabled || patch.isPending}
                  onChange={(e) =>
                    patch.mutate({ id: ix.id, p: { useVolumes: e.currentTarget.checked } })
                  }
                />
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  aria-label={`Search chapters on ${ix.name}`}
                  checked={ix.useChapters}
                  disabled={!ix.enabled || patch.isPending}
                  onChange={(e) =>
                    patch.mutate({ id: ix.id, p: { useChapters: e.currentTarget.checked } })
                  }
                />
              </Table.Td>
              <Table.Td>
                <NumberInput
                  size="xs"
                  w={70}
                  min={0}
                  max={99}
                  key={`${ix.id}-${ix.priority}`}
                  aria-label={`Priority for ${ix.name}`}
                  defaultValue={ix.priority}
                  disabled={!ix.enabled || patch.isPending}
                  onBlur={(e) => {
                    const v = Number(e.currentTarget.value)
                    if (!isNaN(v) && v !== ix.priority)
                      patch.mutate({ id: ix.id, p: { priority: v } })
                  }}
                />
              </Table.Td>
              <Table.Td>
                <Group gap={2} wrap="nowrap" justify="flex-end">
                  {ix.protocol === 'torznab' && (
                    <Tooltip label="Test search">
                      <ActionIcon
                        size="md"
                        variant="subtle"
                        aria-label={`Test search on ${ix.name}`}
                        onClick={() => onSearchTest(ix)}
                      >
                        <IconSearch size={14} />
                      </ActionIcon>
                    </Tooltip>
                  )}
                  <Tooltip label="Test">
                    <ActionIcon
                      size="md"
                      variant="subtle"
                      aria-label={`Test ${ix.name}`}
                      loading={test.isPending && test.variables?.id === ix.id}
                      onClick={() => test.mutate(ix)}
                    >
                      <IconPlugConnected size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Edit">
                    <ActionIcon
                      size="md"
                      variant="subtle"
                      aria-label={`Edit ${ix.name}`}
                      onClick={() => onEdit(ix)}
                    >
                      <IconPencil size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Duplicate">
                    <ActionIcon
                      size="md"
                      variant="subtle"
                      aria-label={`Duplicate ${ix.name}`}
                      onClick={() => onDuplicate(ix)}
                    >
                      <IconCopy size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Delete">
                    <ActionIcon
                      size="sm"
                      variant="subtle"
                      color="red"
                      aria-label={`Remove ${ix.name}`}
                      onClick={() =>
                        confirm({
                          title: 'Remove indexer?',
                          message: `“${ix.name}” will no longer be searched. If this is an extension's last indexer, its extension will be uninstalled.`,
                          label: 'Remove indexer',
                          onConfirm: () => remove.mutateAsync(ix),
                        })
                      }
                    >
                      <IconTrash size={14} />
                    </ActionIcon>
                  </Tooltip>
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  )
}
