import { lazy } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import LibraryPage from './pages/LibraryPage'

// Lazy-loaded routes land in their own chunks, fetched only when the route
// is visited. Library stays eager since it's the landing page.
const AddPage = lazy(() => import('./pages/AddPage'))
const SeriesPage = lazy(() => import('./pages/SeriesPage'))
const CalendarPage = lazy(() => import('./pages/CalendarPage'))
const QueuePage = lazy(() => import('./pages/QueuePage'))
const WantedPage = lazy(() => import('./pages/WantedPage'))
const IndexersPage = lazy(() => import('./pages/IndexersPage'))
const ConnectionsPage = lazy(() => import('./pages/ConnectionsPage'))
const MediaPage = lazy(() => import('./pages/MediaPage'))
const ProfilesPage = lazy(() => import('./pages/ProfilesPage'))
const SystemPage = lazy(() => import('./pages/SystemPage'))

export function AppRoutes() {
  const location = useLocation()
  return (
    <Routes>
      <Route path="/" element={<LibraryPage />} />
      <Route path="/add" element={<AddPage />} />
      <Route path="/series/:id" element={<SeriesPage key={location.pathname} />} />
      <Route path="/calendar" element={<CalendarPage />} />
      <Route path="/queue" element={<QueuePage />} />
      <Route path="/wanted" element={<WantedPage />} />
      <Route path="/connections" element={<ConnectionsPage />} />
      {/* The page was called Sources; keep old bookmarks working. */}
      <Route path="/sources" element={<Navigate to="/connections" replace />} />
      <Route path="/indexers" element={<IndexersPage />} />
      <Route path="/profiles" element={<ProfilesPage />} />
      <Route path="/media" element={<MediaPage />} />
      <Route path="/system" element={<SystemPage />} />
    </Routes>
  )
}
