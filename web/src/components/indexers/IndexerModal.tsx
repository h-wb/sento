import { useRef, useState } from 'react'
import {
  Button,
  Group,
  Modal,
  PasswordInput,
  SegmentedControl,
  Select,
  Stack,
  Text,
  TextInput,
} from '@mantine/core'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api, type IndexerBody, type IndexerView } from '../../api'
import { connectionsQuery, extensionsQuery } from '../../queries'
import { notify } from '../../ui'
import { ExtensionSourcePicker } from './ExtensionSourcePicker'
import { ProwlarrBrowse } from './ProwlarrBrowse'

type TorznabForm = { url: string; apiKey: string; language?: string; downloadClient?: string }
export type SourceForm = { sourceId: string; pkgName?: string }

export function IndexerModal({
  initial,
  asCopy,
  onClose,
  onSaved,
}: {
  initial: IndexerView | null
  asCopy: boolean
  onClose: () => void
  onSaved: () => void
}) {
  const isEdit = !!initial && !asCopy
  const init = initial?.settings as Partial<TorznabForm & SourceForm> | undefined
  const [protocol, setProtocol] = useState<'torznab' | 'source'>(initial?.protocol ?? 'torznab')
  const [name, setName] = useState(asCopy ? `${initial?.name} (copy)` : (initial?.name ?? ''))
  const [torznab, setTorznab] = useState<TorznabForm>({
    url: init?.url ?? '',
    apiKey: init?.apiKey ?? '',
    language: init?.language ?? '',
    downloadClient: init?.downloadClient ?? '',
  })
  const connections = useQuery(connectionsQuery)
  const downloadClients = connections.data?.downloadClients ?? []
  const [source, setSource] = useState<SourceForm>({
    sourceId: init?.sourceId ?? '',
    pkgName: init?.pkgName,
  })
  // The source's own settings (login, mirrors…) edit in the same modal:
  // an overlay of changed values, pushed after the entry saves. Keyed by
  // the host source, so entries of the same source share them.
  const [prefValues, setPrefValues] = useState<Record<string, unknown>>({})

  // Extension catalog: the source flow starts from the extension. Its
  // sources come with the catalog, so nothing is installed until the
  // indexer is actually added.
  const extensions = useQuery({
    ...extensionsQuery,
    enabled: protocol === 'source' && !isEdit,
  })
  const [pkg, setPkg] = useState(init?.pkgName ?? '')
  const pickedExt = (extensions.data ?? []).find((e) => e.pkgName === pkg)

  // Auto-name follows the picked source as long as the user hasn't typed
  // a name themselves: re-picking overwrites our suggestion, never theirs.
  const autoName = useRef('')
  const suggestName = (n: string) => {
    if (!name || name === autoName.current) {
      setName(n)
      autoName.current = n
    }
  }

  // The source's own settings can only be read from an installed
  // extension. When adding installs it, the modal stays open on those
  // settings instead of closing (`configuring`).
  const [installedNow, setInstalledNow] = useState(false)
  const [configuring, setConfiguring] = useState(false)
  const extInstalled = isEdit || installedNow || !!pickedExt?.installedVersion

  // Survives a retry after the entry saved but a pref push failed, so
  // retrying doesn't create a duplicate entry.
  const createdId = useRef<number | null>(null)
  const save = useMutation({
    mutationFn: async () => {
      const body: IndexerBody = {
        name,
        protocol,
        enabled: initial?.enabled ?? true,
        useVolumes: initial?.useVolumes ?? true,
        useChapters: initial?.useChapters ?? true,
        priority: initial?.priority ?? 0,
        settings: protocol === 'torznab' ? torznab : source,
      }
      const installs = protocol === 'source' && !extInstalled
      if (installs) {
        await api.installExtension(pkg)
        setInstalledNow(true)
      }
      if (isEdit) {
        await api.updateIndexer(initial.id, body)
      } else if (createdId.current === null) {
        createdId.current = (await api.addIndexer(body)).id
      }
      for (const [key, value] of Object.entries(prefValues)) {
        await api.setSourcePreference(source.sourceId, key, value)
      }
      // Freshly installed: its settings were not visible until now. Stay
      // open on them if it has any (a login is often required to search).
      if (installs) {
        const prefs = await api.sourcePreferences(source.sourceId).catch(() => [])
        return prefs.length > 0
      }
      return false
    },
    onSuccess: (hasSettings) => {
      if (hasSettings) {
        notify.ok('Indexer added — this source has settings of its own')
        setConfiguring(true)
        return
      }
      notify.ok(isEdit ? 'Indexer updated' : configuring ? 'Settings saved' : 'Indexer added')
      onSaved()
    },
    onError: notify.err,
  })

  return (
    <Modal
      opened
      // Once the entry exists, closing by any route has to refresh the list.
      onClose={configuring ? onSaved : onClose}
      title={isEdit ? `Edit ${initial.name}` : configuring ? `${name} — settings` : 'Add indexer'}
      size="lg"
      closeOnClickOutside={!save.isPending}
      closeOnEscape={!save.isPending}
      withCloseButton={!save.isPending}
    >
      <fieldset className="formFields" disabled={save.isPending}>
        <Stack gap="sm">
          {!isEdit && (
            <SegmentedControl
              value={protocol}
              onChange={(v) => setProtocol(v as 'torznab' | 'source')}
              data={[
                { value: 'torznab', label: 'Torrent indexer' },
                { value: 'source', label: 'Extension source' },
              ]}
            />
          )}
          <TextInput
            label="Name"
            description="Shown in results, history and the queue"
            value={name}
            onChange={(e) => setName(e.currentTarget.value)}
            data-autofocus
          />

          {protocol === 'torznab' && (
            <>
              <TextInput
                label="Torznab URL"
                placeholder="http://prowlarr:9696/1/api"
                description="Use the Torznab URL provided by Prowlarr or Jackett"
                value={torznab.url}
                onChange={(e) => setTorznab({ ...torznab, url: e.currentTarget.value })}
              />
              <PasswordInput
                label="API key"
                value={torznab.apiKey}
                onChange={(e) => setTorznab({ ...torznab, apiKey: e.currentTarget.value })}
              />
              <TextInput
                label="Default language"
                placeholder="Mixed"
                description="Language of this indexer's releases when a release name doesn't state one, e.g. fr for a French tracker. Leave empty for indexers that mix languages."
                maw={320}
                value={torznab.language ?? ''}
                onChange={(e) => setTorznab({ ...torznab, language: e.currentTarget.value.trim() })}
              />
              {(downloadClients.length > 1 || !!torznab.downloadClient) && (
                <Select
                  label="Download client"
                  description="Where this indexer's releases are sent, e.g. a tracker that must go through the VPN client."
                  maw={320}
                  allowDeselect={false}
                  value={torznab.downloadClient ?? ''}
                  onChange={(v) => setTorznab({ ...torznab, downloadClient: v ?? '' })}
                  error={
                    torznab.downloadClient &&
                    !downloadClients.some((c) => c.id === torznab.downloadClient)
                      ? 'This client no longer exists. Pick another.'
                      : undefined
                  }
                  data={[
                    {
                      value: '',
                      label: `Default (${downloadClients[0]?.name ?? 'none configured'})`,
                    },
                    ...downloadClients.slice(1).map((c) => ({ value: c.id, label: c.name })),
                  ]}
                />
              )}
            </>
          )}
          <ProwlarrBrowse
            active={protocol === 'torznab'}
            apiKey={torznab.apiKey}
            onApply={(pick, picked) => {
              setTorznab({ ...torznab, url: pick.url, apiKey: pick.apiKey })
              suggestName(picked)
            }}
          />

          {protocol === 'source' && isEdit && (
            <Text size="xs" c="dimmed">
              {initial?.summary} — the extension/source of an entry cannot change; add a new indexer
              instead.
            </Text>
          )}
          {protocol === 'source' && configuring && (
            <Text size="xs" c="dimmed">
              {name} is added. Set what this source needs below, or close to keep its defaults.
            </Text>
          )}
          <ExtensionSourcePicker
            active={protocol === 'source'}
            picking={!isEdit && !configuring}
            extensions={extensions}
            pkg={pkg}
            pickedExt={pickedExt}
            extInstalled={extInstalled}
            source={source}
            prefValues={prefValues}
            setPrefValues={setPrefValues}
            onPickExtension={(p) => {
              setPkg(p)
              setSource({ sourceId: '' })
              setPrefValues({})
              setInstalledNow(false)
            }}
            onPickSource={(src, p) => {
              setSource({ sourceId: src.id, pkgName: p })
              suggestName(`${src.name} (${src.lang})`)
            }}
            onInstalled={() => setInstalledNow(true)}
            onClose={onClose}
          />

          <Group justify="flex-end" mt="xs">
            <Button variant="default" onClick={configuring ? onSaved : onClose}>
              {configuring ? 'Close' : 'Cancel'}
            </Button>
            <Button
              loading={save.isPending}
              disabled={
                !name.trim() ||
                (protocol === 'torznab' && !torznab.url.trim()) ||
                (protocol === 'source' && !isEdit && !source.sourceId)
              }
              onClick={() => save.mutate()}
            >
              {isEdit ? 'Save' : configuring ? 'Save settings' : 'Add'}
            </Button>
          </Group>
        </Stack>
      </fieldset>
    </Modal>
  )
}
