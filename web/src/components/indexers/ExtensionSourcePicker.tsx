import { useState, type Dispatch, type SetStateAction } from 'react'
import { Alert, Button, Divider, Group, Select, Text, TextInput } from '@mantine/core'
import { useMutation, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type ExtensionView, type Source } from '../../api'
import { errorMessage, notify } from '../../ui'
import SourcePrefsFields from '../SourcePrefsFields'
import type { SourceForm } from './IndexerModal'

// The source flow of the indexer modal: pick an extension from the catalog,
// pick one of its sources, and set the source's own preferences. Adding a
// repo from inside the dialog covers the case where the catalog is empty
// because there is none yet.
export function ExtensionSourcePicker({
  active,
  picking,
  extensions,
  pkg,
  pickedExt,
  extInstalled,
  source,
  prefValues,
  setPrefValues,
  onPickExtension,
  onPickSource,
  onInstalled,
  onClose,
}: {
  // Whether the source protocol is chosen; the picker keeps its state while not.
  active: boolean
  // Adding a new entry, before it saved: the extension and source can still change.
  picking: boolean
  extensions: UseQueryResult<ExtensionView[], Error>
  pkg: string
  pickedExt: ExtensionView | undefined
  extInstalled: boolean
  source: SourceForm
  prefValues: Record<string, unknown>
  setPrefValues: Dispatch<SetStateAction<Record<string, unknown>>>
  onPickExtension: (pkg: string) => void
  onPickSource: (src: Source, pkg: string) => void
  onInstalled: () => void
  onClose: () => void
}) {
  const queryClient = useQueryClient()
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
  const [extSources, setExtSources] = useState<Source[] | null>(null)

  const offerSources = (srcs: Source[], p: string) => {
    setExtSources(srcs)
    if (srcs.length === 1) onPickSource(srcs[0], p)
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
      onInstalled()
      offerSources(srcs, p)
    },
    onError: notify.err,
  })

  if (!active) return null
  return (
    <>
      {picking && extensions.error && (
        // No host answering: nothing can be listed, and the fix is on
        // another page.
        <Alert color="yellow" title="Extension sources are not available">
          <Text size="sm">{errorMessage(extensions.error)}</Text>
          <Text size="sm" mt={4}>
            Connect the extensions sidecar on the{' '}
            <Link to="/connections" onClick={onClose}>
              Connections
            </Link>{' '}
            page first.
          </Text>
        </Alert>
      )}
      {picking && extensions.data?.length === 0 && (
        // A connected host with nothing to offer means no repo (or only
        // failing ones). Adding one is a single field — do it here
        // rather than sending the user to another page and back.
        <Alert color="blue" title="No extensions to choose from yet">
          <Text size="sm" mb="xs">
            Extensions come from repos, and none ship with Sento. Add a Mihon-compatible repo index
            URL to load its catalog.
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
      {picking && !extensions.error && extensions.data?.length !== 0 && (
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
              onPickExtension(v)
              setExtSources(null)
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
                if (!src) return
                onPickSource(src, pkg)
                setPrefValues({})
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
      {source.sourceId && !extInstalled && (
        <Text size="xs" c="dimmed">
          Adding installs {pickedExt?.name ?? 'the extension'} on the host. If the source has
          settings of its own (login, mirrors…), they are offered right after.
        </Text>
      )}
      {source.sourceId && extInstalled && (
        <>
          <Divider label="Source settings" labelPosition="left" mt="xs" />
          <Text size="xs" c="dimmed" mt={-6}>
            The source's own settings (login, mirrors…) — shared by every entry using this source,
            applied when you save.
          </Text>
          <SourcePrefsFields
            sourceId={source.sourceId}
            values={prefValues}
            onChange={(key, value) => setPrefValues((p) => ({ ...p, [key]: value }))}
          />
        </>
      )}
    </>
  )
}
