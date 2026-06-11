import { AppShell, Group, NavLink, Text, Title } from '@mantine/core'
import {
  IconBooks,
  IconCalendar,
  IconDownload,
  IconFiles,
  IconPlus,
  IconServerBolt,
  IconWorldSearch,
} from '@tabler/icons-react'
import { Link, Route, Routes, useLocation } from 'react-router-dom'
import LibraryPage from './pages/LibraryPage'
import AddPage from './pages/AddPage'
import SeriesPage from './pages/SeriesPage'
import CalendarPage from './pages/CalendarPage'
import QueuePage from './pages/QueuePage'
import IndexersPage from './pages/IndexersPage'
import MediaPage from './pages/MediaPage'
import SystemPage from './pages/SystemPage'

const links = [
  { to: '/', label: 'Library', icon: IconBooks },
  { to: '/add', label: 'Add Series', icon: IconPlus },
  { to: '/calendar', label: 'Calendar', icon: IconCalendar },
  { to: '/queue', label: 'Activity', icon: IconDownload },
  { to: '/indexers', label: 'Indexers', icon: IconWorldSearch },
  { to: '/media', label: 'Media', icon: IconFiles },
  { to: '/system', label: 'System', icon: IconServerBolt },
]

export default function App() {
  const location = useLocation()
  return (
    <AppShell header={{ height: 56 }} navbar={{ width: 220, breakpoint: 'sm' }} padding="md">
      <AppShell.Header>
        <Group h="100%" px="md">
          <Title order={3} style={{ letterSpacing: '0.2em' }}>
            sento
          </Title>
          <Text c="dimmed" size="sm">
            manga automation
          </Text>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar p="xs">
        {links.map((l) => (
          <NavLink
            key={l.to}
            component={Link}
            to={l.to}
            label={l.label}
            leftSection={<l.icon size={18} />}
            active={
              l.to === '/' ? location.pathname === '/' : location.pathname.startsWith(l.to)
            }
          />
        ))}
      </AppShell.Navbar>
      <AppShell.Main>
        <Routes>
          <Route path="/" element={<LibraryPage />} />
          <Route path="/add" element={<AddPage />} />
          <Route path="/series/:id" element={<SeriesPage />} />
          <Route path="/calendar" element={<CalendarPage />} />
          <Route path="/queue" element={<QueuePage />} />
          <Route path="/indexers" element={<IndexersPage />} />
          <Route path="/media" element={<MediaPage />} />
          <Route path="/system" element={<SystemPage />} />
        </Routes>
      </AppShell.Main>
    </AppShell>
  )
}
