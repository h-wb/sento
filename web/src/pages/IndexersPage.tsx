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
  Title,
  Tooltip,
} from '@mantine/core'
import { IconCopy, IconPencil, IconPlugConnected, IconTrash } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  api,
  type IndexerBody,
  type IndexerView,
  type ProwlarrIndexer,
  type Source,
} from '../api'
import SourcePrefsFields from '../components/SourcePrefsFields'
import { notify, usePageTitle } from '../ui'

// Indexers are explicit entries (Sonarr's model): every searchable thing
// with its protocol — a Torznab endpoint or an extension source on the
// host. The entry's name is the display name everywhere.
export default function IndexersPage() {
  usePageTitle('Indexers')
  const queryClient = useQueryClient()
  const { data: indexers, error } = useQuery({ queryKey: ['indexers'], queryFn: api.indexers })
  const [editing, setEditing] = useState<IndexerView | 'new' | null>(null)
  const [duplicating, setDuplicating] = useState<IndexerView | null>(null)
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['indexers'] })

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
      <Group justify="space-between" mb="xs">
        <Title order={2}>Indexers</Title>
        <Button size="xs" onClick={() => setEditing('new')}>
          Add indexer
        </Button>
      </Group>
      <Text size="sm" c="dimmed" mb="md">
        Everything Sento searches, with its protocol: Torznab endpoints (torrents) and extension
        sources from the host. The name you give an entry is how it appears in results, history
        and the queue. Series can override the toggles individually.
      </Text>
      {error && (
        <Alert color="red" mb="md">
          {String(error)}
        </Alert>
      )}
      <Table striped maw={860}>
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
                  checked={ix.enabled}
                  onChange={(e) => patch.mutate({ id: ix.id, p: { enabled: e.currentTarget.checked } })}
                />
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  checked={ix.useVolumes}
                  disabled={!ix.enabled}
                  onChange={(e) =>
                    patch.mutate({ id: ix.id, p: { useVolumes: e.currentTarget.checked } })
                  }
                />
              </Table.Td>
              <Table.Td>
                <Switch
                  size="xs"
                  checked={ix.useChapters}
                  disabled={!ix.enabled}
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
                  defaultValue={ix.priority}
                  disabled={!ix.enabled}
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
                      size="sm"
                      variant="subtle"
                      loading={test.isPending && test.variables?.id === ix.id}
                      onClick={() => test.mutate(ix)}
                    >
                      <IconPlugConnected size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Edit">
                    <ActionIcon size="sm" variant="subtle" onClick={() => setEditing(ix)}>
                      <IconPencil size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Duplicate">
                    <ActionIcon size="sm" variant="subtle" onClick={() => setDuplicating(ix)}>
                      <IconCopy size={14} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Delete">
                    <ActionIcon
                      size="sm"
                      variant="subtle"
                      color="red"
                      onClick={() => {
                        if (window.confirm(`Remove indexer “${ix.name}”?`)) remove.mutate(ix)
                      }}
                    >
                      <IconTrash size={14} />
                    </ActionIcon>
                  </Tooltip>
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
      {indexers && indexers.length === 0 && (
        <Text c="dimmed" size="sm">
          No indexers yet. Add a Torznab endpoint (every Prowlarr/Jackett indexer is one) or an
          extension source from the host.
        </Text>
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

type TorznabForm = { url: string; apiKey: string }
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
  const isEdit = !!initial && !asCopy
  const init = initial?.settings as Partial<TorznabForm & SourceForm> | undefined
  const [protocol, setProtocol] = useState<'torznab' | 'source'>(initial?.protocol ?? 'torznab')
  const [name, setName] = useState(asCopy ? `${initial?.name} (copy)` : (initial?.name ?? ''))
  const [torznab, setTorznab] = useState<TorznabForm>({
    url: init?.url ?? '',
    apiKey: init?.apiKey ?? '',
  })
  const [source, setSource] = useState<SourceForm>({
    sourceId: init?.sourceId ?? '',
    pkgName: init?.pkgName,
  })
  // The source's own settings (login, mirrors…) edit in the same modal:
  // an overlay of changed values, pushed after the entry saves. Keyed by
  // the host source, so entries of the same source share them.
  const [prefValues, setPrefValues] = useState<Record<string, unknown>>({})

  // Extension catalog: the source flow starts from the extension — adding
  // its source as an indexer is what installs it.
  const extensions = useQuery({
    queryKey: ['extensions'],
    queryFn: api.extensions,
    enabled: protocol === 'source' && !isEdit,
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

  const loadSources = useMutation({
    mutationFn: async (p: string) => {
      const ext = (extensions.data ?? []).find((e) => e.pkgName === p)
      if (ext && !ext.installedVersion) await api.installExtension(p)
      return api.extensionSources(p)
    },
    onSuccess: (srcs, p) => {
      setExtSources(srcs)
      if (srcs.length === 1) {
        setSource({ sourceId: srcs[0].id, pkgName: p })
        suggestName(`${srcs[0].name} (${srcs[0].lang})`)
      }
    },
    onError: notify.err,
  })

  // Prowlarr browse assist for torznab entries.
  const [browse, setBrowse] = useState<{ url: string; apiKey: string } | null>(null)
  const [found, setFound] = useState<ProwlarrIndexer[] | null>(null)
  const browseMut = useMutation({
    mutationFn: () => api.browseProwlarr(browse!),
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
      if (isEdit) {
        await api.updateIndexer(initial.id, body)
      } else if (createdId.current === null) {
        createdId.current = (await api.addIndexer(body)).id
      }
      for (const [key, value] of Object.entries(prefValues)) {
        await api.setSourcePreference(source.sourceId, key, value)
      }
    },
    onSuccess: () => {
      notify.ok(isEdit ? 'Indexer updated' : 'Indexer added')
      onSaved()
    },
    onError: notify.err,
  })

  return (
    <Modal opened onClose={onClose} title={isEdit ? `Edit ${initial.name}` : 'Add indexer'} size="lg">
      <Stack gap="sm">
        {!isEdit && (
          <SegmentedControl
            value={protocol}
            onChange={(v) => setProtocol(v as 'torznab' | 'source')}
            data={[
              { value: 'torznab', label: 'Torznab (torrents)' },
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
              description="Prowlarr and Jackett expose every indexer as a Torznab endpoint"
              value={torznab.url}
              onChange={(e) => setTorznab({ ...torznab, url: e.currentTarget.value })}
            />
            <PasswordInput
              label="API key"
              value={torznab.apiKey}
              onChange={(e) => setTorznab({ ...torznab, apiKey: e.currentTarget.value })}
            />
            {!browse && (
              <Button
                variant="subtle"
                size="compact-sm"
                style={{ alignSelf: 'flex-start' }}
                onClick={() => setBrowse({ url: '', apiKey: torznab.apiKey })}
              >
                Browse a Prowlarr instead…
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
                  onClick={() => browseMut.mutate()}
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
                      setTorznab({ url: pick.url, apiKey: browse.apiKey })
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
        {protocol === 'source' && !isEdit && (
          <>
            <Select
              label="Extension"
              description={
                extensions.isPending
                  ? 'Loading the extension catalog…'
                  : `${(extensions.data ?? []).length} extensions from your repos — picking one installs it`
              }
              placeholder="Search extensions…"
              data={(extensions.data ?? []).map((e) => ({
                value: e.pkgName,
                label: `${e.name} (${e.lang})${e.nsfw ? ' · nsfw' : ''}${e.installedVersion ? ' · installed' : ''}`,
              }))}
              value={pkg || null}
              searchable
              limit={30}
              nothingFoundMessage="No extension matches — check the repos on the Sources page"
              onChange={(v) => {
                if (!v) return
                setPkg(v)
                setExtSources(null)
                setSource({ sourceId: '' })
                setPrefValues({})
                loadSources.mutate(v)
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
        {protocol === 'source' && source.sourceId && (
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
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button
            loading={save.isPending}
            disabled={!name.trim() || (protocol === 'source' && !isEdit && !source.sourceId)}
            onClick={() => save.mutate()}
          >
            {isEdit ? 'Save' : 'Add'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
