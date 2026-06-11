import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Loader,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type ConnectionSettings, type ConnectionTest, type ExtensionView } from '../api'
import { notify, usePageTitle } from '../ui'

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
      {settings.data.sourceHost.url !== '' && <ExtensionsCard />}
    </>
  )
}

const testLabels: Record<string, string> = {
  prowlarr: 'Prowlarr',
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
      <Title order={2} mb="xs">
        Sources
      </Title>
      <Text size="sm" c="dimmed" mb="md">
        Where releases come from and how they download. Saving applies immediately; the Indexers
        tab reflects the sites of whatever is connected here.
      </Text>

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
            Prowlarr
          </Title>
          <Text size="xs" c="dimmed" mb="sm">
            Indexer aggregator for torrent releases (volumes). Leave the URL empty to disable.
          </Text>
          <Stack gap="xs">
            <TextInput
              label="URL"
              placeholder="http://localhost:9696"
              value={form.prowlarr.url}
              onChange={(e) => set({ prowlarr: { ...form.prowlarr, url: e.currentTarget.value } })}
            />
            <PasswordInput
              label="API key"
              value={form.prowlarr.apiKey}
              onChange={(e) => set({ prowlarr: { ...form.prowlarr, apiKey: e.currentTarget.value } })}
            />
          </Stack>
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

// ExtensionsCard manages the sidecar's extensions. Sento stores the
// desired list and reconciles a fresh host back to it — the host itself
// is stateless.
function ExtensionsCard() {
  const queryClient = useQueryClient()
  const [filter, setFilter] = useState('')
  const extensions = useQuery({ queryKey: ['extensions'], queryFn: api.extensions })
  const [busyPkg, setBusyPkg] = useState<string | null>(null)

  const act = (pkg: string, fn: () => Promise<void>, msg: string) => {
    setBusyPkg(pkg)
    fn()
      .then(() => {
        notify.ok(msg)
        queryClient.invalidateQueries({ queryKey: ['extensions'] })
        queryClient.invalidateQueries({ queryKey: ['indexer-settings'] })
      })
      .catch(notify.err)
      .finally(() => setBusyPkg(null))
  }

  if (extensions.isPending)
    return (
      <Card withBorder padding="md" maw={640} mt="md">
        <Group gap="xs">
          <Loader size="xs" />
          <Text size="sm" c="dimmed">
            Loading extensions from the host…
          </Text>
        </Group>
      </Card>
    )
  if (extensions.error)
    return (
      <Alert color="orange" maw={640} mt="md">
        Extensions unavailable: {String(extensions.error)}
      </Alert>
    )

  const all = extensions.data
  const installed = all.filter((e) => e.installedVersion)
  const q = filter.trim().toLowerCase()
  const matches = q
    ? all.filter(
        (e) => !e.installedVersion && (e.name.toLowerCase().includes(q) || e.pkgName.includes(q)),
      )
    : []

  const row = (e: ExtensionView) => (
    <Group key={e.pkgName} justify="space-between" wrap="nowrap" gap="xs">
      <Group gap={6} wrap="nowrap" style={{ minWidth: 0 }}>
        <Text size="sm" truncate>
          {e.name}
        </Text>
        <Badge size="xs" variant="outline" color="gray">
          {e.lang}
        </Badge>
        {e.nsfw && (
          <Badge size="xs" variant="outline" color="pink">
            nsfw
          </Badge>
        )}
        <Text size="xs" c="dimmed">
          {e.installedVersion ?? e.availableVersion}
        </Text>
      </Group>
      <Group gap={4} wrap="nowrap">
        {e.hasUpdate && (
          <Button
            size="compact-xs"
            variant="light"
            color="yellow"
            loading={busyPkg === e.pkgName}
            onClick={() =>
              act(e.pkgName, () => api.installExtension(e.pkgName), `${e.name} updated`)
            }
          >
            Update {e.availableVersion}
          </Button>
        )}
        {e.installedVersion ? (
          <Button
            size="compact-xs"
            variant="subtle"
            color="red"
            loading={busyPkg === e.pkgName}
            onClick={() =>
              act(e.pkgName, () => api.uninstallExtension(e.pkgName), `${e.name} uninstalled`)
            }
          >
            Uninstall
          </Button>
        ) : (
          <Button
            size="compact-xs"
            loading={busyPkg === e.pkgName}
            onClick={() =>
              act(e.pkgName, () => api.installExtension(e.pkgName), `${e.name} installed`)
            }
          >
            Install
          </Button>
        )}
      </Group>
    </Group>
  )

  return (
    <Card withBorder padding="md" maw={640} mt="md">
      <Title order={5} mb={4}>
        Extensions
      </Title>
      <Text size="xs" c="dimmed" mb="sm">
        Installed extensions provide the sources on the Indexers tab. Sento remembers this list
        and re-installs it automatically if the host is replaced.
      </Text>
      <Stack gap={6}>
        {installed.length === 0 && (
          <Text size="sm" c="dimmed">
            No extensions installed yet — search below to add sources.
          </Text>
        )}
        {installed.map(row)}
        <TextInput
          size="xs"
          mt={installed.length > 0 ? 'sm' : 0}
          placeholder={`Search ${all.length} available extensions…`}
          leftSection={<IconSearch size={12} />}
          value={filter}
          onChange={(e) => setFilter(e.currentTarget.value)}
        />
        {q && matches.length === 0 && (
          <Text size="xs" c="dimmed">
            Nothing matches “{filter}”.
          </Text>
        )}
        {matches.slice(0, 20).map(row)}
        {matches.length > 20 && (
          <Text size="xs" c="dimmed">
            …{matches.length - 20} more — refine the search.
          </Text>
        )}
      </Stack>
    </Card>
  )
}
