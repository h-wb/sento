import { useState } from 'react'
import { Alert, Button, Group, Modal, Stack, Text, TextInput } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { api, type IndexerView } from '../api'

export function IndexerSearchTest({ indexer, onClose }: { indexer: IndexerView; onClose: () => void }) {
  const [query, setQuery] = useState('')
  const check = useMutation({ mutationFn: (title: string) => api.testIndexerSearch(indexer.id, title) })
  const result = check.data

  return (
    <Modal opened onClose={onClose} title={`Test search · ${indexer.name}`} size="lg">
      <Stack>
        <Text size="sm" c="dimmed">
          Search for a title you know is on this indexer. This checks the feed without downloading anything.
        </Text>
        <form onSubmit={(event) => { event.preventDefault(); check.mutate(query.trim()) }}>
          <Group align="flex-end">
            <TextInput label="Title" value={query} onChange={(event) => setQuery(event.currentTarget.value)}
              maxLength={200} required style={{ flex: 1 }} disabled={check.isPending} />
            <Button type="submit" loading={check.isPending} disabled={!query.trim()}>Test search</Button>
          </Group>
        </form>
        {check.error && <Alert color="red">{check.error.message}</Alert>}
        {result && (
          <Stack gap="sm" aria-live="polite">
            <Text size="sm">Books results for “{result.query}”: {result.booksFound}</Text>
            {result.unfilteredFound !== undefined && (
              <Text size="sm">Unfiltered results: {result.unfilteredFound}</Text>
            )}
            {result.warnings.map((warning) => <Alert color="yellow" key={warning}>{warning}</Alert>)}
            {result.samples.length > 0 ? (
              <>
                <Text size="sm" fw={600}>
                  {result.unfilteredFound !== undefined ? 'Unfiltered samples' : 'Books samples'}
                </Text>
                {result.samples.map((sample, index) => (
                  <div key={`${sample.title}-${index}`}>
                    <Text size="sm">{sample.title}</Text>
                    <Text size="xs" c="dimmed">{sample.categories.join(', ') || 'No categories reported'}</Text>
                  </div>
                ))}
              </>
            ) : <Text size="sm" c="dimmed">No releases found. Try another title.</Text>}
          </Stack>
        )}
      </Stack>
    </Modal>
  )
}
