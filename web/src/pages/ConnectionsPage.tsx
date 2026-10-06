import { useState, type ReactNode } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Divider,
  Group,
  List,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type ConnectionSettings, type ConnectionTest } from '../api'
import type { RepoStatus } from '../types/sourcehost'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { useUnsavedChanges } from '../useUnsavedChanges'

// The services Sento connects to — one per download path: qBittorrent for
// torrents, the sento-host sidecar for extension sources. What gets
// searched lives on the Indexers page; this page is only the plumbing.
// Stored server-side; the yaml/env config only seeds the very first run.
export default function ConnectionsPage() {
  usePageTitle('Connections')
  const queryClient = useQueryClient()
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

  const onSaved = () => {
    queryClient.invalidateQueries({ queryKey: ['connections'] })
    // The provider set changed: site lists and status are stale.
    queryClient.invalidateQueries({ queryKey: ['indexer-settings'] })
    queryClient.invalidateQueries({ queryKey: ['status'] })
    queryClient.invalidateQueries({ queryKey: ['health'] })
    queryClient.invalidateQueries({ queryKey: ['extensions'] })
    queryClient.invalidateQueries({ queryKey: ['extension-repos'] })
  }
  const nothingSet = saved.qbittorrent.url === '' && saved.sourceHost.url === ''

  return (
    <>
      <PageHeader
        title="Connections"
        description="Download services and extension repositories."
      />

      <Stack maw={680}>
        {nothingSet && (
          <Alert color="blue" title="Nothing is connected yet">
            <List size="sm" type="ordered" spacing={2}>
              <List.Item>
                Connect qBittorrent for torrents or the source host for extension sources.
              </List.Item>
              <List.Item>For extension sources, add a repository below.</List.Item>
              <List.Item>
                Choose what to search on the <Link to="/indexers">Indexers</Link> page.
              </List.Item>
            </List>
          </Alert>
        )}

        <ServiceCard
          section="Torrents"
          title="qBittorrent"
          blurb="Downloads torrent releases. Sento must have access to its download folder to import files."
          service="qbittorrent"
          saved={saved}
          live={live('qbittorrent')}
          probing={status.isFetching}
          onSaved={onSaved}
          isSet={(c) => c.qbittorrent.url !== ''}
          same={(a, b) => JSON.stringify(a.qbittorrent) === JSON.stringify(b.qbittorrent)}
          only={(c) => ({ ...blank, qbittorrent: c.qbittorrent })}
          merge={(base, form) => ({ ...base, qbittorrent: form.qbittorrent })}
          fields={(form, set) => (
            <Stack gap="xs">
              <TextInput
                label="URL"
                placeholder="http://localhost:8080"
                description="Leave empty to disable torrent downloads"
                value={form.qbittorrent.url}
                onChange={(e) => set({ qbittorrent: { ...form.qbittorrent, url: e.currentTarget.value } })}
              />
              <Group grow>
                <TextInput
                  label="Username"
                  value={form.qbittorrent.username}
                  onChange={(e) =>
                    set({ qbittorrent: { ...form.qbittorrent, username: e.currentTarget.value } })
                  }
                />
                <PasswordInput
                  label="Password"
                  value={form.qbittorrent.password}
                  onChange={(e) =>
                    set({ qbittorrent: { ...form.qbittorrent, password: e.currentTarget.value } })
                  }
                />
              </Group>
              <TextInput
                label="Category"
                description="Sento only imports torrents in this category"
                value={form.qbittorrent.category}
                onChange={(e) =>
                  set({ qbittorrent: { ...form.qbittorrent, category: e.currentTarget.value } })
                }
              />
            </Stack>
          )}
        />

        <ServiceCard
          section="Extension sources"
          title="Source host"
          blurb="Runs Mihon-compatible extensions so Sento can find and download chapters from their sources."
          service="sourceHost"
          saved={saved}
          live={live('sourceHost')}
          probing={status.isFetching}
          onSaved={onSaved}
          isSet={(c) => c.sourceHost.url !== ''}
          same={(a, b) => a.sourceHost.url === b.sourceHost.url}
          only={(c) => ({ ...blank, sourceHost: c.sourceHost })}
          merge={(base, form) => ({ ...base, sourceHost: form.sourceHost })}
          fields={(form, set) => (
            <TextInput
              label="URL"
              placeholder="http://localhost:8901"
              description="Leave empty to disable extension sources"
              value={form.sourceHost.url}
              onChange={(e) => set({ sourceHost: { url: e.currentTarget.value } })}
            />
          )}
        >
          {saved.sourceHost.url !== '' && (
            <>
              <Divider my="md" />
              <Repos />
            </>
          )}
        </ServiceCard>
      </Stack>
    </>
  )
}

