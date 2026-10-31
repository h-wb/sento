import { useState, type ReactNode } from 'react'
import { Alert, Button, Group, Menu, Modal, Stack, Text } from '@mantine/core'
import { IconDotsVertical, IconTrash } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { api, type Edition } from '../api'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'
import { editionDetails } from './editionText'
// A card's menu: what it collects, then the edition's rarer maintenance.
export default function EditionActions({
  seriesID,
  edition,
  title,
  others,
  run,
  children,
  more,
}: {
  children?: ReactNode
  // Rare maintenance, one level down: the menu opens on what is common.
  more?: ReactNode
  seriesID: number
  edition: Edition
  // The card's title, which names the menu.
  title: string
  others: Edition[]
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const { confirm, confirmation } = useConfirmAction()
  const [merge, setMerge] = useState<Edition | null>(null)
  // A local edition merges into its duplicate; a catalog edition into
  // another of the same MangaBaka edition it was kept apart from.
  const candidates = others.filter((o) => {
    if (o.language !== edition.language || o.type !== edition.type) return false
    if (edition.custom)
      return (
        o.editionName === edition.editionName &&
        o.publisher === edition.publisher &&
        o.medium === edition.medium
      )
    return (
      !o.custom &&
      (edition.providerEditionId
        ? o.providerEditionId === edition.providerEditionId
        : o.editionName === edition.editionName)
    )
  })
  return (
    <>
      <Menu position="bottom-end" withinPortal>
        <Menu.Target>
          <Button
            size="compact-xs"
            variant="subtle"
            px={4}
            aria-label={`More actions for ${title}`}
          >
            <IconDotsVertical size={14} />
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          {children}
          {(more || candidates.length > 0 || edition.collections.length > 1) && (
            <Menu.Sub position="left-start">
              <Menu.Sub.Target>
                <Menu.Sub.Item>More</Menu.Sub.Item>
              </Menu.Sub.Target>
              <Menu.Sub.Dropdown>
                {more}
                {candidates.length > 0 && (
                  <>
                    <Menu.Label>Merge into</Menu.Label>
                    {candidates.map((o) => (
                      <Menu.Item key={o.id} onClick={() => setMerge(o)}>
                        {o.editionName} · {editionDetails(o)}
                      </Menu.Item>
                    ))}
                  </>
                )}
                {edition.collections.length > 1 && (
                  <>
                    <Menu.Label>Separate into its own edition</Menu.Label>
                    {edition.collections.map((c) => (
                      <Menu.Item
                        key={c.id}
                        onClick={() =>
                          confirm({
                            title: `Separate the ${c.medium} collection?`,
                            message:
                              'Use this when it is a different publication, such as another publisher’s translation. Its books become an edition of their own, not collected until you choose to. Files stay with the edition they are in.',
                            label: 'Separate',
                            onConfirm: async () => {
                              await api.separateCollection(seriesID, c.id)
                              run(async () => {}, 'Collection separated')
                            },
                          })
                        }
                      >
                        {[c.medium, c.publisher].filter(Boolean).join(' · ')}
                      </Menu.Item>
                    ))}
                  </>
                )}
              </Menu.Sub.Dropdown>
            </Menu.Sub>
          )}
          {edition.custom && (
            <>
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
                      await api.deleteEdition(seriesID, edition.id)
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
          edition={edition}
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
  edition,
  into,
  close,
  done,
}: {
  seriesID: number
  edition: Edition
  into: Edition
  close: () => void
  done: () => void
}) {
  const preview = useQuery({
    queryKey: ['series', seriesID, 'merge-preview', edition.id, into.id],
    queryFn: () => api.mergePreview(seriesID, edition.id, into.id),
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
              {p.localBooks.length} books · {p.fileCount} files · {p.localVersions.length} versions
            </Text>
            {p.localBooks.map((w) => (
              <Text size="xs" key={w.id}>
                {w.sequenceStr} · {w.countType} →{' '}
                {p.mapping[w.id]
                  ? p.catalogBooks.find((v) => v.id === p.mapping[w.id])?.sequenceStr
                  : 'No match'}
              </Text>
            ))}
            {p.localVersions.map((t) => (
              <Text size="xs" key={t.id}>
                Version: {t.name}
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
                await api.mergeEditions(seriesID, edition.id, into.id)
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
