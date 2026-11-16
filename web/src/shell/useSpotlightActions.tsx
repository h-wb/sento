import { Avatar } from '@mantine/core'
import type { SpotlightActionData } from '@mantine/spotlight'
import { IconPlus } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { seriesQueries } from '../queries'
import { configLinks,primaryLinks,systemLink,type NavItem } from './navigation'

export function useSpotlightActions(openAdd: () => void) {
  const navigate = useNavigate()
  // Cached library list, shared with LibraryPage — powers ⌘K series search.
  const seriesList = useQuery(seriesQueries.list())
  const toAction = (l: NavItem, group: string): SpotlightActionData => ({
    id: l.to,
    label: l.label,
    group,
    leftSection: <l.icon size={18} />,
    onClick: () => navigate(l.to),
  })
  const spotlightActions: SpotlightActionData[] = [
    {
      id: 'add-series',
      label: 'Add series',
      group: 'Actions',
      leftSection: <IconPlus size={18} />,
      onClick: openAdd,
    },
    ...primaryLinks.map((l) => toAction(l, 'Pages')),
    ...configLinks.map((l) => toAction(l, 'Configure')),
    toAction(systemLink, 'Pages'),
    ...(seriesList.data ?? []).map(
      (s): SpotlightActionData => ({
        id: `series-${s.id}`,
        label: s.title,
        description: [s.type, s.status].filter(Boolean).join(' · ') || undefined,
        group: 'Library',
        leftSection: (
          <Avatar src={s.coverUrl} size="sm" radius="sm">
            {s.title.slice(0, 1)}
          </Avatar>
        ),
        onClick: () => navigate(`/series/${s.id}`),
      }),
    ),
  ]

  return spotlightActions
}
