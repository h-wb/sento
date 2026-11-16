import { Button, Card, Group, Stack, Text } from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import { api, type SeriesFile } from '../../api'
import { fmtSize } from '../../format'
import { notify } from '../../ui'
import type { BookSelection } from '../BookDialog'
import { useConfirmAction } from '../ConfirmAction'
import { FileScoreDetails } from '../FileScoreDetails'
import { fileName } from '../editionText'
import { fileOrigin } from '../factFlags'

export function BookFiles({ shown, unused, seriesID, sel, busy, confirm, onChanged }: {
  shown: SeriesFile[]
  unused: SeriesFile[]
  seriesID: number
  sel: BookSelection
  busy: boolean
  confirm: ReturnType<typeof useConfirmAction>['confirm']
  onChanged: () => void
}) {
  return (
              <Stack gap="xs">
                {shown.length === 0 && (
                  <Text size="sm" c="dimmed">
                    No file for this book.
                  </Text>
                )}
                {shown.map((f) => (
                  <Card key={f.id} withBorder padding="xs">
                    <Group justify="space-between" wrap="nowrap">
                      <div style={{ minWidth: 0 }}>
                        <Text size="sm" title={f.path} style={{ overflowWrap: 'anywhere' }}>
                          {fileName(f.path)}
                        </Text>
                        <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                          {fileOrigin(f)} · {fmtSize(f.size)}
                        </Text>
                        {/* Its score below says why. */}
                        {unused.includes(f) && (
                          <Text size="xs" c="orange" mt={2}>
                            On disk, but this edition doesn’t use it.
                          </Text>
                        )}
                        <FileScoreDetails seriesID={seriesID} versionID={sel.versionId} fileID={f.id} />
                      </div>
                      <Button
                        size="compact-xs"
                        color="red"
                        variant="subtle"
                        leftSection={<IconTrash size={14} />}
                        disabled={busy}
                        onClick={() =>
                          confirm({
                            title: 'Delete file from disk?',
                            message: `Delete ${f.path}? This removes the archive from every book and edition that uses it. This cannot be undone. Missing books may download again.`,
                            label: 'Delete file',
                            onConfirm: async () => {
                              await api.deleteFile(f.id)
                              onChanged()
                              notify.ok('File deleted')
                            },
                          })
                        }
                      >
                        Delete
                      </Button>
                    </Group>
                  </Card>
                ))}
              </Stack>
  )
}
