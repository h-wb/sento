import {
  Alert,
  Badge,
  Card,
  Group,
  Image,
  SimpleGrid,
  Text,
  Title,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../api'

export default function LibraryPage() {
  const { data: series, error } = useQuery({ queryKey: ['series'], queryFn: api.listSeries })

  return (
    <>
      <Title order={2} mb="md">
        Library
      </Title>
      {error && <Alert color="red">{String(error)}</Alert>}
      {series && series.length === 0 && (
        <Text c="dimmed">Nothing here yet — add a series to get started.</Text>
      )}
      <SimpleGrid cols={{ base: 2, sm: 4, lg: 6 }}>
        {series?.map((s) => (
          <Card
            key={s.id}
            component={Link}
            to={`/series/${s.id}`}
            padding="xs"
            withBorder
            style={{ textDecoration: 'none' }}
          >
            <Card.Section>
              <Image src={s.coverUrl} height={220} alt={s.title} fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E" />
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
            </Group>
          </Card>
        ))}
      </SimpleGrid>
    </>
  )
}
