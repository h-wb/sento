import { useState } from 'react'
import { Button, Group } from '@mantine/core'
import { IconPackages } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type IndexerView } from '../api'
import { extensionsQuery } from '../queries'
import { notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { useConfirmAction } from '../components/ConfirmAction'
import { IndexerSearchTest } from '../components/IndexerSearchTest'
import { IndexerTable } from '../components/indexers/IndexerTable'
import { IndexerModal } from '../components/indexers/IndexerModal'
import { ManageExtensionsModal } from '../components/extensions/ManageExtensionsModal'

// Indexers are explicit entries (Sonarr's model): every searchable thing
// with its protocol — a Torznab endpoint or an extension source on the
// host. The entry's name is the display name everywhere.
export default function IndexersPage() {
  usePageTitle('Indexers')
  const { confirm, confirmation } = useConfirmAction()
  const queryClient = useQueryClient()
  const { data: indexers, error, isPending, refetch } = useQuery({ queryKey: ['indexers'], queryFn: api.indexers })
  // The sidecar's extension catalog: drives the version/update line on
  // extension-source rows and the Manage extensions dialog. Undefined
  // (query failed) when no sidecar is configured — then neither shows.
  const extensions = useQuery(extensionsQuery)
  const extByPkg = new Map((extensions.data ?? []).map((e) => [e.pkgName, e]))
  const [editing, setEditing] = useState<IndexerView | 'new' | null>(null)
  const [duplicating, setDuplicating] = useState<IndexerView | null>(null)
  const [managing, setManaging] = useState(false)
  const [searchTesting, setSearchTesting] = useState<IndexerView | null>(null)
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['health'] })
    // Saving an entry may have installed its extension on the host.
    queryClient.invalidateQueries({ queryKey: ['extensions'] })
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
          <Group gap="xs" wrap="nowrap">
            {extensions.data && (
              <Button
                size="xs"
                variant="default"
                leftSection={<IconPackages size={14} />}
                onClick={() => setManaging(true)}
              >
                Manage extensions
              </Button>
            )}
            <Button size="xs" onClick={() => setEditing('new')}>
              Add indexer
            </Button>
          </Group>
        }
      />
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {!!indexers?.length && (
        <IndexerTable
          indexers={indexers}
          extByPkg={extByPkg}
          patch={patch}
          test={test}
          remove={remove}
          confirm={confirm}
          onSearchTest={setSearchTesting}
          onEdit={setEditing}
          onDuplicate={setDuplicating}
        />
      )}
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
      {managing && <ManageExtensionsModal onClose={() => setManaging(false)} />}
      {searchTesting && <IndexerSearchTest indexer={searchTesting} onClose={() => setSearchTesting(null)} />}
    </>
  )
}
