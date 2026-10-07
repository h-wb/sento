import { useState, type ReactNode } from 'react'
import {
  Alert,
  Badge,
  Button,
  Divider,
  Group,
  List,
  PasswordInput,
  Select,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import { IconLock, IconPlus } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type ConnectionSettings, type ConnectionTest } from '../api'
import type { RepoStatus } from '../types/extensions'
import type { DownloadClientConn } from '../types/service'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EditorModal } from '../components/EditionDialogs'
import classes from './ConnectionsPage.module.css'

// The download clients Sento fetches through. They come in two types:
// qBittorrent instances for torrents (any number), and the extensions
// sidecar for chapters from extension sources (one). Each is a card showing
// what it is and whether it answers right now; its settings open in a
// dialog, the sidecar's extension repos included. What gets searched lives
// on the Indexers page; this page is only the plumbing. Stored server-side;
// the yaml/env config only seeds the very first run, so a client given
// there is already on the page.
type ClientType = 'qbittorrent' | 'extensions'
type Editing =
  | { kind: 'new'; client: DownloadClientConn }
  | { kind: 'client'; client: DownloadClientConn }
  | { kind: 'extensions' }

export default function ConnectionsPage() {
  usePageTitle('Connections')
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Editing | null>(null)
  const settings = useQuery({ queryKey: ['connections'], queryFn: api.connections })
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
    queryClient.invalidateQueries({ queryKey: ['indexer-settings'] })
    queryClient.invalidateQueries({ queryKey: ['status'] })
    queryClient.invalidateQueries({ queryKey: ['health'] })
    queryClient.invalidateQueries({ queryKey: ['extensions'] })
    queryClient.invalidateQueries({ queryKey: ['extension-repos'] })
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

// NewClientDialog adds a download client: pick its type, then fill in that
// type's settings. There is one sidecar, so its type is only offered while
// none is connected.
function NewClientDialog({
  saved,
  client,
  onSaved,
  onSidecarSaved,
  onClose,
}: {
  saved: ConnectionSettings
  client: DownloadClientConn
  onSaved: () => void
  onSidecarSaved: () => void
  onClose: () => void
}) {
  const hasSidecar = saved.extensions.url !== ''
  const [type, setType] = useState<ClientType>('qbittorrent')
  const typeSelect = (
    <Select
      label="Type"
      allowDeselect={false}
      value={type}
      onChange={(v) => v && setType(v as ClientType)}
      data={[
        { value: 'qbittorrent', label: 'qBittorrent — torrents' },
        {
          value: 'extensions',
          label: hasSidecar
            ? 'Extensions sidecar — already connected'
            : 'Extensions sidecar — extension sources',
          disabled: hasSidecar,
        },
      ]}
    />
  )
  return type === 'qbittorrent' ? (
    <ClientDialog
      key="qbittorrent"
      client={client}
      isNew
      isDefault={saved.downloadClients.length === 0}
      saved={saved}
      typeSelect={typeSelect}
      onSaved={onSaved}
      onClose={onClose}
    />
  ) : (
    <SidecarDialog
      key="extensions"
      isNew
      saved={saved}
      typeSelect={typeSelect}
      onSaved={onSidecarSaved}
      onClose={onClose}
    />
  )
}

// SidecarDialog is the extensions sidecar's settings: where it runs and,
// once connected, the extension repos it loads.
function SidecarDialog({
  isNew,
  saved,
  live,
  typeSelect,
  onSaved,
  onClose,
}: {
  isNew: boolean
  saved: ConnectionSettings
  live?: ConnectionTest
  typeSelect?: ReactNode
  onSaved: () => void
  onClose: () => void
}) {
  // Given by the deployment's configuration: shown, not editable.
  const managed = saved.managed.extensionsUrl
  const remove = useMutation({
    mutationFn: () => api.saveConnections({ ...saved, extensions: { url: '' } }),
    onSuccess: () => {
      notify.ok('Extensions sidecar removed')
      onSaved()
    },
    onError: notify.err,
  })
  return (
    <ConnectionDialog
      title={isNew ? 'Add download client' : 'Extensions sidecar'}
      service="extensions"
      saved={saved}
      live={live}
      onSaved={onSaved}
      onClose={onClose}
      alwaysSavable={isNew}
      readOnly={managed}
      blurb="Runs Mihon-compatible extensions so Sento can find and download chapters from their sources."
      isSet={(c) => c.extensions.url.trim() !== ''}
      same={(a, b) => a.extensions.url === b.extensions.url}
      only={(c) => ({ ...blank, extensions: c.extensions })}
      merge={(base, form) => ({ ...base, extensions: { url: form.extensions.url.trim() } })}
      extra={
        !isNew &&
        !managed && (
          <Button
            variant="subtle"
            color="red"
            size="xs"
            loading={remove.isPending}
            onClick={() => remove.mutate()}
          >
            Remove
          </Button>
        )
      }
      fields={(form, set) => (
        <Stack gap="xs">
          {typeSelect}
          <TextInput
            label="URL"
            placeholder="http://localhost:8901"
            disabled={managed}
            value={form.extensions.url}
            onChange={(e) => set({ extensions: { url: e.currentTarget.value } })}
          />
        </Stack>
      )}
      below={
        !isNew && (
          <section aria-label="Extension repos">
            <Divider mb="sm" />
            <Repos />
          </section>
        )
      }
    />
  )
}

const blank: ConnectionSettings = {
  prowlarr: { url: '', apiKey: '' },
  downloadClients: [],
  extensions: { url: '' },
  managed: { extensionsUrl: false },
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

// ClientDialog edits one download client; the rest of the settings are
// saved as they are on the server.
function ClientDialog({
  client,
  isNew,
  isDefault,
  saved,
  live,
  typeSelect,
  onSaved,
  onClose,
}: {
  client: DownloadClientConn
  isNew: boolean
  isDefault: boolean
  saved: ConnectionSettings
  live?: ConnectionTest
  typeSelect?: ReactNode
  onSaved: () => void
  onClose: () => void
}) {
  // The dialog's "saved" view includes the client being added, so the
  // form starts from it.
  const base: ConnectionSettings = isNew
    ? { ...saved, downloadClients: [...saved.downloadClients, client] }
    : saved
  const mine = (c: ConnectionSettings) => c.downloadClients.find((d) => d.id === client.id) ?? client
  const put = (c: ConnectionSettings, value: DownloadClientConn) => ({
    ...c,
    downloadClients: c.downloadClients.map((d) => (d.id === client.id ? value : d)),
  })
  const remove = useMutation({
    mutationFn: () =>
      api.saveConnections({
        ...saved,
        downloadClients: saved.downloadClients.filter((d) => d.id !== client.id),
      }),
    onSuccess: () => {
      notify.ok(`${client.name} removed`)
      onSaved()
    },
    onError: notify.err,
  })
  return (
    <ConnectionDialog
      title={isNew ? 'Add download client' : client.name}
      service={`downloadClient:${client.id}`}
      saved={base}
      live={live}
      onSaved={onSaved}
      onClose={onClose}
      alwaysSavable={isNew}
      isSet={(c) => mine(c).url.trim() !== ''}
      same={(a, b) => JSON.stringify(mine(a)) === JSON.stringify(mine(b))}
      only={(c) => ({ ...blank, downloadClients: [mine(c)] })}
      merge={(b, form) => put(b, mine(form))}
      blurb={
        isDefault
          ? 'The default client: indexers use it unless they name another.'
          : 'Choose this client per indexer on the Indexers page, e.g. for trackers that must download through a VPN.'
      }
      extra={
        !isNew && (
          <Button
            variant="subtle"
            color="red"
            size="xs"
            loading={remove.isPending}
            onClick={() => remove.mutate()}
          >
            Remove
          </Button>
        )
      }
      fields={(form, set) => {
        const value = mine(form)
        const edit = (patch: Partial<DownloadClientConn>) =>
          set({ downloadClients: put(form, { ...value, ...patch }).downloadClients })
        return (
          <Stack gap="xs">
            {typeSelect}
            <TextInput
              label="Name"
              placeholder="qBittorrent"
              value={value.name}
              onChange={(e) => edit({ name: e.currentTarget.value })}
            />
            <TextInput
              label="URL"
              placeholder="http://localhost:8080"
              value={value.url}
              onChange={(e) => edit({ url: e.currentTarget.value })}
            />
            <Group grow>
              <TextInput
                label="Username"
                value={value.username}
                onChange={(e) => edit({ username: e.currentTarget.value })}
              />
              <PasswordInput
                label="Password"
                value={value.password}
                onChange={(e) => edit({ password: e.currentTarget.value })}
              />
            </Group>
            <TextInput
              label="Category"
              description="Sento only imports torrents in this category"
              value={value.category}
              onChange={(e) => edit({ category: e.currentTarget.value })}
            />
          </Stack>
        )
      }}
    />
  )
}

// StatusBadge renders one probe result. `draft` marks a test of unsaved
// values, so it can't be mistaken for the state of what is running.
function StatusBadge({
  result,
  configured,
  probing,
  draft,
}: {
  result?: ConnectionTest
  configured: boolean
  probing?: boolean
  draft?: boolean
}) {
  if (!configured) {
    return (
      <Badge color="gray" variant="light">
        Not configured
      </Badge>
    )
  }
  if (!result) {
    return (
      <Badge color="gray" variant="light">
        {probing ? 'Checking…' : 'Unknown'}
      </Badge>
    )
  }
  const label = result.ok
    ? `${draft ? 'Reachable' : 'Connected'}${result.detail ? ` · ${result.detail}` : ''}`
    : 'Unreachable'
  return (
    <Tooltip label={result.error || 'The service answered'} multiline maw={360}>
      <Badge color={result.ok ? 'green' : 'red'} variant="light" style={{ textTransform: 'none' }}>
        {label}
      </Badge>
    </Tooltip>
  )
}

// ConnectionDialog is one connection's settings: its form, a Test of the
// values as typed, and Save. Saving writes only this connection; the rest
// stays as it is on the server.
function ConnectionDialog({
  title,
  blurb,
  service,
  saved,
  live,
  onSaved,
  onClose,
  alwaysSavable = false,
  readOnly = false,
  isSet,
  same,
  only,
  merge,
  fields,
  extra,
  below,
}: {
  title: string
  blurb?: string
  service: string
  saved: ConnectionSettings
  live?: ConnectionTest
  onSaved: () => void
  onClose: () => void
  // A connection being added has nothing saved to differ from.
  alwaysSavable?: boolean
  // Fixed by the deployment's configuration: the values are shown and can
  // be tested, but there is nothing to save.
  readOnly?: boolean
  isSet: (c: ConnectionSettings) => boolean
  same: (a: ConnectionSettings, b: ConnectionSettings) => boolean
  only: (c: ConnectionSettings) => ConnectionSettings
  merge: (base: ConnectionSettings, form: ConnectionSettings) => ConnectionSettings
  fields: (form: ConnectionSettings, set: (patch: Partial<ConnectionSettings>) => void) => ReactNode
  extra?: ReactNode
  // Shown under the buttons: things that act on the saved connection
  // right away, apart from the form.
  below?: ReactNode
}) {
  const [form, setForm] = useState<ConnectionSettings>(saved)
  const [draftTest, setDraftTest] = useState<ConnectionTest | null>(null)
  const set = (patch: Partial<ConnectionSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setDraftTest(null)
  }
  const dirty = !same(form, saved)

  const save = useMutation({
    mutationFn: () => api.saveConnections(merge(saved, form)),
    onSuccess: () => {
      notify.ok(`${title} saved`)
      onSaved()
    },
    onError: notify.err,
  })
  const test = useMutation({
    mutationFn: () => api.testConnections(only(form)),
    onSuccess: (results) => {
      const mine = results.find((t) => t.service === service)
      if (mine) setDraftTest(mine)
      else notify.err('Nothing to test — enter a URL first')
    },
    onError: notify.err,
  })
  const busy = save.isPending || test.isPending

  return (
    <EditorModal title={title} dirty={dirty} busy={busy} onClose={onClose}>
      <Stack gap="sm">
        {blurb && (
          <Text size="sm" c="dimmed">
            {blurb}
          </Text>
        )}
        {readOnly && (
          <Alert color="gray" py="xs" icon={<IconLock size={16} />} title="Read-only">
            <Text size="xs">
              Its address is set by the deployment&apos;s configuration (
              <code>SENTO_EXTENSIONS_URL</code>). Change it there and restart Sento.
            </Text>
          </Alert>
        )}
        {live && !live.ok && (
          <Alert color="red" py="xs" title="Not reachable right now">
            <Text size="xs" style={{ overflowWrap: 'anywhere' }}>
              {live.error}
            </Text>
          </Alert>
        )}
        <fieldset disabled={busy} className="formFields">
          {fields(form, set)}
        </fieldset>
        <Group justify="space-between" mt="xs">
          <Group gap="xs">
            <Button
              variant="default"
              size="xs"
              onClick={() => test.mutate()}
              loading={test.isPending}
              disabled={!isSet(form) || save.isPending}
            >
              Test
            </Button>
            {draftTest && <StatusBadge result={draftTest} configured draft />}
          </Group>
          <Group gap="xs">
            {extra}
            <Button variant="default" size="xs" onClick={onClose} disabled={busy}>
              {readOnly ? 'Close' : 'Cancel'}
            </Button>
            {!readOnly && (
              <Button
                size="xs"
                onClick={() => save.mutate()}
                loading={save.isPending}
                disabled={test.isPending || (alwaysSavable ? !isSet(form) : !dirty)}
              >
                Save
              </Button>
            )}
          </Group>
        </Group>
        {below}
      </Stack>
    </EditorModal>
  )
}

// Repos lists the extension repos with what each one currently yields on
// the host: its name and usable extension count, or why it yields nothing.
// Extensions themselves are driven by indexer entries — adding a source
// installs its extension, deleting the last entry uninstalls it.
function Repos() {
  const queryClient = useQueryClient()
  const repos = useQuery({ queryKey: ['extension-repos'], queryFn: api.extensionRepos })
  const [newRepo, setNewRepo] = useState('')
  const saveRepos = useMutation({
    mutationFn: (urls: string[]) => api.setExtensionRepos(urls),
    onSuccess: (data) => {
      setNewRepo('')
      notify.ok('Extension repos updated')
      queryClient.setQueryData(['extension-repos'], data)
      queryClient.invalidateQueries({ queryKey: ['extensions'] })
    },
    onError: notify.err,
  })
  const urls = repos.data?.urls ?? []
  // Given by the deployment's configuration: shown with their status,
  // not editable.
  const managed = repos.data?.managed ?? false
  const rows: RepoStatus[] = repos.data?.repos ?? urls.map((url) => ({ url, extensions: 0 }))
  const add = () => { if (newRepo.trim() && !saveRepos.isPending && !urls.includes(newRepo.trim())) saveRepos.mutate([...urls, newRepo.trim()]) }

  return (
    <>
      <Group gap="xs" mb={4}>
        <Title order={6}>Extension repos</Title>
        {managed && (
          <Badge size="xs" variant="outline" color="gray" leftSection={<IconLock size={10} />}>
            Read-only
          </Badge>
        )}
      </Group>
      <Text size="xs" c="dimmed" mb="sm">
        {managed ? (
          <>
            This list is set by the deployment&apos;s configuration (
            <code>SENTO_EXTENSIONS_REPOS</code>). Change it there and restart Sento. Choose
          </>
        ) : (
          'Add a Mihon-compatible repository, then choose'
        )}{' '}
        its sources on <Link to="/indexers">Indexers</Link>.
      </Text>
      <QueryFeedback pending={repos.isPending} error={repos.error} onRetry={() => void repos.refetch()} />
      <Stack gap="xs">
        {repos.data && rows.length === 0 && (
          <Text size="sm" c="dimmed">
            Add a repository to see available sources.
          </Text>
        )}
        {rows.map((repo) => (
          <RepoRow
            key={repo.url}
            repo={repo}
            removing={saveRepos.isPending}
            onRemove={
              managed ? undefined : () => saveRepos.mutate(urls.filter((x) => x !== repo.url))
            }
          />
        ))}
        {!managed && <Group gap={6} wrap="nowrap">
          <TextInput
            size="xs"
            style={{ flex: 1 }}
            aria-label="Extension repo URL"
            placeholder="https://…/index.min.json"
            value={newRepo}
            onChange={(e) => setNewRepo(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && newRepo.trim()) add()
            }}
          />
          <Button
            size="xs"
            variant="default"
            loading={saveRepos.isPending}
            disabled={!newRepo.trim() || urls.includes(newRepo.trim())}
            onClick={add}
          >
            Add repo
          </Button>
        </Group>}
      </Stack>
    </>
  )
}

function RepoRow({
  repo,
  removing,
  onRemove,
}: {
  repo: RepoStatus
  removing: boolean
  onRemove?: () => void
}) {
  return (
    <Group gap="sm" wrap="nowrap" justify="space-between" align="flex-start">
      <Stack gap={0} style={{ minWidth: 0, flex: 1 }}>
        <Group gap={8} wrap="nowrap">
          <Text size="sm" fw={600} truncate>
            {repo.name || hostOf(repo.url)}
          </Text>
          {repo.error ? (
            <Badge color="red" variant="light" style={{ flexShrink: 0 }}>
              Failing
            </Badge>
          ) : (
            <Badge
              color={repo.extensions > 0 ? 'green' : 'yellow'}
              variant="light"
              style={{ flexShrink: 0, textTransform: 'none' }}
            >
              {repo.extensions.toLocaleString()} {repo.extensions === 1 ? 'extension' : 'extensions'}
            </Badge>
          )}
        </Group>
        <Text size="xs" c="dimmed" style={{ fontFamily: 'monospace' }} truncate>
          {repo.url}
        </Text>
        {repo.error && (
          <Text size="xs" c="red">
            {repo.error}
          </Text>
        )}
        {!repo.error && repo.extensions === 0 && (
          <Text size="xs" c="dimmed">
            The repo answered but lists nothing this host can run.
          </Text>
        )}
      </Stack>
      {onRemove && (
        <Button size="compact-xs" variant="subtle" color="red" disabled={removing} onClick={onRemove}>
          Remove
        </Button>
      )}
    </Group>
  )
}

function hostOf(url: string) {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}
