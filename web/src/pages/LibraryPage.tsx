import { useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Center,
  Group,
  Image,
  SegmentedControl,
  SimpleGrid,
  Skeleton,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { IconBooks, IconSearch } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { usePageTitle } from '../ui'

export default function LibraryPage() {
  usePageTitle('Library')
  const { data: series, error, isPending } = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const [filter, setFilter] = useState('')
  const [sort, setSort] = useState('title')

  const shown = (series ?? [])
    .filter((s) => s.title.toLowerCase().includes(filter.trim().toLowerCase()))
    .sort((a, b) =>
      sort === 'added' ? b.addedAt.localeCompare(a.addedAt) : a.sortTitle.localeCompare(b.sortTitle),
    )

  return (
    <>
      <Group justify="space-between" mb="md" wrap="wrap">
        <Title order={2}>
          Library{' '}
          {series && series.length > 0 && (
            <Text span c="dimmed" size="lg">
              ({series.length})
            </Text>
          )}
        </Title>
        {series && series.length > 0 && (
          <Group gap="xs">
            <TextInput
              size="xs"
              placeholder="Filter…"
              leftSection={<IconSearch size={14} />}
              value={filter}
              onChange={(e) => setFilter(e.currentTarget.value)}
              w={180}
            />
            <SegmentedControl
              size="xs"
              data={[
                { value: 'title', label: 'Title' },
                { value: 'added', label: 'Recent' },
              ]}
              value={sort}
              onChange={setSort}
            />
          </Group>
        )}
      </Group>
      {error && <Alert color="red">{String(error)}</Alert>}

      {isPending && (
        <SimpleGrid cols={{ base: 2, sm: 4, lg: 6 }}>
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} height={280} radius="sm" />
          ))}
        </SimpleGrid>
      )}

      {series && series.length === 0 && (
        <Center mih={300}>
          <Stack align="center" gap="xs">
            <IconBooks size={42} opacity={0.4} />
            <Text c="dimmed">Your library is empty.</Text>
            <Button component={Link} to="/add" variant="light">
              Add your first series
            </Button>
          </Stack>
        </Center>
      )}
      {series && series.length > 0 && shown.length === 0 && (
        <Text c="dimmed">No series match “{filter}”.</Text>
      )}

      <SimpleGrid cols={{ base: 2, sm: 4, lg: 6 }}>
        {shown.map((s) => (
          <Card
            key={s.id}
            component={Link}
            to={`/series/${s.id}`}
            padding="xs"
            withBorder
            style={{ textDecoration: 'none' }}
            opacity={s.monitored ? 1 : 0.55}
          >
            <Card.Section>
              <Image
                src={s.coverUrl}
                height={220}
                alt={s.title}
                fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E"
              />
            </Card.Section>
            <Text fw={600} size="sm" mt="xs" lineClamp={2}>
              {s.title}
            </Text>
            <Group gap={4} mt={4}>
              <Badge size="xs" variant="light">
                {s.status}
              </Badge>
              {s.finalVolume > 0 && (
                <Badge size="xs" variant="outline">
                  {s.finalVolume} vol
                </Badge>
              )}
              {!s.monitored && (
                <Badge size="xs" color="gray" variant="outline">
                  unmonitored
                </Badge>
              )}
            </Group>
          </Card>
        ))}
      </SimpleGrid>
    </>
  )
}
