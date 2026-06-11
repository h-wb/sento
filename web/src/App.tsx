import { AppShell, Badge, Burger, Group, Indicator, NavLink, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import {
  IconBooks,
  IconCalendar,
  IconDownload,
  IconFiles,
  IconPlus,
  IconServerBolt,
  IconPlug,
  IconWorldSearch,
} from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { Link, Route, Routes, useLocation } from 'react-router-dom'
import { api } from './api'
import LibraryPage from './pages/LibraryPage'
import AddPage from './pages/AddPage'
import SeriesPage from './pages/SeriesPage'
import CalendarPage from './pages/CalendarPage'
import QueuePage from './pages/QueuePage'
import IndexersPage from './pages/IndexersPage'
import SourcesPage from './pages/SourcesPage'
import MediaPage from './pages/MediaPage'
import SystemPage from './pages/SystemPage'

const links = [
  { to: '/', label: 'Library', icon: IconBooks },
  { to: '/add', label: 'Add Series', icon: IconPlus },
  { to: '/calendar', label: 'Calendar', icon: IconCalendar },
  { to: '/queue', label: 'Activity', icon: IconDownload },
  { to: '/sources', label: 'Sources', icon: IconPlug },
  { to: '/indexers', label: 'Indexers', icon: IconWorldSearch },
  { to: '/media', label: 'Media', icon: IconFiles },
  { to: '/system', label: 'System', icon: IconServerBolt },
]

export default function App() {
  const location = useLocation()
  const [navOpened, { toggle: toggleNav, close: closeNav }] = useDisclosure()

  // Light background polling so the shell can hint at what's going on.
  const status = useQuery({ queryKey: ['status'], queryFn: api.status, refetchInterval: 60_000 })
  const queue = useQuery({
    queryKey: ['queue', 'active-count'],
    queryFn: () => api.queue(false),
    refetchInterval: 15_000,
  })
  const active = queue.data?.length ?? 0
  const healthy = !status.isError && (status.data?.database.ok ?? true)

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 220, breakpoint: 'sm', collapsed: { mobile: !navOpened } }}
      padding="md"
    >
      <AppShell.Header>
        <Group h="100%" px="md">
          <Burger opened={navOpened} onClick={toggleNav} hiddenFrom="sm" size="sm" />
          <Indicator color={healthy ? 'green' : 'red'} size={7} offset={2} position="top-end">
            <Title order={3} style={{ letterSpacing: '0.2em' }}>
              sento
            </Title>
          </Indicator>
          <Text c="dimmed" size="sm" visibleFrom="xs">
            manga automation
          </Text>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar p="xs">
        <AppShell.Section grow>
          {links.map((l) => (
            <NavLink
              key={l.to}
              component={Link}
              to={l.to}
              label={l.label}
              leftSection={<l.icon size={18} />}
              rightSection={
                l.to === '/queue' && active > 0 ? (
                  <Badge size="xs" circle color="blue">
                    {active}
                  </Badge>
                ) : undefined
              }
              active={l.to === '/' ? location.pathname === '/' : location.pathname.startsWith(l.to)}
              onClick={closeNav}
            />
          ))}
        </AppShell.Section>
        <AppShell.Section>
          <Text size="xs" c="dimmed" px="sm" py={4}>
            {status.data ? `v${status.data.version}` : '…'}
            {!healthy && ' · backend unreachable'}
          </Text>
        </AppShell.Section>
      </AppShell.Navbar>
      <AppShell.Main>
        <Routes>
          <Route path="/" element={<LibraryPage />} />
          <Route path="/add" element={<AddPage />} />
          <Route path="/series/:id" element={<SeriesPage />} />
          <Route path="/calendar" element={<CalendarPage />} />
          <Route path="/queue" element={<QueuePage />} />
          <Route path="/sources" element={<SourcesPage />} />
          <Route path="/indexers" element={<IndexersPage />} />
          <Route path="/media" element={<MediaPage />} />
          <Route path="/system" element={<SystemPage />} />
        </Routes>
      </AppShell.Main>
    </AppShell>
  )
}
