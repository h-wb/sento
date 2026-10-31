import { useState } from 'react'
import { Alert, Button, Checkbox, Group, NumberInput, Select, Stack, Text, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import type { Edition, Book } from '../types/store'
import { useConfirmAction } from './ConfirmAction'
import { Origin } from './Origin'
import { EditorModal } from './EditorModal'
import { useEditor } from './useEditor'
import { edited } from './editionText'

// What MangaBaka lists behind a book: one copy per medium, each with its
// own ISBN and date. The book shows the earliest date and the preferred
// copy's ISBN, and this says which.
function BookCopies({
  seriesID,
  book,
  edition,
}: {
  seriesID: number
  book: Book
  edition?: Edition
}) {
  const works = useQuery({
    queryKey: ['series', seriesID, 'book-works', book.id],
    queryFn: () => api.bookWorks(seriesID, book.id),
    enabled: book.id > 0 && book.copies > 0,
  })
  if (!edition || book.id === 0) return null
  if (edition.custom)
    return (
      <Group gap="xs">
        <Origin kind="local" />
        <Text size="sm" c="dimmed">
          Part of a local edition. MangaBaka does not update it.
        </Text>
      </Group>
    )
  if (book.copies === 0)
    return (
      <Group gap="xs">
        <Origin kind="local" />
        <Text size="sm" c="dimmed">
          MangaBaka does not list this book in this edition.
        </Text>
      </Group>
    )
  const copies = works.data ?? []
  const absent = [...new Set(edition.collections.map((c) => c.medium))].filter(
    (m) => !copies.some((w) => w.medium === m && w.syncState === 'linked'),
  )
  return (
    <Stack gap={6} aria-label="Copies listed in MangaBaka">
      <Group gap="xs">
        <Origin kind="catalog" compact />
        <Text size="sm" fw={600}>
          Listed in MangaBaka
        </Text>
        {book.syncState === 'stale' && <Origin kind="missing" />}
      </Group>
      {copies.map((w, i) => (
        <Text key={w.id} size="sm" c={w.syncState === 'linked' ? undefined : 'dimmed'}>
          <b>{w.medium || 'unknown medium'}</b>
          {[
            w.publisher,
            w.isbn && `ISBN ${w.isbn}`,
            w.releaseDate,
            w.pages ? `${w.pages} pages` : '',
            w.syncState !== 'linked' && 'no longer listed',
            i === 0 && copies.length > 1 && 'ISBN and pages shown',
          ]
            .filter(Boolean)
            .map((part) => ` · ${part}`)
            .join('')}
        </Text>
      ))}
      {works.isSuccess && absent.length > 0 && (
        <Text size="sm" c="dimmed">
          Not listed in {absent.join(' or ')}.
        </Text>
      )}
      {copies.length > 1 && (
        <Text size="xs" c="dimmed">
          The date shown is the earliest of these copies.
        </Text>
      )}
    </Stack>
  )
}

export function BookEditor({
  value,
  seriesID,
  edition,
  save,
  remove,
  reset,
  close,
}: {
  value: Book
  seriesID: number
  edition?: Edition
  save: (v: Book) => Promise<void>
  remove: () => Promise<void>
  reset: () => Promise<void>
  close: () => void
}) {
  const [v, set] = useState(value)
  const edit = useEditor()
  const { confirm, confirmation } = useConfirmAction()
  return (
    <>
      <EditorModal
        dirty={JSON.stringify(v) !== JSON.stringify(value)}
        busy={edit.busy}
        onClose={close}
        title={v.id ? 'Edit book' : 'Add book'}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void edit.run(() => save(v))
          }}
        >
          <Stack>
            <BookCopies seriesID={seriesID} book={value} edition={edition} />
            <Text size="sm" c="dimmed">
              Numbers identify books within this edition. Specials can use a label without a number.
              {value.copies > 0 && ' What you change here is kept when MangaBaka is refreshed.'}
            </Text>
            <fieldset className="formFields" disabled={edit.busy}>
              <Stack>
                <TextInput
                  label="Display label"
                  description={edited(value, 'sequence_string')}
                  required
                  value={v.sequenceStr}
                  onChange={(e) => set({ ...v, sequenceStr: e.currentTarget.value })}
                />
                <Checkbox
                  label="Has a volume or chapter number"
                  checked={v.numberKnown}
                  onChange={(e) => set({ ...v, numberKnown: e.currentTarget.checked })}
                />
                {v.numberKnown && (
                  <NumberInput
                    label="Number"
                    decimalScale={2}
                    min={0}
                    value={v.sequenceNum / 100}
                    onChange={(n) => set({ ...v, sequenceNum: Math.round(Number(n) * 100) })}
                  />
                )}
                <Select
                  label="Category"
                  description="Regular books are collected by default. Enable extras in edition settings to include bonuses."
                  data={[
                    { value: 'main', label: 'Regular' },
                    { value: 'extra', label: 'Bonus / special' },
                    { value: 'other', label: 'Other' },
                  ]}
                  value={v.countType}
                  onChange={(s) => set({ ...v, countType: s ?? 'main' })}
                />
                <TextInput
                  type="date"
                  label="Publication date"
                  description={edited(value, 'release_date')}
                  value={v.releaseDate ?? ''}
                  onChange={(e) => set({ ...v, releaseDate: e.currentTarget.value })}
                />
                <TextInput
                  label="ISBN"
                  description={edited(value, 'isbn')}
                  value={v.isbn ?? ''}
                  onChange={(e) => set({ ...v, isbn: e.currentTarget.value })}
                />
                <TextInput
                  label="Revision"
                  description={edited(value, 'revision')}
                  value={v.revision ?? ''}
                  onChange={(e) => set({ ...v, revision: e.currentTarget.value })}
                />
              </Stack>
            </fieldset>
            {edit.error && <Alert color="red">{edit.error}</Alert>}
            <Group justify="space-between">
              {v.providerId && (
                <Button variant="subtle" disabled={edit.busy} onClick={() => void edit.run(reset)}>
                  Use MangaBaka’s values
                </Button>
              )}
              {v.id > 0 && (
                <Button
                  color="red"
                  variant="subtle"
                  onClick={() =>
                    confirm({
                      title: v.providerId ? 'Retire catalog book?' : 'Delete book?',
                      message: 'Owned books must be unlinked first. Files stay on disk.',
                      label: v.providerId ? 'Retire book' : 'Delete book',
                      onConfirm: remove,
                    })
                  }
                >
                  {v.providerId ? 'Retire book' : 'Delete book'}
                </Button>
              )}
              <Group>
                <Button type="submit" loading={edit.busy}>
                  Save book
                </Button>
              </Group>
            </Group>
          </Stack>
        </form>
      </EditorModal>
      {confirmation}
    </>
  )
}
