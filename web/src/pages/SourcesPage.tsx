import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type ConnectionSettings, type ConnectionTest } from '../api'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'

// Connection settings for the services Sento orchestrates. Stored
// server-side; the yaml/env config only seeds the very first run.
export default function SourcesPage() {
  usePageTitle('Sources')
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['connections'], queryFn: api.connections })

  if (settings.isPending) return null
  if (settings.error) return <Alert color="red">{String(settings.error)}</Alert>
  return (
    <>
      <SourcesForm
        initial={settings.data}
        onSaved={() => {
          queryClient.invalidateQueries({ queryKey: ['connections'] })
          // The provider set changed: site lists and status are stale.
          queryClient.invalidateQueries({ queryKey: ['indexer-settings'] })
          queryClient.invalidateQueries({ queryKey: ['status'] })
          queryClient.invalidateQueries({ queryKey: ['extensions'] })
        }}
      />
      {settings.data.sourceHost.url !== '' && <ReposCard />}
    </>
  )
}

const testLabels: Record<string, string> = {
  qbittorrent: 'qBittorrent',
  sourceHost: 'Source host',
}

function SourcesForm({
  initial,
  onSaved,
}: {
  initial: ConnectionSettings
  onSaved: () => void
}) {
  const [form, setForm] = useState<ConnectionSettings>(initial)
  const [tests, setTests] = useState<ConnectionTest[] | null>(null)
  const set = (patch: Partial<ConnectionSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setTests(null)
  }

  const save = useMutation({
    mutationFn: () => api.saveConnections(form),
    onSuccess: () => {
      notify.ok('Connections saved and applied — no restart needed')
      onSaved()
    },
    onError: notify.err,
  })
  const test = useMutation({
    mutationFn: () => api.testConnections(form),
    onSuccess: (results) => {
      setTests(results)
      if (results.length === 0) notify.err('Nothing to test — no service has a URL')
    },
    onError: notify.err,
  })

  const dirty = JSON.stringify(form) !== JSON.stringify(initial)

  return (
    <>
      <PageHeader
        title="Sources"
        description="The source host serves extension sources, qBittorrent downloads torrent grabs. Individual indexers (Torznab endpoints and extension sources) are managed on the Indexers page."
      />

      <Stack maw={640}>
        <Card withBorder padding="md">
          <Title order={5} mb={4}>
            Source host
          </Title>
          <Text size="xs" c="dimmed" mb="sm">
            The sento-host sidecar runs Tachiyomi-style extensions that serve chapter downloads;
            Sento fetches the pages itself. Leave the URL empty to disable extension sources.
          </Text>
          <TextInput
            label="URL"
            placeholder="http://localhost:8901"
            value={form.sourceHost.url}
            onChange={(e) => set({ sourceHost: { url: e.currentTarget.value } })}
          />
        </Card>

        <Card withBorder padding="md">
          <Title order={5} mb={4}>
            qBittorrent
          </Title>
          <Text size="xs" c="dimmed" mb="sm">
            Download client for grabbed torrents. Leave the URL empty to disable.
          </Text>
          <Stack gap="xs">
            <TextInput
              label="URL"
              placeholder="http://localhost:8080"
              value={form.qbittorrent.url}
              onChange={(e) =>
                set({ qbittorrent: { ...form.qbittorrent, url: e.currentTarget.value } })
              }
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
              description="Torrents are added under this category and imports only look at it"
              value={form.qbittorrent.category}
              onChange={(e) =>
                set({ qbittorrent: { ...form.qbittorrent, category: e.currentTarget.value } })
              }
            />
          </Stack>
        </Card>

        <Group>
          <Button variant="default" onClick={() => test.mutate()} loading={test.isPending}>
            Test
          </Button>
          <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!dirty}>
            Save
          </Button>
          {dirty && (
            <Button variant="subtle" onClick={() => setForm(initial)}>
              Reset
            </Button>
          )}
          {tests?.map((t) => (
            <Tooltip key={t.service} label={t.error || t.detail || 'reachable'}>
              <Badge color={t.ok ? 'green' : 'red'} variant="light">
                {testLabels[t.service] ?? t.service} {t.ok ? '✓' : '✗'}
              </Badge>
            </Tooltip>
          ))}
        </Group>
      </Stack>
    </>
  )
}

// ReposCard manages the extension repo list — host plumbing. The
// extensions themselves are driven by indexer entries: adding a source
// indexer installs its extension, deleting the last entry uninstalls it.
function ReposCard() {
  const queryClient = useQueryClient()
  const repos = useQuery({ queryKey: ['extension-repos'], queryFn: api.extensionRepos })
  const [newRepo, setNewRepo] = useState('')
  const saveRepos = useMutation({
    mutationFn: (urls: string[]) => api.setExtensionRepos(urls),
    onSuccess: () => {
      setNewRepo('')
      notify.ok('Extension repos updated')
      queryClient.invalidateQueries({ queryKey: ['extension-repos'] })
      queryClient.invalidateQueries({ queryKey: ['extensions'] })
    },
    onError: notify.err,
  })

  return (
    <Card withBorder padding="md" maw={640} mt="md">
      <Title order={5} mb={4}>
        Extension repos
      </Title>
      <Text size="xs" c="dimmed" mb="sm">
        Mihon-compatible repo index URLs — none ship with Sento. Extensions install automatically
        when you add their source as an indexer (Indexers page) and uninstall when the last
        entry is removed.
      </Text>
      <Stack gap={4}>
        {repos.data?.urls.map((u) => (
          <Group key={u} gap={6} wrap="nowrap" justify="space-between">
            <Text size="xs" style={{ fontFamily: 'monospace' }} truncate>
              {u}
            </Text>
            <Button
              size="compact-xs"
              variant="subtle"
              color="red"
              onClick={() => saveRepos.mutate((repos.data?.urls ?? []).filter((x) => x !== u))}
            >
              Remove
            </Button>
          </Group>
        ))}
        <Group gap={6} wrap="nowrap">
          <TextInput
            size="xs"
            style={{ flex: 1 }}
            placeholder="Extension repo URL (…/index.min.json)"
            value={newRepo}
            onChange={(e) => setNewRepo(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && newRepo.trim())
                saveRepos.mutate([...(repos.data?.urls ?? []), newRepo.trim()])
            }}
          />
          <Button
            size="compact-sm"
            variant="default"
            loading={saveRepos.isPending}
            disabled={!newRepo.trim()}
            onClick={() => saveRepos.mutate([...(repos.data?.urls ?? []), newRepo.trim()])}
          >
            Add repo
          </Button>
        </Group>
      </Stack>
    </Card>
  )
}
