import { useRef, useState } from 'react'
import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Divider,
  Group,
  Modal,
  NumberInput,
  PasswordInput,
  SegmentedControl,
  Select,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
  Tooltip,
} from '@mantine/core'
import { IconCopy, IconPencil, IconPlugConnected, IconTrash } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  api,
  type IndexerBody,
  type IndexerView,
  type ProwlarrIndexer,
  type Source,
} from '../api'
import SourcePrefsFields from '../components/SourcePrefsFields'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { useConfirmAction } from '../components/ConfirmAction'

// Indexers are explicit entries (Sonarr's model): every searchable thing
// with its protocol — a Torznab endpoint or an extension source on the
// host. The entry's name is the display name everywhere.
export default function IndexersPage() {
  usePageTitle('Indexers')
  const { confirm, confirmation } = useConfirmAction()
  const queryClient = useQueryClient()
  const { data: indexers, error, isPending, refetch } = useQuery({ queryKey: ['indexers'], queryFn: api.indexers })
  const [editing, setEditing] = useState<IndexerView | 'new' | null>(null)
  const [duplicating, setDuplicating] = useState<IndexerView | null>(null)
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['health'] })
    return queryClient.invalidateQueries({ queryKey: ['indexers'] })
  }

  const patch = useMutation({
    mutationFn: ({ id, p }: { id: number; p: Parameters<typeof api.patchIndexer>[1] }) =>
      api.patchIndexer(id, p),
    onError: notify.err,
    onSettled: refresh,
  })
  const test = useMutation({
    mutationFn: (ix: IndexerView) => api.testIndexer(ix.id),
    onSuccess: (_d, ix) => notify.ok(`${ix.name} is reachable`),
    onError: notify.err,
  })
  const remove = useMutation({
    mutationFn: (ix: IndexerView) => api.deleteIndexer(ix.id),
    onSuccess: () => notify.ok('Indexer removed'),
    onError: notify.err,
    onSettled: refresh,
  })

  return (
    <>
      <PageHeader
        title="Indexers"
        description="Where Sento finds releases. Choose which indexers search volumes and chapters."
        action={
          <Button size="xs" onClick={() => setEditing('new')}>
            Add indexer
          </Button>
        }
      />
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {!!indexers?.length && <Table.ScrollContainer minWidth={700}><Table highlightOnHover verticalSpacing="sm" className="dataTable">
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
          {indexers?.map((ix) => (
            <Table.Tr key={ix.id} opacity={ix.reachable ? 1 : 0.55}>
              <Table.Td>
                <Text size="sm">{ix.name}</Text>
                <Text size="xs" c="dimmed">
                  {ix.summary}
                  {!ix.reachable && ' — unreachable'}
                </Text>
              </Table.Td>
              <Table.Td>
                <Badge size="xs" variant="outline" color={ix.protocol === 'torznab' ? 'blue' : 'teal'}>
                  {ix.protocol}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  aria-label={`Enable ${ix.name}`}
                  disabled={patch.isPending}
                  checked={ix.enabled}
                  onChange={(e) => patch.mutate({ id: ix.id, p: { enabled: e.currentTarget.checked } })}
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
                    if (!isNaN(v) && v !== ix.priority) patch.mutate({ id: ix.id, p: { priority: v } })
                  }}
                />
              </Table.Td>
              <Table.Td>
                <Group gap={2} wrap="nowrap" justify="flex-end">
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
                    <ActionIcon size="md" variant="subtle" aria-label={`Edit ${ix.name}`} onClick={() => setEditing(ix)}>
                      <IconPencil size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Duplicate">
                    <ActionIcon size="md" variant="subtle" aria-label={`Duplicate ${ix.name}`} onClick={() => setDuplicating(ix)}>
                      <IconCopy size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Delete">
                    <ActionIcon
                      size="sm"
                      variant="subtle"
                      color="red"
                      aria-label={`Remove ${ix.name}`}
                      onClick={() => confirm({ title: 'Remove indexer?', message: `“${ix.name}” will no longer be searched. If this is an extension's last indexer, its extension will be uninstalled.`, label: 'Remove indexer', onConfirm: () => remove.mutateAsync(ix) })}
                    >
                      <IconTrash size={14} />
                    </ActionIcon>
                  </Tooltip>
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table></Table.ScrollContainer>}
      {confirmation}
      {indexers && indexers.length === 0 && (
        <EmptyState title="No indexers yet" description="Add a torrent indexer from Prowlarr or Jackett, or choose an extension source." action={<Button size="sm" variant="default" onClick={() => setEditing('new')}>Add your first indexer</Button>} />
      )}
      {(editing || duplicating) && (
        <IndexerModal
          initial={editing === 'new' ? null : (editing ?? duplicating)}
          asCopy={!!duplicating}
          onClose={() => {
            setEditing(null)
            setDuplicating(null)
          }}
          onSaved={() => {
            setEditing(null)
            setDuplicating(null)
            refresh()
          }}
        />
      )}
    </>
  )
}

