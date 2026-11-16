import { useState, type ReactNode } from 'react'
import {
  Alert,
  Badge,
  Group,
  List,
  SimpleGrid,
  Stack,
  Text,
  Title,
  UnstyledButton,
} from '@mantine/core'
import { IconLock, IconPlus } from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { connectionsQuery, invalidateExtensions } from '../queries'
import { hostOf } from '../format'
import { usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { StatusBadge } from '../components/connections/ConnectionDialog'
import {
  ClientDialog,
  NewClientDialog,
  SidecarDialog,
  type Editing,
} from '../components/connections/ClientDialogs'
import type { DownloadClientConn } from '../types/service'
import classes from './ConnectionsPage.module.css'

// The download clients Sento fetches through. They come in two types:
// qBittorrent instances for torrents (any number), and the extensions
// sidecar for chapters from extension sources (one). Each is a card showing
// what it is and whether it answers right now; its settings open in a
// dialog, the sidecar's extension repos included. What gets searched lives
// on the Indexers page; this page is only the plumbing. Stored server-side;
// the yaml/env config only seeds the very first run, so a client given
// there is already on the page.
export default function ConnectionsPage() {
  usePageTitle('Connections')
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Editing | null>(null)
  const settings = useQuery(connectionsQuery)
  // Live state of what is saved, probed on load — no button to press to
  // find out whether a service is actually reachable.
  const status = useQuery({
    queryKey: ['connection-status', settings.data],
    queryFn: () => api.testConnections(settings.data!),
    enabled: !!settings.data,
    refetchInterval: 60_000,
  })

  if (settings.isPending || settings.error) return <>
    <PageHeader title="Connections" description="Download services and extension repositories." />
    <QueryFeedback pending={settings.isPending} error={settings.error} onRetry={() => void settings.refetch()} />
  </>
  const saved = settings.data
  const live = (service: string) => status.data?.find((t) => t.service === service)

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['connections'] })
    // The provider set changed: site lists and status are stale.
    invalidateExtensions(queryClient)
    queryClient.invalidateQueries({ queryKey: ['status'] })
  }
  const onSaved = () => {
    setEditing(null)
    refresh()
  }
  const clients = saved.downloadClients
  const sidecar = saved.extensions.url !== ''
  const nothingSet = clients.length === 0 && !sidecar
  const newClient = (): DownloadClientConn => ({
    id: clients.length === 0 ? 'default' : `client-${Date.now().toString(36)}`,
    name: clients.length === 0 ? 'qBittorrent' : '',
    url: '',
    username: '',
    password: '',
    category: 'sento',
  })

  return (
    <>
      <PageHeader
        title="Connections"
        description="The download clients Sento fetches through."
      />

      <Stack maw={900} gap="xl">
        {nothingSet && (
          <Alert color="blue" title="Nothing is connected yet">
            <List size="sm" type="ordered" spacing={2}>
              <List.Item>
                Add a download client: qBittorrent for torrents, or the extensions sidecar for
                extension sources.
              </List.Item>
              <List.Item>For extension sources, add a repository in the sidecar's settings.</List.Item>
              <List.Item>
                Choose what to search on the <Link to="/indexers">Indexers</Link> page.
              </List.Item>
            </List>
          </Alert>
        )}

        <section aria-label="Download clients">
          <Title order={4}>Download clients</Title>
          <Text size="sm" c="dimmed" mb="sm">
            qBittorrent downloads torrent releases; Sento must have access to its download folder to
            import files, and indexers use the default client unless they name another. The
            extensions sidecar downloads chapters from Mihon-compatible extension sources.
          </Text>
          <SimpleGrid cols={{ base: 1, xs: 2, md: 3 }} spacing="sm">
            {clients.map((client, index) => (
              <ConnectionTile
                key={client.id}
                name={client.name}
                detail={`qBittorrent · ${hostOf(client.url)} · ${client.category}`}
                tag={index === 0 ? 'Default' : undefined}
                status={<StatusBadge result={live(`downloadClient:${client.id}`)} configured probing={status.isFetching} />}
                onClick={() => setEditing({ kind: 'client', client })}
              />
            ))}
            {sidecar && (
              <ConnectionTile
                name="Extensions sidecar"
                detail={`Extension sources · ${hostOf(saved.extensions.url)}`}
                tag={saved.managed.extensionsUrl ? 'Read-only' : undefined}
                locked={saved.managed.extensionsUrl}
                status={<StatusBadge result={live('extensions')} configured probing={status.isFetching} />}
                onClick={() => setEditing({ kind: 'extensions' })}
              />
            )}
            <UnstyledButton className={classes.add} onClick={() => setEditing({ kind: 'new', client: newClient() })}>
              <IconPlus size={16} />
              <Text size="sm">Add download client</Text>
            </UnstyledButton>
          </SimpleGrid>
        </section>
      </Stack>

      {editing?.kind === 'new' && (
        <NewClientDialog
          saved={saved}
          client={editing.client}
          onSaved={onSaved}
          // A sidecar is only useful with a repo: go straight on to its
          // settings, where repos are added.
          onSidecarSaved={() => {
            refresh()
            setEditing({ kind: 'extensions' })
          }}
          onClose={() => setEditing(null)}
        />
      )}
      {editing?.kind === 'client' && (
        <ClientDialog
          key={editing.client.id}
          client={editing.client}
          isNew={false}
          isDefault={clients[0]?.id === editing.client.id}
          saved={saved}
          live={live(`downloadClient:${editing.client.id}`)}
          onSaved={onSaved}
          onClose={() => setEditing(null)}
        />
      )}
      {editing?.kind === 'extensions' && (
        <SidecarDialog
          key={saved.extensions.url}
          isNew={false}
          saved={saved}
          live={live('extensions')}
          onSaved={onSaved}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  )
}

// ConnectionTile is one connection at a glance: what it is, where it
// points, and whether it answers. Clicking it opens its settings.
function ConnectionTile({
  name,
  detail,
  tag,
  locked,
  status,
  onClick,
}: {
  name: string
  detail: string
  tag?: string
  // Set by the deployment's configuration: it opens to look, not to edit.
  locked?: boolean
  status: ReactNode
  onClick: () => void
}) {
  return (
    <UnstyledButton
      className={classes.tile}
      onClick={onClick}
      aria-label={`${locked ? 'View' : 'Edit'} ${name}`}
    >
      <Group justify="space-between" wrap="nowrap" gap="xs" mb={4}>
        <Text fw={600} truncate>
          {name}
        </Text>
        {tag && (
          <Badge
            size="xs"
            variant="outline"
            color="gray"
            style={{ flexShrink: 0 }}
            leftSection={locked ? <IconLock size={10} /> : undefined}
          >
            {tag}
          </Badge>
        )}
      </Group>
      <Text size="xs" c="dimmed" truncate mb="sm">
        {detail}
      </Text>
      {status}
    </UnstyledButton>
  )
}
