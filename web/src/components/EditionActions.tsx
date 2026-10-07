import { useState, type ReactNode } from 'react'
import { Alert, Button, Group, Menu, Modal, Stack, Text } from '@mantine/core'
import { IconDotsVertical, IconTrash } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { api, type Collection } from '../api'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'
// Local edition management lives alongside its books and collected versions.
export default function EditionActions({
  seriesID,
  collection,
  others,
  run,
  children,
}: {
  children?: ReactNode
  seriesID: number
  collection: Collection
  others: Collection[]
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const { confirm, confirmation } = useConfirmAction()
  const [merge, setMerge] = useState<Collection | null>(null)
  const candidates = others.filter(
    (o) =>
      o.editionName === collection.editionName &&
      o.language === collection.language &&
      o.publisher === collection.publisher &&
      o.medium === collection.medium &&
      o.type === collection.type &&
      JSON.stringify(o.editionTraits) === JSON.stringify(collection.editionTraits) &&
      o.contentVariant === collection.contentVariant,
  )
  return (
    <>
      <Menu position="bottom-end" withinPortal>
        <Menu.Target>
          <Button
            size="compact-xs"
            variant="subtle"
            px={4}
            aria-label={`More actions for ${collection.editionName}`}
          >
            <IconDotsVertical size={14} />
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          {children}
          {collection.custom && (
            <>
              <Menu.Label>Merge into</Menu.Label>
              {candidates.map((o) => (
                <Menu.Item key={o.id} onClick={() => setMerge(o)}>
                  {o.editionName} · {o.language} {o.medium ? `· ${o.medium}` : ''}
                </Menu.Item>
              ))}
              {candidates.length === 0 && <Menu.Item disabled>No matching edition</Menu.Item>}
              <Menu.Divider />
              <Menu.Item
                color="red"
                leftSection={<IconTrash size={14} />}
                onClick={() =>
                  confirm({
                    title: 'Remove local edition?',
                    message:
                      'This removes the edition from your library. Owned files and pending downloads must be unlinked first; files on disk are kept.',
                    label: 'Remove edition',
                    onConfirm: async () => {
                      await api.deleteCollection(seriesID, collection.id)
                      run(async () => {}, 'Edition removed')
                    },
                  })
                }
              >
                Remove edition
              </Menu.Item>
            </>
          )}
        </Menu.Dropdown>
      </Menu>
      {confirmation}
      {merge && (
        <MergePreview
          seriesID={seriesID}
          collection={collection}
          into={merge}
          close={() => setMerge(null)}
          done={() => {
            setMerge(null)
            run(async () => {}, 'Editions merged')
          }}
        />
      )}
    </>
  )
}

function MergePreview({
  seriesID,
  collection,
  into,
  close,
  done,
}: {
  seriesID: number
  collection: Collection
  into: Collection
  close: () => void
  done: () => void
}) {
  const preview = useQuery({
    queryKey: ['series', seriesID, 'merge-preview', collection.id, into.id],
    queryFn: () => api.mergePreview(seriesID, collection.id, into.id),
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const p = preview.data
  return (
    <Modal
      opened
      title="Review edition merge"
      onClose={() => {
        if (!busy) close()
      }}
      closeOnEscape={!busy}
      closeOnClickOutside={!busy}
    >
      <Stack>
        <Text size="sm">
          The destination edition keeps its metadata. Your collected versions, their policies, pins,
          and confirmed file mappings are retained. Files stay in place.
        </Text>
        <QueryFeedback
          pending={preview.isPending}
          error={preview.error}
          onRetry={() => void preview.refetch()}
        />
        {p && (
          <>
            <Text size="sm">
              {p.localBooks.length} books · {p.fileCount} files · {p.localTargets.length} versions
            </Text>
            {p.localBooks.map((w) => (
              <Text size="xs" key={w.id}>
                {w.sequenceStr} · {w.countType} →{' '}
                {p.mapping[w.id]
                  ? p.catalogBooks.find((v) => v.id === p.mapping[w.id])?.sequenceStr
                  : 'No match'}
              </Text>
            ))}
            {p.localTargets.map((t) => (
              <Text size="xs" key={t.id}>
                Version: {t.name}
                {t.requiredGroup ? ` · group ${t.requiredGroup}` : ''}
                {t.profileId ? ` · profile ${t.profileId}` : ' · inherited profile'}
              </Text>
            ))}
            {p.pending && <Alert color="orange">Finish pending downloads before merging.</Alert>}
            {p.unmatched.length > 0 && (
              <Alert color="orange">
                Some books have no unique match. Resolve them before merging.
              </Alert>
            )}
          </>
        )}
        {error && <Alert color="red">{error}</Alert>}
        <Group justify="flex-end">
          <Button variant="default" disabled={busy} onClick={close}>
            Cancel
          </Button>
          <Button
            disabled={!p || p.pending || p.unmatched.length > 0}
            loading={busy}
            onClick={async () => {
              setBusy(true)
              setError('')
              try {
                await api.mergeCollection(seriesID, collection.id, into.id)
                done()
              } catch (e) {
                setError(String(e))
              } finally {
                setBusy(false)
              }
            }}
          >
            Merge duplicate editions
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
