import { useState, type ReactNode } from 'react'
import { Button, Divider, Group, PasswordInput, Select, Stack, TextInput } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { api, type ConnectionSettings, type ConnectionTest } from '../../api'
import type { DownloadClientConn } from '../../types/service'
import { notify } from '../../ui'
import { Repos } from '../extensions/ExtensionRepos'
import { ConnectionDialog } from './ConnectionDialog'

type ClientType = 'qbittorrent' | 'extensions'

const blank: ConnectionSettings = {
  prowlarr: { url: '', apiKey: '' },
  downloadClients: [],
  extensions: { url: '' },
  managed: { extensionsUrl: false },
}

// NewClientDialog adds a download client: pick its type, then fill in that
// type's settings. There is one sidecar, so its type is only offered while
// none is connected.
export function NewClientDialog({
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
export function SidecarDialog({
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
          <RemoveConnectionButton
            saved={saved}
            clear={(c) => ({ ...c, extensions: { url: '' } })}
            message="Extensions sidecar removed"
            onSaved={onSaved}
          />
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

// ClientDialog edits one download client; the rest of the settings are
// saved as they are on the server.
export function ClientDialog({
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
  const mine = (c: ConnectionSettings) =>
    c.downloadClients.find((d) => d.id === client.id) ?? client
  const put = (c: ConnectionSettings, value: DownloadClientConn) => ({
    ...c,
    downloadClients: c.downloadClients.map((d) => (d.id === client.id ? value : d)),
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
          <RemoveConnectionButton
            saved={saved}
            clear={(c) => ({
              ...c,
              downloadClients: c.downloadClients.filter((d) => d.id !== client.id),
            })}
            message={`${client.name} removed`}
            onSaved={onSaved}
          />
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

// RemoveConnectionButton clears one connection's part of the settings and
// reports what was removed; both dialogs share it.
function RemoveConnectionButton({
  saved,
  clear,
  message,
  onSaved,
}: {
  saved: ConnectionSettings
  clear: (c: ConnectionSettings) => ConnectionSettings
  message: string
  onSaved: () => void
}) {
  const remove = useMutation({
    mutationFn: () => api.saveConnections(clear(saved)),
    onSuccess: () => {
      notify.ok(message)
      onSaved()
    },
    onError: notify.err,
  })
  return (
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
