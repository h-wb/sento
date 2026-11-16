import { useState } from 'react'
import { Badge, Button, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { IconLock } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { hostOf, plural } from '../../format'
import type { RepoStatus } from '../../types/extensions'
import { notify } from '../../ui'
import { QueryFeedback } from '../QueryFeedback'

// Repos lists the extension repos with what each one currently yields on
// the host: its name and usable extension count, or why it yields nothing.
// Extensions themselves are driven by indexer entries — adding a source
// installs its extension, deleting the last entry uninstalls it.
export function Repos() {
  const queryClient = useQueryClient()
  const repos = useQuery({ queryKey: ['extension-repos'], queryFn: api.extensionRepos })
  const [newRepo, setNewRepo] = useState('')
  const saveRepos = useMutation({
    mutationFn: (urls: string[]) => api.setExtensionRepos(urls),
    onSuccess: (data) => {
      notify.ok('Extension repos updated')
      queryClient.setQueryData(['extension-repos'], data)
      queryClient.invalidateQueries({ queryKey: ['extensions'] })
    },
    onError: (e, sent) => {
      // Give back what was being added, so it can be corrected.
      const added = sent.find((u) => !urls.includes(u))
      if (added) setNewRepo(added)
      notify.err(e)
    },
  })
  const urls = repos.data?.urls ?? []
  const typed = newRepo.trim()
  const known = urls.includes(typed)
  // Given by the deployment's configuration: shown with their status,
  // not editable.
  const managed = repos.data?.managed ?? false
  const rows: RepoStatus[] = repos.data?.repos ?? urls.map((url) => ({ url, extensions: 0 }))
  // The field empties as the repo is added; an error gives it back.
  const add = () => {
    if (!typed || known || saveRepos.isPending) return
    setNewRepo('')
    saveRepos.mutate([...urls, typed])
  }

  return (
    <>
      <Group gap="xs" mb={4}>
        <Title order={6}>Extension repos</Title>
        {managed && (
          <Badge size="xs" variant="outline" color="gray" leftSection={<IconLock size={10} />}>
            Read-only
          </Badge>
        )}
      </Group>
      <Text size="xs" c="dimmed" mb="sm">
        {managed ? (
          <>
            This list is set by the deployment&apos;s configuration (
            <code>SENTO_EXTENSIONS_REPOS</code>). Change it there and restart Sento. Choose
          </>
        ) : (
          'Add a Mihon-compatible repository, then choose'
        )}{' '}
        its sources on <Link to="/indexers">Indexers</Link>.
      </Text>
      <QueryFeedback
        pending={repos.isPending}
        error={repos.error}
        onRetry={() => void repos.refetch()}
      />
      <Stack gap="xs">
        {repos.data && rows.length === 0 && (
          <Text size="sm" c="dimmed">
            Add a repository to see available sources.
          </Text>
        )}
        {rows.map((repo) => (
          <RepoRow
            key={repo.url}
            repo={repo}
            removing={saveRepos.isPending}
            onRemove={
              managed ? undefined : () => saveRepos.mutate(urls.filter((x) => x !== repo.url))
            }
          />
        ))}
        {!managed && (
          <Group gap={6} wrap="nowrap">
            <TextInput
              size="xs"
              style={{ flex: 1 }}
              aria-label="Extension repo URL"
              placeholder="https://…/index.min.json"
              value={newRepo}
              onChange={(e) => setNewRepo(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') add()
              }}
              error={known ? 'Already added' : undefined}
            />
            <Button
              size="xs"
              variant="default"
              loading={saveRepos.isPending}
              disabled={!typed || known}
              onClick={add}
            >
              Add repo
            </Button>
          </Group>
        )}
      </Stack>
    </>
  )
}

function RepoRow({
  repo,
  removing,
  onRemove,
}: {
  repo: RepoStatus
  removing: boolean
  onRemove?: () => void
}) {
  return (
    <Group gap="sm" wrap="nowrap" justify="space-between" align="flex-start">
      <Stack gap={0} style={{ minWidth: 0, flex: 1 }}>
        <Group gap={8} wrap="nowrap">
          <Text size="sm" fw={600} truncate>
            {repo.name || hostOf(repo.url)}
          </Text>
          {repo.error ? (
            <Badge color="red" variant="light" style={{ flexShrink: 0 }}>
              Failing
            </Badge>
          ) : (
            <Badge
              color={repo.extensions > 0 ? 'green' : 'yellow'}
              variant="light"
              style={{ flexShrink: 0, textTransform: 'none' }}
            >
              {repo.extensions.toLocaleString()} {plural(repo.extensions, 'extension')}
            </Badge>
          )}
        </Group>
        <Text size="xs" c="dimmed" style={{ fontFamily: 'monospace' }} truncate>
          {repo.url}
        </Text>
        {repo.error && (
          <Text size="xs" c="red">
            {repo.error}
          </Text>
        )}
        {!repo.error && repo.extensions === 0 && (
          <Text size="xs" c="dimmed">
            The repo answered but lists nothing this host can run.
          </Text>
        )}
      </Stack>
      {onRemove && (
        <Button
          size="compact-xs"
          variant="subtle"
          color="red"
          disabled={removing}
          onClick={onRemove}
        >
          Remove
        </Button>
      )}
    </Group>
  )
}
