import { useState } from 'react'
import {
  Accordion,
  Badge,
  Button,
  Group,
  Modal,
  Stack,
  Text,
} from '@mantine/core'
import { IconPackage, IconRefresh } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type ExtensionView } from '../../api'
import { extensionsQuery, invalidateExtensions } from '../../queries'
import { notify } from '../../ui'
import { QueryFeedback } from '../QueryFeedback'

// ManageExtensionsModal is the shared extension manager: what the sidecar
// has installed, what the repos offer, and explicit Update actions. Repos
// themselves stay on the Connections page.
export function ManageExtensionsModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient()
  const extensions = useQuery({
    ...extensionsQuery,
    refetchOnMount: 'always',
  })
  const refresh = () => {
    invalidateExtensions(queryClient)
  }
  const update = useMutation({
    mutationFn: (pkg: string) => api.installExtension(pkg),
    onError: notify.err,
    onSettled: refresh,
  })
  const updateAll = useMutation({
    mutationFn: async () => {
      for (const e of extensions.data ?? []) {
        if (e.installedVersion && e.hasUpdate) await api.installExtension(e.pkgName)
      }
    },
    onSuccess: () => notify.ok('Extensions updated'),
    onError: notify.err,
    onSettled: refresh,
  })
  const list = extensions.data ?? []
  const updatable = list.filter((e) => e.installedVersion && e.hasUpdate)
  const [open, setOpen] = useState<string | null>(null)

  return (
    <Modal opened onClose={onClose} title="Manage extensions" size="lg">
      <Stack gap="sm">
        <Group justify="space-between" wrap="nowrap">
          <Text size="xs" c="dimmed">
            Installed and available versions on the extensions sidecar. Updates are explicit — a
            failed update leaves the installed version working. Repos are managed on{' '}
            <Link to="/connections" onClick={onClose}>
              Connections
            </Link>
            .
          </Text>
          {updatable.length > 0 && (
            <Button
              size="xs"
              variant="default"
              leftSection={<IconRefresh size={14} />}
              loading={updateAll.isPending}
              onClick={() => updateAll.mutate()}
            >
              Update all ({updatable.length})
            </Button>
          )}
        </Group>
        <QueryFeedback
          pending={extensions.isPending}
          error={extensions.error}
          onRetry={() => void extensions.refetch()}
        />
        {!!list.length && (
          <Accordion value={open} onChange={setOpen} variant="separated">
            {list.map((e) => (
              <Accordion.Item key={e.pkgName} value={e.pkgName}>
                <Accordion.Control
                  icon={<IconPackage size={16} />}
                  aria-label={`Details for ${e.name}`}
                >
                  <Group gap="xs" wrap="nowrap">
                    <Text size="sm" fw={500} truncate>
                      {e.name}
                    </Text>
                    <Badge size="xs" variant="outline" color="gray">
                      {e.lang}
                    </Badge>
                    {e.nsfw && (
                      <Badge size="xs" variant="outline" color="red">
                        nsfw
                      </Badge>
                    )}
                    {e.desired && (
                      <Badge size="xs" variant="outline" color="teal">
                        in use
                      </Badge>
                    )}
                    <Text size="xs" c="dimmed" ml="auto" style={{ flexShrink: 0 }}>
                      {e.installedVersion ? `v${e.installedVersion}` : 'not installed'}
                      {e.hasUpdate && e.availableVersion ? ` → v${e.availableVersion}` : ''}
                    </Text>
                    {e.installedVersion && e.hasUpdate && (
                      <Button
                        size="compact-xs"
                        variant="default"
                        leftSection={<IconRefresh size={12} />}
                        loading={update.isPending && update.variables === e.pkgName}
                        onClick={(ev) => {
                          ev.stopPropagation()
                          update.mutate(e.pkgName)
                        }}
                      >
                        Update
                      </Button>
                    )}
                  </Group>
                </Accordion.Control>
                <Accordion.Panel>
                  <Stack gap={4}>
                    <Text size="xs" c="dimmed">
                      Package: <code>{e.pkgName}</code>
                    </Text>
                    <Text size="xs" c="dimmed">
                      {e.installedVersion
                        ? e.hasUpdate
                          ? `Update available: v${e.installedVersion} → v${e.availableVersion}`
                          : 'Up to date with the configured repos.'
                        : `Available: v${e.availableVersion ?? '?'}`}
                    </Text>
                    <ExtensionSources ext={e} />
                  </Stack>
                </Accordion.Panel>
              </Accordion.Item>
            ))}
          </Accordion>
        )}
        {extensions.data && list.length === 0 && (
          <Text size="sm" c="dimmed">
            No extensions — add a repository on the Connections page to load a catalog.
          </Text>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Close
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

// ExtensionSources lists what one extension provides: as its repo index
// declares, or — when the index is silent — by asking the installed one.
function ExtensionSources({ ext }: { ext: ExtensionView }) {
  const declared = ext.sources ?? []
  const live = useQuery({
    queryKey: ['extension-sources', ext.pkgName],
    queryFn: () => api.extensionSources(ext.pkgName),
    enabled: declared.length === 0 && !!ext.installedVersion,
  })
  const srcs = declared.length > 0 ? declared : (live.data ?? [])
  if (srcs.length === 0) {
    return (
      <Text size="xs" c="dimmed">
        Sources: unknown until installed.
      </Text>
    )
  }
  return (
    <Text size="xs" c="dimmed">
      Sources: {srcs.map((s) => `${s.name} (${s.lang})`).join(', ')}
    </Text>
  )
}
