import { ActionIcon, Anchor, Badge, Card, Group, Stack, Table, Text, ThemeIcon, Title, Tooltip } from '@mantine/core'
import { BarChart } from '@mantine/charts'
import { IconAlertTriangle, IconCircleCheck, IconCircleX, IconRefresh } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type TaskStatus } from '../api'
import { fmtUntil, fmtWhen, notify, usePageTitle } from '../ui'
import { PageHeader } from '../components/PageHeader'
import { QueryFeedback } from '../components/QueryFeedback'

function fmtInterval(secs: number): string {
  if (secs >= 86400) return `${Math.round(secs / 86400)} day${secs >= 2 * 86400 ? 's' : ''}`
  if (secs >= 3600) return `${Math.round(secs / 3600)} hour${secs >= 2 * 3600 ? 's' : ''}`
  return `${Math.round(secs / 60)} minute${secs >= 120 ? 's' : ''}`
}

function fmtDuration(ms: number): string {
  if (ms <= 0) return '—'
  if (ms < 1000) return `${ms} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`
}

export default function SystemPage() {
  usePageTitle('System')
  const queryClient = useQueryClient()
  const { data: status, error, isPending, refetch } = useQuery({ queryKey: ['status'], queryFn: api.status })
  // Schedule state moves while you watch (running flags, next runs).
  const { data: tasks, error: tasksError, isPending: tasksPending, refetch: refetchTasks } = useQuery({
    queryKey: ['tasks'],
    queryFn: api.systemTasks,
    refetchInterval: 10_000,
  })

  // Shared with the navigation badge, which keeps it polled.
  const { data: health, error: healthError, isPending: healthPending, refetch: refetchHealth } = useQuery({
    queryKey: ['health'],
    queryFn: api.healthChecks,
  })
  const recheck = useMutation({
    mutationFn: api.runHealthChecks,
    onSuccess: (report) => queryClient.setQueryData(['health'], report),
    onError: notify.err,
  })

  const runTask = useMutation({
    mutationFn: (t: TaskStatus) => api.runTask(t.slug),
    onSuccess: (_d, t) => {
      notify.ok(`${t.name} triggered`)
      setTimeout(() => queryClient.invalidateQueries({ queryKey: ['tasks'] }), 500)
    },
    onError: notify.err,
  })

  // Last-run durations, longest first — a quick read on which loops are heavy.
  const durations = (tasks ?? [])
    .filter((t) => t.lastDurationMs > 0)
    .sort((a, b) => b.lastDurationMs - a.lastDurationMs)
    .map((t) => ({ task: t.name, ms: t.lastDurationMs }))

  return (
    <>
      <PageHeader title="System" description="Service status and background tasks." />
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {status && (
        <Card withBorder maw={420} mb="lg">
          <Group justify="space-between">
            <Text>Version</Text>
            <Badge variant="light">{status.version}</Badge>
          </Group>
          <Group justify="space-between" mt="xs">
            <Text>Database</Text>
            <Badge color={status.database.ok ? 'green' : 'red'}>
              {status.database.ok ? 'ok' : 'down'}
            </Badge>
          </Group>
        </Card>
      )}

      <Group justify="space-between" maw={900} mb="xs">
        <Title order={4}>Health</Title>
        <Group gap="xs">
          {health && (
            <Text size="xs" c="dimmed" title={health.checkedAt}>
              Checked {fmtWhen(health.checkedAt)}
            </Text>
          )}
          <Tooltip label="Check again">
            <ActionIcon
              size="sm"
              variant="subtle"
              aria-label="Check health again"
              loading={recheck.isPending}
              onClick={() => recheck.mutate()}
            >
              <IconRefresh size={14} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Group>
      <QueryFeedback pending={healthPending} error={healthError} onRetry={() => void refetchHealth()} />
      {health && (
        <Card withBorder maw={900} mb="lg">
          {health.issues.length === 0 ? (
            <Group gap="sm" wrap="nowrap">
              <ThemeIcon variant="light" color="green" size="md" radius="xl">
                <IconCircleCheck size={16} />
              </ThemeIcon>
              <Text size="sm">No issues found. Indexers, download services and the library folder are working.</Text>
            </Group>
          ) : (
            <Stack gap="md">
              {health.issues.map((issue) => (
                <Group key={`${issue.source}:${issue.message}`} gap="sm" wrap="nowrap" align="flex-start">
                  <ThemeIcon variant="light" color={issue.level === 'error' ? 'red' : 'yellow'} size="md" radius="xl">
                    {issue.level === 'error' ? <IconCircleX size={16} /> : <IconAlertTriangle size={16} />}
                  </ThemeIcon>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Text size="sm">{issue.message}</Text>
                    {issue.detail && (
                      <Text size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        {issue.detail}
                      </Text>
                    )}
                  </div>
                  {issue.link && issue.link !== '/system' && (
                    <Anchor component={Link} to={issue.link} size="sm" style={{ whiteSpace: 'nowrap' }}>
                      Fix
                    </Anchor>
                  )}
                </Group>
              ))}
            </Stack>
          )}
        </Card>
      )}

      {durations.length > 0 && (
        <Card withBorder maw={900} mb="lg">
          <Text size="sm" fw={600} mb="md">
            Last run duration (ms)
          </Text>
          <BarChart
            h={Math.max(160, durations.length * 38)}
            data={durations}
            dataKey="task"
            orientation="vertical"
            series={[{ name: 'ms', color: 'brand.5' }]}
            withTooltip
            gridAxis="none"
          />
        </Card>
      )}

      <Title order={4} mb="xs">
        Scheduled tasks
      </Title>
      <Text size="sm" c="dimmed" mb="sm">
        Tasks run automatically on this schedule. Use the run button to start one now.
      </Text>
      <QueryFeedback pending={tasksPending} error={tasksError} onRetry={() => void refetchTasks()} />
      <Table.ScrollContainer minWidth={680}><Table highlightOnHover verticalSpacing="sm" maw={900}>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Name</Table.Th>
            <Table.Th>Interval</Table.Th>
            <Table.Th>Last Execution</Table.Th>
            <Table.Th>Last Duration</Table.Th>
            <Table.Th>Next Execution</Table.Th>
            <Table.Th w={50} />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {tasks?.map((t) => (
            <Table.Tr key={t.slug}>
              <Table.Td>
                <Group gap={6} wrap="nowrap">
                  <Text size="sm">{t.name}</Text>
                  {t.lastError && (
                    <Tooltip label={t.lastError} multiline maw={360}>
                      <Badge size="xs" color="red" variant="light">
                        failed
                      </Badge>
                    </Tooltip>
                  )}
                </Group>
              </Table.Td>
              <Table.Td>
                <Text size="sm" c="dimmed">
                  {fmtInterval(t.intervalSec)}
                </Text>
              </Table.Td>
              <Table.Td>
                <Text size="sm" c="dimmed" title={t.lastRun}>
                  {t.lastRun ? fmtWhen(t.lastRun) : 'not yet'}
                </Text>
              </Table.Td>
              <Table.Td>
                <Text size="sm" c="dimmed">
                  {fmtDuration(t.lastDurationMs)}
                </Text>
              </Table.Td>
              <Table.Td>
                <Text size="sm" c="dimmed" title={t.nextRun}>
                  {t.running ? 'running…' : fmtUntil(t.nextRun ?? '')}
                </Text>
              </Table.Td>
              <Table.Td>
                <Tooltip label={t.running ? 'Running' : 'Run now'}>
                  <ActionIcon
                    size="sm"
                    variant="subtle"
                    aria-label={`Run ${t.name}`}
                    loading={t.running || (runTask.isPending && runTask.variables?.slug === t.slug)}
                    disabled={runTask.isPending}
                    onClick={() => runTask.mutate(t)}
                  >
                    <IconRefresh size={14} />
                  </ActionIcon>
                </Tooltip>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table></Table.ScrollContainer>
      {tasks && tasks.length === 0 && (
        <Text size="sm" c="dimmed">
          No scheduler is running.
        </Text>
      )}
    </>
  )
}
