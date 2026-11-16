import { Button, Group, Modal, NumberInput, Stack, Text } from '@mantine/core'
import type { Dispatch, SetStateAction } from 'react'
import type { Book } from '../../types/store'
import { api } from '../../api'

export function SplitBoxDialog({ split, setSplit, busy, run, seriesID }: {
  split: { book: Book; count: string | number }
  setSplit: Dispatch<SetStateAction<{ book: Book; count: string | number } | null>>
  busy: boolean
  run: (fn: () => Promise<unknown>) => Promise<void>
  seriesID: number
}) {
  return (
        <Modal
          opened
          onClose={() => setSplit(null)}
          title={`Split "${split.book.sequenceStr}" into volumes`}
        >
          <Stack gap="sm">
            <Text size="sm" c="dimmed">
              Catalogs list a box set as one item, but releases are named after the books inside it.
              This adds volumes 1 to N to this edition, with the box's ISBN and date, and stops
              collecting the box itself.
            </Text>
            <NumberInput
              label="Volumes in the box"
              min={2}
              max={100}
              value={split.count}
              onChange={(count) => setSplit({ ...split, count })}
            />
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setSplit(null)}>
                Cancel
              </Button>
              <Button
                disabled={busy || Number(split.count) < 2}
                onClick={() =>
                  void run(() => api.splitBox(seriesID, split.book.id, Number(split.count))).then(
                    () => setSplit(null),
                  )
                }
              >
                Add volumes
              </Button>
            </Group>
          </Stack>
        </Modal>
  )
}
