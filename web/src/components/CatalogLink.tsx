import { useEffect, useState } from 'react'
import { Alert, Button, Group, Modal, Select, Stack, Text } from '@mantine/core'
import { api } from '../api'
import type { Edition, CatalogLinkPreview } from '../types/store'
import { useEditor } from './useEditor'

const editionLabel = (c: Edition) =>
  [c.language, c.editionName, c.publisher, c.medium].filter(Boolean).join(' · ')
export function CatalogLink({
  seriesID,
  value,
  editions,
  catalogId,
  done,
  close,
}: {
  seriesID: number
  value: Edition
  editions: Edition[]
  // A suggested catalog edition: preselected, its matches shown at once.
  catalogId?: number
  done: () => Promise<void>
  close: () => void
}) {
  const [id, setID] = useState<string | null>(catalogId ? String(catalogId) : null)
  const [preview, setPreview] = useState<CatalogLinkPreview | null>(null)
  const edit = useEditor()
  const loadPreview = (catalog: number) =>
    edit.run(async () => setPreview(await api.catalogLinkPreview(seriesID, value.id, catalog)))
  useEffect(() => {
    if (catalogId) void loadPreview(catalogId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const added = preview
    ? preview.catalogBooks.filter((b) => !Object.values(preview.mapping).includes(b.id)).length
    : 0
  return (
    <Modal
      opened
      onClose={() => {
        if (!edit.busy) close()
      }}
      title="Link edition to MangaBaka"
    >
      <Stack>
        <Text size="sm">
          Your edition becomes this catalog edition and follows MangaBaka from now on. Its files,
          collected versions, and the fields you edited by hand are kept. Review the book matches
          before linking.
        </Text>
        <Select
          label="Catalog edition"
          data={editions
            .filter(
              (c) =>
                !c.custom &&
                c.id !== value.id &&
                c.type === value.type &&
                c.language === value.language,
            )
            .map((c) => ({ value: String(c.id), label: editionLabel(c) }))}
          value={id}
          onChange={(v) => {
            setID(v)
            setPreview(null)
          }}
        />
        <Button
          variant="default"
          disabled={!id}
          loading={edit.busy}
          onClick={() => void loadPreview(Number(id))}
        >
          Preview matches
        </Button>
        {preview && (
          <>
            <Text size="sm">
              {Object.keys(preview.mapping).length} of your books match the catalog
              {preview.unmatched.length > 0 && `; ${preview.unmatched.length} do not and stay as they are`}
              {added > 0 && `; ${added} more from the catalog will be added`}.
            </Text>
            <Stack gap={4}>
              {preview.localBooks.map((w) => (
                <Text key={w.id} size="xs">
                  {w.sequenceStr} · {w.countType} →{' '}
                  {preview.mapping[w.id]
                    ? `${preview.catalogBooks.find((v) => v.id === preview.mapping[w.id])?.sequenceStr} · ${preview.catalogBooks.find((v) => v.id === preview.mapping[w.id])?.countType}`
                    : 'No catalog match; kept locally'}
                </Text>
              ))}
            </Stack>
            <Text size="xs" c="dimmed">
              The separate catalog entry is folded into your edition, so it appears once.
            </Text>
          </>
        )}
        {edit.error && <Alert color="red">{edit.error}</Alert>}
        <Group justify="flex-end">
          <Button variant="default" disabled={edit.busy} onClick={close}>
            Cancel
          </Button>
          <Button
            disabled={!preview}
            loading={edit.busy}
            onClick={() =>
              void edit.run(async () => {
                await api.linkCatalog(seriesID, value.id, Number(id))
                await done()
              })
            }
          >
            Link edition
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