type TorznabForm = { url: string; apiKey: string; language?: string; downloadClient?: string }
type SourceForm = { sourceId: string; pkgName?: string }

function IndexerModal({
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
  const queryClient = useQueryClient()
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
  const connections = useQuery({ queryKey: ['connections'], queryFn: api.connections })
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
    queryKey: ['extensions'],
    queryFn: api.extensions,
    enabled: protocol === 'source' && !isEdit,
  })
  // Adding a repo from inside the dialog, for the case where the catalog
  // is empty because there is none yet.
  const [newRepo, setNewRepo] = useState('')
  const [repoProblem, setRepoProblem] = useState('')
  const addRepo = useMutation({
    mutationFn: async () => {
      const current = await api.extensionRepos()
      return api.setExtensionRepos([...current.urls, newRepo.trim()])
    },
    onSuccess: (data) => {
      const added = data.repos?.find((r) => r.url === newRepo.trim())
      // Saved either way; say so when it yields nothing, since the
      // catalog below will stay empty.
      setRepoProblem(
        added?.error
          ? `Added, but it is failing: ${added.error}`
          : added && added.extensions === 0
            ? 'Added, but it lists no extension this host can run.'
            : '',
      )
      setNewRepo('')
      queryClient.invalidateQueries({ queryKey: ['extensions'] })
      queryClient.invalidateQueries({ queryKey: ['extension-repos'] })
    },
    onError: notify.err,
  })
  const [pkg, setPkg] = useState(init?.pkgName ?? '')
  const [extSources, setExtSources] = useState<Source[] | null>(null)
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

  const offerSources = (srcs: Source[], p: string) => {
    setExtSources(srcs)
    if (srcs.length === 1) {
      setSource({ sourceId: srcs[0].id, pkgName: p })
      suggestName(`${srcs[0].name} (${srcs[0].lang})`)
    }
  }
  // Fallback for repos whose index does not declare sources: the only way
  // to learn them is to install the extension and ask it.
  const loadSources = useMutation({
    mutationFn: async (p: string) => {
      const ext = (extensions.data ?? []).find((e) => e.pkgName === p)
      if (ext && !ext.installedVersion) await api.installExtension(p)
      return api.extensionSources(p)
    },
    onSuccess: (srcs, p) => {
      setInstalledNow(true)
      offerSources(srcs, p)
    },
    onError: notify.err,
  })
  // The source's own settings can only be read from an installed
  // extension. When adding installs it, the modal stays open on those
  // settings instead of closing (`configuring`).
  const [installedNow, setInstalledNow] = useState(false)
  const [configuring, setConfiguring] = useState(false)
  const extInstalled = isEdit || installedNow || !!pickedExt?.installedVersion

  // Prowlarr browse assist for torznab entries.
  const [browse, setBrowse] = useState<{ url: string; apiKey: string } | null>(null)
  const [found, setFound] = useState<ProwlarrIndexer[] | null>(null)
  const browseMut = useMutation({
    mutationFn: (b: { url: string; apiKey: string }) => api.browseProwlarr(b),
    onSuccess: setFound,
    onError: notify.err,
  })

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
      closeOnClickOutside={!save.isPending} closeOnEscape={!save.isPending} withCloseButton={!save.isPending}
    >
      <fieldset className="formFields" disabled={save.isPending}><Stack gap="sm">
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
                  torznab.downloadClient && !downloadClients.some((c) => c.id === torznab.downloadClient)
                    ? 'This client no longer exists. Pick another.'
                    : undefined
                }
                data={[
                  { value: '', label: `Default (${downloadClients[0]?.name ?? 'none configured'})` },
                  ...downloadClients.slice(1).map((c) => ({ value: c.id, label: c.name })),
                ]}
              />
            )}
            {!browse && (
              <Button
                variant="subtle"
                size="compact-sm"
                style={{ alignSelf: 'flex-start' }}
                onClick={async () => {
                  // The Prowlarr Sento already knows (seeded by the
                  // deployment or remembered from the last browse): list
                  // its indexers at once instead of asking again.
                  const known = (await api.connections().catch(() => null))?.prowlarr
                  const start = { url: known?.url ?? '', apiKey: known?.apiKey || torznab.apiKey }
                  setBrowse(start)
                  if (start.url && start.apiKey) browseMut.mutate(start)
                }}
              >
                Choose from Prowlarr…
              </Button>
            )}
            {browse && (
              <Stack
                gap="xs"
                p="xs"
                style={{ border: '1px dashed var(--mantine-color-gray-6)', borderRadius: 6 }}
              >
                <Text size="xs" c="dimmed">
                  Pick an indexer from a Prowlarr — the per-indexer Torznab URL and key fill in.
                </Text>
                <Group grow>
                  <TextInput
                    size="xs"
                    placeholder="Prowlarr URL"
                    value={browse.url}
                    onChange={(e) => setBrowse({ ...browse, url: e.currentTarget.value })}
                  />
                  <PasswordInput
                    size="xs"
                    placeholder="API key"
                    value={browse.apiKey}
                    onChange={(e) => setBrowse({ ...browse, apiKey: e.currentTarget.value })}
                  />
                </Group>
                <Button
                  size="compact-sm"
                  variant="default"
                  loading={browseMut.isPending}
                  disabled={!browse.url}
                  onClick={() => browseMut.mutate(browse)}
                >
                  List indexers
                </Button>
                {found && (
                  <Select
                    size="xs"
                    placeholder={`${found.length} indexers — pick one`}
                    data={found.map((f) => ({ value: f.url, label: f.name }))}
                    searchable
                    onChange={(url) => {
                      const pick = found.find((f) => f.url === url)
                      if (!pick) return
                      setTorznab({ ...torznab, url: pick.url, apiKey: browse.apiKey })
                      suggestName(pick.name)
                    }}
                  />
                )}
              </Stack>
            )}
          </>
        )}

        {protocol === 'source' && isEdit && (
          <Text size="xs" c="dimmed">
            {initial?.summary} — the extension/source of an entry cannot change; add a new
            indexer instead.
          </Text>
        )}
        {protocol === 'source' && configuring && (
          <Text size="xs" c="dimmed">
            {name} is added. Set what this source needs below, or close to keep its defaults.
          </Text>
        )}
        {protocol === 'source' && !isEdit && !configuring && extensions.error && (
          // No host answering: nothing can be listed, and the fix is on
          // another page.
          <Alert color="yellow" title="Extension sources are not available">
            <Text size="sm">{String(extensions.error)}</Text>
            <Text size="sm" mt={4}>
              Connect the extensions sidecar on the{' '}
              <Link to="/connections" onClick={onClose}>
                Connections
              </Link>{' '}
              page first.
            </Text>
          </Alert>
        )}
        {protocol === 'source' && !isEdit && !configuring && extensions.data?.length === 0 && (
          // A connected host with nothing to offer means no repo (or only
          // failing ones). Adding one is a single field — do it here
          // rather than sending the user to another page and back.
          <Alert color="blue" title="No extensions to choose from yet">
            <Text size="sm" mb="xs">
              Extensions come from repos, and none ship with Sento. Add a Mihon-compatible repo
              index URL to load its catalog.
            </Text>
            <Group gap={6} wrap="nowrap">
              <TextInput
                size="xs"
                style={{ flex: 1 }}
                aria-label="Extension repo URL"
                placeholder="https://…/index.min.json"
                value={newRepo}
                onChange={(e) => setNewRepo(e.currentTarget.value)}
              />
              <Button
                size="xs"
                variant="default"
                loading={addRepo.isPending}
                disabled={!newRepo.trim()}
                onClick={() => addRepo.mutate()}
              >
                Add repo
              </Button>
            </Group>
            {repoProblem && (
              <Text size="xs" c="red" mt={6}>
                {repoProblem}
              </Text>
            )}
          </Alert>
        )}
        {protocol === 'source' && !isEdit && !configuring && !extensions.error && extensions.data?.length !== 0 && (
          <>
            <Select
              label="Extension"
              description={
                extensions.isPending
                  ? 'Loading the extension catalog…'
                  : `${(extensions.data ?? []).length} extensions from your repos — installed when you add the indexer`
              }
              placeholder="Search extensions…"
              data={(extensions.data ?? []).map((e) => ({
                value: e.pkgName,
                label: `${e.name} (${e.lang})${e.nsfw ? ' · nsfw' : ''}${e.installedVersion ? ' · installed' : ''}`,
              }))}
              value={pkg || null}
              searchable
              limit={30}
              nothingFoundMessage="No extension matches — repos are managed on the Connections page"
              onChange={(v) => {
                if (!v) return
                setPkg(v)
                setExtSources(null)
                setSource({ sourceId: '' })
                setPrefValues({})
                setInstalledNow(false)
                const declared = (extensions.data ?? []).find((e) => e.pkgName === v)?.sources
                if (declared && declared.length > 0) offerSources(declared, v)
                else loadSources.mutate(v)
              }}
            />
            {loadSources.isPending && (
              <Text size="xs" c="dimmed">
                {pickedExt && !pickedExt.installedVersion
                  ? `Installing ${pickedExt.name} on the host…`
                  : 'Loading sources…'}
              </Text>
            )}
            {extSources && extSources.length > 1 && (
              <Select
                label="Source"
                description="This extension provides several sources (languages/mirrors)"
                data={extSources.map((s) => ({ value: s.id, label: `${s.name} (${s.lang})` }))}
                value={source.sourceId || null}
                searchable
                onChange={(v) => {
                  if (!v) return
                  const src = extSources.find((s) => s.id === v)
                  setSource({ sourceId: v, pkgName: pkg })
                  setPrefValues({})
                  if (src) suggestName(`${src.name} (${src.lang})`)
                }}
              />
            )}
            {extSources && extSources.length === 1 && (
              <Text size="xs" c="dimmed">
                Source: {extSources[0].name} ({extSources[0].lang})
              </Text>
            )}
          </>
        )}
        {protocol === 'source' && source.sourceId && !extInstalled && (
          <Text size="xs" c="dimmed">
            Adding installs {pickedExt?.name ?? 'the extension'} on the host. If the source has
            settings of its own (login, mirrors…), they are offered right after.
          </Text>
        )}
        {protocol === 'source' && source.sourceId && extInstalled && (
          <>
            <Divider label="Source settings" labelPosition="left" mt="xs" />
            <Text size="xs" c="dimmed" mt={-6}>
              The source's own settings (login, mirrors…) — shared by every entry using this
              source, applied when you save.
            </Text>
            <SourcePrefsFields
              sourceId={source.sourceId}
              values={prefValues}
              onChange={(key, value) => setPrefValues((p) => ({ ...p, [key]: value }))}
            />
          </>
        )}

        <Group justify="flex-end" mt="xs">
          <Button variant="default" onClick={configuring ? onSaved : onClose}>
            {configuring ? 'Close' : 'Cancel'}
          </Button>
          <Button
            loading={save.isPending}
            disabled={!name.trim() || (protocol === 'torznab' && !torznab.url.trim()) || (protocol === 'source' && !isEdit && !source.sourceId)}
            onClick={() => save.mutate()}
          >
            {isEdit ? 'Save' : configuring ? 'Save settings' : 'Add'}
          </Button>
        </Group>
      </Stack></fieldset>
    </Modal>
  )
}
