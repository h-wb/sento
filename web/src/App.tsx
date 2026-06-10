import { AppShell, Group, NavLink, Text, Title } from '@mantine/core'
import {
  IconBooks,
  IconDownload,
  IconPlus,
  IconServerBolt,
  IconWorldSearch,
} from '@tabler/icons-react'
import { Link, Route, Routes, useLocation } from 'react-router-dom'
import LibraryPage from './pages/LibraryPage'
import AddPage from './pages/AddPage'
import SeriesPage from './pages/SeriesPage'
import QueuePage from './pages/QueuePage'
import IndexersPage from './pages/IndexersPage'
import SystemPage from './pages/SystemPage'

const links = [
  { to: '/', label: 'Library', icon: IconBooks },
  { to: '/add', label: 'Add Series', icon: IconPlus },
  { to: '/queue', label: 'Activity', icon: IconDownload },
  { to: '/indexers', label: 'Indexers', icon: IconWorldSearch },
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
          <Route path="/queue" element={<QueuePage />} />
          <Route path="/indexers" element={<IndexersPage />} />
          <Route path="/system" element={<SystemPage />} />
        </Routes>
      </AppShell.Main>
    </AppShell>
  )
}
