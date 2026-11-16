import {
Anchor,
Badge,
Checkbox,
Group,
Image,
Table,
Text,
Tooltip,
UnstyledButton
} from '@mantine/core'
import {
IconChevronDown,
IconChevronUp,
IconEye,
IconEyeOff,
IconSelector
} from '@tabler/icons-react'
import { Link,useNavigate } from 'react-router-dom'
import { fmtWhen } from '../../format'
import type { Series } from '../../types/store'


type SortField = 'sortTitle' | 'type' | 'status' | 'year' | 'finalVolume' | 'totalChapters' | 'addedAt'

export function SeriesTable({ shown, owned, selected, allSelected, someSelected, toggleAll, toggleOne, sortField, sortDir, toggleSort }: { shown: Series[]; owned: (id: number) => { complete: boolean; share: number; label: string; title: string } | null; selected: Set<number>; allSelected: boolean; someSelected: boolean; toggleAll: () => void; toggleOne: (id: number) => void; sortField: SortField; sortDir: 'asc' | 'desc'; toggleSort: (field: SortField) => void }) {
  const navigate = useNavigate()
  const sortTh = (label: string, field: SortField, align: 'left' | 'right' = 'left') => (
    <Table.Th
      onClick={() => toggleSort(field)}
      style={{ cursor: 'pointer', userSelect: 'none' }}
      ta={align}
      aria-sort={sortField === field ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
    >
      <UnstyledButton onClick={(e) => { e.stopPropagation(); toggleSort(field) }} aria-label={`Sort by ${label}`} w="100%"><Group gap={4} wrap="nowrap" justify={align === 'right' ? 'flex-end' : 'flex-start'}>
        {label}
        {sortField === field ? (
          sortDir === 'asc' ? (
            <IconChevronUp size={13} />
          ) : (
            <IconChevronDown size={13} />
          )
        ) : (
          <IconSelector size={13} style={{ opacity: 0.35 }} />
        )}
      </Group></UnstyledButton>
    </Table.Th>
  )

  return (
        <Table.ScrollContainer minWidth={720}>
          <Table highlightOnHover verticalSpacing="xs" stickyHeader>
            <Table.Thead>
              <Table.Tr>
                <Table.Th w={36}>
                  <Checkbox
                    aria-label="Select all"
                    checked={allSelected}
                    indeterminate={someSelected && !allSelected}
                    onChange={toggleAll}
                  />
                </Table.Th>
                {sortTh('Title', 'sortTitle')}
                {sortTh('Type', 'type')}
                {sortTh('Status', 'status')}
                {sortTh('Year', 'year')}
                <Table.Th ta="right">Owned</Table.Th>
                {sortTh('Vol', 'finalVolume', 'right')}
                {sortTh('Chapters', 'totalChapters', 'right')}
                {sortTh('Added', 'addedAt')}
                <Table.Th w={40} ta="center">
                  <Tooltip label="Monitored">
                    <IconEye size={15} style={{ verticalAlign: 'middle' }} />
                  </Tooltip>
                </Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {shown.map((s) => (
                <Table.Tr
                  key={s.id}
                  onClick={() => navigate(`/series/${s.id}`)}
                  style={{ cursor: 'pointer' }}
                  bg={selected.has(s.id) ? 'var(--mantine-color-brand-light)' : undefined}
                  opacity={s.monitored ? 1 : 0.55}
                >
                  <Table.Td onClick={(e) => e.stopPropagation()}>
                    <Checkbox
                      aria-label={`Select ${s.title}`}
                      checked={selected.has(s.id)}
                      onChange={() => toggleOne(s.id)}
                    />
                  </Table.Td>
                  <Table.Td>
                    <Group gap="sm" wrap="nowrap">
                      <Image
                        src={s.coverUrl}
                        w={28}
                        h={40}
                        radius="sm"
                        alt=""
                        fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E"
                      />
                      <Anchor component={Link} to={`/series/${s.id}`} c="inherit" fw={500} size="sm" lineClamp={1} underline="hover">
                        {s.title}
                      </Anchor>
                    </Group>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed" tt="capitalize">
                      {s.type || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" variant="light">
                      {s.status}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed">
                      {s.year || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="right">
                    <Text size="sm" c={owned(s.id)?.complete === false ? undefined : 'dimmed'} title={owned(s.id)?.title}>
                      {owned(s.id)?.label ?? '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="right">
                    <Text size="sm" c="dimmed">
                      {s.finalVolume || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="right">
                    <Text size="sm" c="dimmed">
                      {s.totalChapters || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed" title={s.addedAt}>
                      {fmtWhen(s.addedAt)}
                    </Text>
                  </Table.Td>
                  <Table.Td ta="center">
                    {s.monitored ? (
                      <IconEye size={15} style={{ verticalAlign: 'middle' }} />
                    ) : (
                      <IconEyeOff size={15} style={{ verticalAlign: 'middle', opacity: 0.5 }} />
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
  )
}
