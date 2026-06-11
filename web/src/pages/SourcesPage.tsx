import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  PasswordInput,
  SegmentedControl,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type ConnectionSettings, type ConnectionTest } from '../api'
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
    <SourcesForm
      initial={settings.data}
      onSaved={() => {
        queryClient.invalidateQueries({ queryKey: ['connections'] })
        // The provider set changed: site lists and status are stale.
        queryClient.invalidateQueries({ queryKey: ['indexer-settings'] })
        queryClient.invalidateQueries({ queryKey: ['status'] })
      }}
    />
  )
}

const testLabels: Record<string, string> = {
  prowlarr: 'Prowlarr',
  qbittorrent: 'qBittorrent',
  suwayomi: 'Suwayomi',
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
            Extension sources
          </Title>
          <Text size="xs" c="dimmed" mb="sm">
            Tachiyomi-style extensions serve chapter downloads. Either a Suwayomi server or the
            sento-host sidecar provides them; Sento downloads the pages itself.
          </Text>
          <SegmentedControl
            value={form.sourceBackend}
            onChange={(v) => set({ sourceBackend: v as ConnectionSettings['sourceBackend'] })}
            data={[
              { value: 'none', label: 'None' },
              { value: 'suwayomi', label: 'Suwayomi' },
              { value: 'host', label: 'Sento host' },
            ]}
            mb="sm"
          />
          {form.sourceBackend === 'suwayomi' && (
            <Stack gap="xs">
              <TextInput
                label="URL"
                placeholder="http://localhost:4567"
                value={form.suwayomi.url}
                onChange={(e) => set({ suwayomi: { ...form.suwayomi, url: e.currentTarget.value } })}
              />
              <Group grow>
                <TextInput
                  label="Username"
                  value={form.suwayomi.username}
                  onChange={(e) =>
                    set({ suwayomi: { ...form.suwayomi, username: e.currentTarget.value } })
                  }
                />
                <PasswordInput
                  label="Password"
                  value={form.suwayomi.password}
                  onChange={(e) =>
                    set({ suwayomi: { ...form.suwayomi, password: e.currentTarget.value } })
                  }
                />
              </Group>
            </Stack>
          )}
          {form.sourceBackend === 'host' && (
            <TextInput
              label="URL"
              placeholder="http://localhost:8901"
              description="The sento-host sidecar (host/ in the repo) — runs keiyoushi extensions directly"
              value={form.sourceHost.url}
              onChange={(e) => set({ sourceHost: { url: e.currentTarget.value } })}
            />
          )}
          {form.sourceBackend !== 'none' &&
            initial.sourceBackend !== 'none' &&
            form.sourceBackend !== initial.sourceBackend && (
              <Alert color="yellow" p="xs" mt="sm">
                Switching backends re-resolves source bindings: manga identities differ between
                hosts, so the next search re-binds each series automatically.
              </Alert>
            )}
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