const blank: ConnectionSettings = {
  prowlarr: { url: '', apiKey: '' },
  qbittorrent: { url: '', username: '', password: '', category: '' },
  sourceHost: { url: '' },
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

// ServiceCard is one connection: its live status, its own form, and its
// own Test and Save — editing one service never touches the other.
function ServiceCard({
  section,
  title,
  blurb,
  service,
  saved,
  live,
  probing,
  onSaved,
  isSet,
  same,
  only,
  merge,
  fields,
  children,
}: {
  section: string
  title: string
  blurb: string
  service: string
  saved: ConnectionSettings
  live?: ConnectionTest
  probing: boolean
  onSaved: () => void
  isSet: (c: ConnectionSettings) => boolean
  same: (a: ConnectionSettings, b: ConnectionSettings) => boolean
  only: (c: ConnectionSettings) => ConnectionSettings
  merge: (base: ConnectionSettings, form: ConnectionSettings) => ConnectionSettings
  fields: (form: ConnectionSettings, set: (patch: Partial<ConnectionSettings>) => void) => ReactNode
  children?: ReactNode
}) {
  const [form, setForm] = useState<ConnectionSettings>(saved)
  const [draftTest, setDraftTest] = useState<ConnectionTest | null>(null)
  const set = (patch: Partial<ConnectionSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setDraftTest(null)
  }
  const dirty = !same(form, saved)
  useUnsavedChanges(dirty)

  const save = useMutation({
    // Only this service's fields are written; the rest stays as saved.
    mutationFn: () => api.saveConnections(merge(saved, form)),
    onSuccess: () => {
      notify.ok(`${title} saved`)
      setDraftTest(null)
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

  return (
    <Card withBorder padding="md" aria-label={title} component="section">
      <Text size="xs" c="dimmed" tt="uppercase" fw={600} mb={4}>
        {section}
      </Text>
      <Group justify="space-between" align="center" mb={4} wrap="wrap" gap="xs">
        <Title order={5}>{title}</Title>
        <StatusBadge result={live} configured={isSet(saved)} probing={probing} />
      </Group>
      <Text size="xs" c="dimmed" mb="sm">
        {blurb}
      </Text>
      {live && !live.ok && isSet(saved) && (
        <Alert color="red" mb="sm" py="xs">
          <Text size="xs">{live.error}</Text>
        </Alert>
      )}

      <fieldset disabled={save.isPending || test.isPending} className="formFields">{fields(form, set)}</fieldset>

      <Group mt="md" gap="xs">
        <Button
          variant="default"
          size="xs"
          onClick={() => test.mutate()}
          loading={test.isPending}
          disabled={!isSet(form) || save.isPending}
        >
          Test
        </Button>
        <Button size="xs" onClick={() => save.mutate()} loading={save.isPending} disabled={!dirty || test.isPending}>
          Save
        </Button>
        {dirty && (
          <Button
            variant="subtle"
            size="xs"
            disabled={save.isPending || test.isPending}
            onClick={() => {
              setForm(saved)
              setDraftTest(null)
            }}
          >
            Reset
          </Button>
        )}
        {dirty && <Text size="xs" c="dimmed">Unsaved changes</Text>}
        {draftTest && <StatusBadge result={draftTest} configured draft />}
      </Group>
      {children}
    </Card>
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
  const rows: RepoStatus[] = repos.data?.repos ?? urls.map((url) => ({ url, extensions: 0 }))
  const add = () => { if (newRepo.trim() && !saveRepos.isPending && !urls.includes(newRepo.trim())) saveRepos.mutate([...urls, newRepo.trim()]) }

  return (
    <>
      <Title order={6} mb={4}>
        Extension repos
      </Title>
      <Text size="xs" c="dimmed" mb="sm">
        Add a Mihon-compatible repository, then choose its sources on <Link to="/indexers">Indexers</Link>.
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
            onRemove={() => saveRepos.mutate(urls.filter((x) => x !== repo.url))}
          />
        ))}
        <Group gap={6} wrap="nowrap">
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
        </Group>
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
  onRemove: () => void
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
      <Button size="compact-xs" variant="subtle" color="red" disabled={removing} onClick={onRemove}>
        Remove
      </Button>
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
