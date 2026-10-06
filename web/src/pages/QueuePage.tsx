import ReviewInbox from '../components/ReviewInbox'
import { useState } from 'react'
import {
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Progress,
  Stack,
  Table,
  Tabs,
  Text,
  Tooltip,
} from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { ErrorDetails } from '../components/ErrorDetails'
import { IconBan, IconDownload, IconRefresh } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { fmtWhen, notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { useConfirmAction } from '../components/ConfirmAction'

const statusColor: Record<string, string> = {
  queued: 'gray',
  downloading: 'blue',
  importing: 'cyan',
  imported: 'green',
  failed: 'red',
  needs_review: 'orange',
  retry_wait: 'yellow',
  dispatch_unknown: 'orange',
  cancelled: 'gray',
}
const statusLabel: Record<string, string> = {
  queued: 'Queued',
  downloading: 'Downloading',
  importing: 'Importing',
  imported: 'Imported',
  failed: 'Failed',
  needs_review: 'Needs review',
  retry_wait: 'Retry scheduled',
  dispatch_unknown: 'Confirm download',
  cancelled: 'Cancelled',
}

export default function QueuePage() {
  usePageTitle('Activity')
  const small = useMediaQuery('(max-width: 48em)')
  const [tab, setTab] = useState<string | null>('queue')
  const queryClient = useQueryClient()
  const { confirm, confirmation } = useConfirmAction()
  const queue = useQuery({
    queryKey: ['queue', true],
    queryFn: () => api.queue(true),
    refetchInterval: 5000,
  })
  const blocked = useQuery({ queryKey: ['blocklist'], queryFn: api.blocklist })
  const items = queue.data ?? []
  const active = items.filter((i) => i.status !== 'imported' && i.status !== 'cancelled')
  const completed = items.filter((i) => i.status === 'imported' || i.status === 'cancelled')
  const shown = tab === 'history' ? completed : active
  const failed = active.filter((i) => i.status === 'failed').length
  const process = useMutation({
    mutationFn: api.processQueue,
    onSuccess: (r) => notify.ok(`${r.processed} ${r.processed === 1 ? 'item' : 'items'} imported`),
    onError: notify.err,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      void queryClient.invalidateQueries({ queryKey: ['series'] })
    },
  })
  const block = useMutation({
    mutationFn: (qid: number) => api.blocklistQueueItem(qid),
    onSuccess: () => notify.ok('Release blocked'),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      void queryClient.invalidateQueries({ queryKey: ['blocklist'] })
    },
  })
  const unblock = useMutation({
    mutationFn: (id: number) => api.unblock(id),
    onSuccess: () => notify.ok('Release unblocked'),
    onError: notify.err,
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['blocklist'] }),
  })
  return (
    <>
      <PageHeader
        title="Activity"
        description="Downloads and imports."
        action={
          <Tooltip label="Check finished downloads and import available files. Downloads are also checked automatically.">
            <Button
              size="sm"
              variant="default"
              leftSection={<IconRefresh size={15} />}
              onClick={() => process.mutate()}
              loading={process.isPending}
            >
              Check downloads
            </Button>
          </Tooltip>
        }
      />
      <Tabs value={tab} onChange={setTab}>
        <Tabs.List mb="lg">
          <Tabs.Tab value="queue">
            Queue <span className="tabCount">{active.length}</span>
          </Tabs.Tab>
          <Tabs.Tab value="review">Review</Tabs.Tab>
          <Tabs.Tab value="history">
            Completed <span className="tabCount">{completed.length}</span>
          </Tabs.Tab>
          <Tabs.Tab value="blocked">
            Blocked releases <span className="tabCount">{blocked.data?.length ?? 0}</span>
          </Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel value="queue">
          {failed > 0 && (
            <Text size="sm" c="red" mb="md">
              {failed} {failed === 1 ? 'download needs' : 'downloads need'} attention. Review the
              error below or block the release.
            </Text>
          )}
        </Tabs.Panel>
        {tab !== 'blocked' && tab !== 'review' && (
          <>
            <QueryFeedback
              pending={queue.isPending}
              error={queue.error}
              onRetry={() => void queue.refetch()}
            />
            {!queue.isPending && !queue.error && shown.length === 0 && (
              <EmptyState
                icon={<IconDownload size={26} stroke={1.5} />}
                title={tab === 'history' ? 'No completed imports' : 'No downloads in progress'}
                description={
                  tab === 'history'
                    ? 'Finished imports will appear here.'
                    : 'Releases you download will appear here. Sento imports them when they finish.'
                }
                action={
                  tab === 'queue' ? (
                    <Button component={Link} to="/" variant="default" size="sm">
                      Browse library
                    </Button>
                  ) : undefined
                }
              />
            )}
            {shown.length > 0 && (
              <>
                {small ? (
                  <Stack gap="sm">
                    {shown.map((i) => (
                      <Card key={i.id} padding="md">
                        <Group justify="space-between" mb="xs">
                          <Badge size="sm" variant="dot" color={statusColor[i.status] ?? 'gray'}>
                            {statusLabel[i.status] ?? i.status}
                          </Badge>
                          <Text size="xs" c="dimmed">
                            {i.protocol === 'source' ? 'Source' : 'Torrent'} ·{' '}
                            {fmtWhen(i.createdAt)}
                          </Text>
                        </Group>
                        <Anchor
                          component={Link}
                          to={`/series/${i.seriesId}`}
                          size="sm"
                          fw={500}
                          c="inherit"
                          underline="hover"
                          lineClamp={3}
                          title={i.title}
                          style={{ overflowWrap: 'anywhere' }}
                        >
                          {i.title}
                        </Anchor>
                        {i.status === 'downloading' && i.progress != null && (
                          <Group gap="sm" wrap="nowrap" mt="md">
                            <Progress
                              value={Math.max(0, Math.min(100, i.progress * 100))}
                              size={4}
                              style={{ flex: 1 }}
                              aria-label={`Download progress for ${i.title}`}
                            />
                            <Text size="xs" c="dimmed">
                              {Math.round(Math.max(0, Math.min(1, i.progress)) * 100)}%
                            </Text>
                          </Group>
                        )}
                        {i.error && (
                          <div style={{ marginTop: 12 }}>
                            <ErrorDetails error={i.error} title="Download error" />
                          </div>
                        )}
                        {i.status === 'failed' && (
                          <Button
                            size="xs"
                            variant="default"
                            mt="sm"
                            onClick={() =>
                              confirm({
                                title: 'Block this release?',
                                message: `“${i.title}” will be excluded from future downloads. You can unblock it later.`,
                                label: 'Block release',
                                onConfirm: () => block.mutateAsync(i.id),
                              })
                            }
                          >
                            Block release
                          </Button>
                        )}
                      </Card>
                    ))}
                  </Stack>
                ) : (
                  <Table
                    highlightOnHover
                    verticalSpacing="md"
                    className="dataTable"
                    style={{ tableLayout: 'fixed' }}
                  >
                    <Table.Thead>
                      <Table.Tr>
                        <Table.Th w="50%">Release</Table.Th>
                        <Table.Th w="12%">Type</Table.Th>
                        <Table.Th w="25%">Status</Table.Th>
                        <Table.Th w="13%">Added</Table.Th>
                      </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                      {shown.map((i) => (
                        <Table.Tr key={i.id}>
                          <Table.Td>
                            <Stack gap={5} style={{ minWidth: 0 }}>
                              <Anchor
                                component={Link}
                                to={`/series/${i.seriesId}`}
                                size="sm"
                                c="inherit"
                                underline="hover"
                                lineClamp={2}
                                title={i.title}
                                style={{ overflowWrap: 'anywhere' }}
                              >
                                {i.title}
                              </Anchor>
                              {i.error && <ErrorDetails error={i.error} title="Download error" />}
                              {i.status === 'failed' && (
                                <Button
                                  size="compact-xs"
                                  variant="subtle"
                                  color="red"
                                  style={{ alignSelf: 'flex-start' }}
                                  onClick={() =>
                                    confirm({
                                      title: 'Block this release?',
                                      message: `“${i.title}” will be excluded from future downloads. You can unblock it later.`,
                                      label: 'Block release',
                                      onConfirm: () => block.mutateAsync(i.id),
                                    })
                                  }
                                >
                                  Block release
                                </Button>
                              )}
                            </Stack>
                          </Table.Td>
                          <Table.Td>
                            <Text size="xs" c="dimmed">
                              {i.protocol === 'source' ? 'Source' : 'Torrent'}
                            </Text>
                          </Table.Td>
                          <Table.Td>
                            <Stack gap={8}>
                              <Badge
                                size="sm"
                                variant="dot"
                                color={statusColor[i.status] ?? 'gray'}
                              >
                                {statusLabel[i.status] ?? i.status}
                              </Badge>
                              {i.status === 'downloading' && i.progress != null && (
                                <Group gap={8} wrap="nowrap" maw={160}>
                                  <Progress
                                    value={Math.max(0, Math.min(100, i.progress * 100))}
                                    size={4}
                                    style={{ flex: 1 }}
                                    aria-label={`Download progress for ${i.title}`}
                                  />
                                  <Text size="xs" c="dimmed" className="tabnums">
                                    {Math.round(Math.max(0, Math.min(1, i.progress)) * 100)}%
                                  </Text>
                                </Group>
                              )}
                            </Stack>
                          </Table.Td>
                          <Table.Td>
                            <Text size="xs" c="dimmed" title={i.createdAt}>
                              {fmtWhen(i.createdAt)}
                            </Text>
                          </Table.Td>
                        </Table.Tr>
                      ))}
                    </Table.Tbody>
                  </Table>
                )}
                {tab === 'history' && (
                  <Text size="xs" c="dimmed" mt="sm">
                    Recent imports from the latest 200 queue items.
                  </Text>
                )}
              </>
            )}
          </>
        )}
        <Tabs.Panel value="review">
          <ReviewInbox />
        </Tabs.Panel>
        <Tabs.Panel value="blocked">
          <QueryFeedback
            pending={blocked.isPending}
            error={blocked.error}
            onRetry={() => void blocked.refetch()}
          />
          {blocked.data?.length === 0 && !blocked.error && (
            <EmptyState
              icon={<IconBan size={26} stroke={1.5} />}
              title="No blocked releases"
              description="Block a failed release to prevent Sento from downloading it again."
            />
          )}
          {(blocked.data?.length ?? 0) > 0 && (
            <Stack gap="sm">
              {blocked.data?.map((b) => (
                <Card key={b.id} padding="md">
                  <Group justify="space-between" align="flex-start" wrap="nowrap">
                    <Stack gap={4} style={{ minWidth: 0, flex: 1 }}>
                      <Text size="sm" style={{ overflowWrap: 'anywhere' }}>
                        {b.key}
                      </Text>
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        {b.reason}
                      </Text>
                      <Text size="xs" c="dimmed">
                        {b.protocol === 'source' ? 'Source' : 'Torrent'}
                      </Text>
                    </Stack>
                    <Button
                      size="xs"
                      variant="default"
                      loading={unblock.isPending && unblock.variables === b.id}
                      disabled={unblock.isPending}
                      onClick={() => unblock.mutate(b.id)}
                    >
                      Unblock
                    </Button>
                  </Group>
                </Card>
              ))}
            </Stack>
          )}
        </Tabs.Panel>
      </Tabs>
      {confirmation}
    </>
  )
}
