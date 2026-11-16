import { useState } from 'react'
import { Button, Group, PasswordInput, Select, Stack, Text, TextInput } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { api, type ProwlarrIndexer } from '../../api'
import { notify } from '../../ui'

// Prowlarr browse assist for torznab entries.
export function ProwlarrBrowse({
  active,
  apiKey,
  onApply,
}: {
  // Whether the torznab protocol is chosen; the browse keeps its state while not.
  active: boolean
  apiKey: string
  onApply: (pick: { url: string; apiKey: string }, name: string) => void
}) {
  const [browse, setBrowse] = useState<{ url: string; apiKey: string } | null>(null)
  const [found, setFound] = useState<ProwlarrIndexer[] | null>(null)
  const browseMut = useMutation({
    mutationFn: (b: { url: string; apiKey: string }) => api.browseProwlarr(b),
    onSuccess: setFound,
    onError: notify.err,
  })

  if (!active) return null
  return (
    <>
      {!browse && (
        <Button
          variant="subtle"
          size="compact-sm"
          style={{ alignSelf: 'flex-start' }}
          onClick={async () => {
            // The Prowlarr Sento already knows (seeded by the
            // deployment or remembered from the last browse): list
            // its indexers at once instead of asking again.
            const known = (await api.connections().catch(() => null))?.prowlarr
            const start = { url: known?.url ?? '', apiKey: known?.apiKey || apiKey }
            setBrowse(start)
            if (start.url && start.apiKey) browseMut.mutate(start)
          }}
        >
          Choose from Prowlarr…
        </Button>
      )}
      {browse && (
        <Stack
          gap="xs"
          p="xs"
          style={{ border: '1px dashed var(--mantine-color-gray-6)', borderRadius: 6 }}
        >
          <Text size="xs" c="dimmed">
            Pick an indexer from a Prowlarr — the per-indexer Torznab URL and key fill in.
          </Text>
          <Group grow>
            <TextInput
              size="xs"
              placeholder="Prowlarr URL"
              value={browse.url}
              onChange={(e) => setBrowse({ ...browse, url: e.currentTarget.value })}
            />
            <PasswordInput
              size="xs"
              placeholder="API key"
              value={browse.apiKey}
              onChange={(e) => setBrowse({ ...browse, apiKey: e.currentTarget.value })}
            />
          </Group>
          <Button
            size="compact-sm"
            variant="default"
            loading={browseMut.isPending}
            disabled={!browse.url}
            onClick={() => browseMut.mutate(browse)}
          >
            List indexers
          </Button>
          {found && (
            <Select
              size="xs"
              placeholder={`${found.length} indexers — pick one`}
              data={found.map((f) => ({ value: f.url, label: f.name }))}
              searchable
              onChange={(url) => {
                const pick = found.find((f) => f.url === url)
                if (!pick) return
                onApply({ url: pick.url, apiKey: browse.apiKey }, pick.name)
              }}
            />
          )}
        </Stack>
      )}
    </>
  )
}
