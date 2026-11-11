import { useActivity } from './components/useActivity'
import { activityCounts } from './components/activityModel'
import {
  ActionIcon,
  AppShell,
  Avatar,
  Badge,
  Tooltip,
  Burger,
  Center,
  ColorSwatch,
  Divider,
  Group,
  Kbd,
  Loader,
  Menu,
  Modal,
  NavLink,
  Table,
  Text,
  Title,
  UnstyledButton,
  useMantineColorScheme,
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { NavigationProgress, nprogress } from '@mantine/nprogress'
import { Spotlight, spotlight, type SpotlightActionData } from '@mantine/spotlight'
import { lazy, Suspense, useEffect } from 'react'
import {
  IconAdjustmentsHorizontal,
  IconBooks,
  IconCalendar,
  IconDownload,
  IconCheck,
  IconDeviceDesktop,
  IconFiles,
  IconMoon,
  IconPalette,
  IconPlus,
  IconSearch,
  IconServerBolt,
  IconPlug,
  IconSun,
  IconWorldSearch,
  IconZoomCheck,
} from '@tabler/icons-react'
import { useIsFetching, useQuery } from '@tanstack/react-query'
import { Link, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { api } from './api'
import { versionLabel } from './ui'
import classes from './App.module.css'
import { AddSeriesCtx } from './addSeries'
import { AddSeriesForm } from './components/AddSeriesForm'
import { PRESETS, useThemePreset } from './theme'
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

type NavItem = { to: string; label: string; icon: typeof IconBooks; key: string }

// Day-to-day pages, always on top. `key` is the page's "g <key>" shortcut.
const primaryLinks: NavItem[] = [
  { to: '/', label: 'Library', icon: IconBooks, key: 'l' },
  { to: '/calendar', label: 'Calendar', icon: IconCalendar, key: 'c' },
  { to: '/queue', label: 'Activity', icon: IconDownload, key: 'a' },
  { to: '/wanted', label: 'Wanted', icon: IconZoomCheck, key: 'w' },
]
// Set-up-once config, ordered to mirror the pipeline: search → download →
// quality decision → import.
const configLinks: NavItem[] = [
  { to: '/indexers', label: 'Indexers', icon: IconWorldSearch, key: 'i' },
  { to: '/connections', label: 'Connections', icon: IconPlug, key: 'o' },
  { to: '/profiles', label: 'Profiles', icon: IconAdjustmentsHorizontal, key: 'p' },
  { to: '/media', label: 'Media', icon: IconFiles, key: 'm' },
]
const systemLink: NavItem = { to: '/system', label: 'System', icon: IconServerBolt, key: 's' }
const allLinks = [...primaryLinks, ...configLinks, systemLink]

const MODES = [
  { value: 'light', label: 'Light', icon: IconSun },
  { value: 'dark', label: 'Dark', icon: IconMoon },
  { value: 'auto', label: 'System', icon: IconDeviceDesktop },
] as const

const modKey = /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘' : 'Ctrl '

// "g" then a page's key jumps to it (Gmail-style sequence); "g n" adds a series.
const SHORTCUTS: [string[], string][] = [
  [[`${modKey}K`, '/'], 'Search pages and series'],
  ...allLinks.map((l): [string[], string] => [[`g ${l.key}`], `Go to ${l.label}`]),
  [['g n'], 'Add a series'],
  [['?'], 'Show this help'],
]

export default function App() {
  const location = useLocation()
  const navigate = useNavigate()
  const [navOpened, { toggle: toggleNav, close: closeNav }] = useDisclosure()
  const [helpOpen, { open: openHelp, close: closeHelp }] = useDisclosure(false)
  const [addOpen, { open: openAdd, close: closeAdd }] = useDisclosure(false)
  const { colorScheme, setColorScheme } = useMantineColorScheme()
  const { themeId, setThemeId } = useThemePreset()

  // Light background polling so the shell can hint at what's going on.
  const status = useQuery({ queryKey: ['status'], queryFn: api.status, refetchInterval: 60_000 })
  const health = useQuery({ queryKey: ['health'], queryFn: api.healthChecks, refetchInterval: 60_000 })
  const activity = useActivity()
  // Cached library list, shared with LibraryPage — powers ⌘K series search.
  const seriesList = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const activityCount = activityCounts(activity.data)
  const active = activityCount.attention || activityCount.progress
  const healthy = !status.isError && status.data?.database.ok === true

  // Drive the top progress bar off real network activity.
  const fetching = useIsFetching()
  useEffect(() => {
    if (fetching > 0) nprogress.start()
    else nprogress.complete()
  }, [fetching])

  // Global keyboard shortcuts: "/" opens search, "?" shows help, and a
  // Gmail-style "g <key>" jumps between pages. Ignored while typing.
  useEffect(() => {
    let gAt = 0
    const typing = (el: EventTarget | null) =>
      el instanceof HTMLElement &&
      (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable)
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target)) return
      if (e.key === '/') {
        e.preventDefault()
        spotlight.open()
      } else if (e.key === '?') {
        e.preventDefault()
        openHelp()
      } else if (e.key === 'g') {
        gAt = e.timeStamp
      } else if (gAt > 0 && e.timeStamp - gAt < 1000) {
        if (e.key === 'n') {
          e.preventDefault()
          openAdd()
          gAt = 0
        } else {
          const link = allLinks.find((l) => l.key === e.key)
          if (link) {
            e.preventDefault()
            navigate(link.to)
            gAt = 0
          }
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, openHelp, openAdd])

  const isActive = (to: string) =>
    to === '/' ? location.pathname === '/' || location.pathname.startsWith('/series/') : location.pathname.startsWith(to)

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

  const healthIssues = health.data?.issues ?? []
  const healthSummary = `${healthIssues.length} health ${healthIssues.length === 1 ? 'issue' : 'issues'}`
  // The count is announced as a description so the link keeps its name.
  const healthBadge =
    healthIssues.length > 0 ? (
      <Tooltip label={healthSummary}>
        <Badge aria-hidden size="xs" circle color={healthIssues.some((i) => i.level === 'error') ? 'red' : 'yellow'}>
          {healthIssues.length}
        </Badge>
      </Tooltip>
    ) : undefined

  const renderLink = (l: NavItem) => (
    <NavLink
      key={l.to}
      className={classes.navlink}
      component={Link}
      to={l.to}
      label={l.label}
      leftSection={<l.icon size={18} />}
      rightSection={
        l.to === '/queue' && active > 0 ? (
          <Badge size="xs" circle color={activityCount.attention > 0 ? 'orange' : 'brand'}>
            {active}
          </Badge>
        ) : l.to === '/system' ? (
          healthBadge
        ) : undefined
      }
      aria-description={l.to === '/system' && healthIssues.length > 0 ? healthSummary : l.to === '/queue' && activityCount.attention > 0 ? `${activityCount.attention} ${activityCount.attention === 1 ? 'issue needs' : 'issues need'} attention` : undefined}
      active={isActive(l.to)}
      onClick={closeNav}
    />
  )

  return (
    <AddSeriesCtx.Provider value={openAdd}>
      <NavigationProgress color="brand" />
      <AppShell
        header={{ height: 64 }}
        navbar={{ width: 216, breakpoint: 'sm', collapsed: { mobile: !navOpened } }}
        padding={0}
      >
      <AppShell.Header className={classes.header}>
        <Group h="100%" px="md" gap="md">
          <Burger opened={navOpened} onClick={toggleNav} hiddenFrom="sm" size="sm" aria-label="Toggle navigation" />
          <Group gap={10} wrap="nowrap">
            <span className={classes.brandMark} aria-hidden="true"><IconBooks size={19} stroke={1.7} /></span>
            <Title order={3} className={classes.wordmark}>sento</Title>
          </Group>
          <Text c="dimmed" size="xs" visibleFrom="sm" className={classes.breadcrumb}>
            / &nbsp; {allLinks.find((l) => isActive(l.to))?.label ?? (location.pathname === '/add' ? 'Add series' : 'Series')}
          </Text>
          <Group gap="xs" ml="auto" wrap="nowrap">
            <UnstyledButton
              className={classes.searchTrigger}
              onClick={spotlight.open}
              visibleFrom="xs"
            >
              <Group gap="xs" px="sm" h={32} wrap="nowrap">
                <IconSearch size={15} />
                <Text size="sm">Search library…</Text>
                <Kbd size="xs">{modKey}K</Kbd>
              </Group>
            </UnstyledButton>
            <ActionIcon hiddenFrom="xs" variant="default" size="lg" aria-label="Search library" onClick={spotlight.open}><IconSearch size={18} /></ActionIcon>
            <Menu position="bottom-end" withArrow width={220} closeOnItemClick={false}>
              <Menu.Target>
                <ActionIcon variant="default" size="lg" radius="md" aria-label="Appearance">
                  <IconPalette size={18} />
                </ActionIcon>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Label>Mode</Menu.Label>
                {MODES.map((m) => (
                  <Menu.Item
                    key={m.value}
                    leftSection={<m.icon size={16} />}
                    rightSection={
                      colorScheme === m.value ? <IconCheck size={14} /> : undefined
                    }
                    onClick={() => setColorScheme(m.value)}
                  >
                    {m.label}
                  </Menu.Item>
                ))}
                <Menu.Divider />
                <Menu.Label>Theme</Menu.Label>
                {PRESETS.map((p) => (
                  <Menu.Item
                    key={p.id}
                    leftSection={<ColorSwatch color={p.swatch} size={14} />}
                    rightSection={
                      p.id === themeId ? <IconCheck size={14} /> : undefined
                    }
                    onClick={() => setThemeId(p.id)}
                  >
                    {p.label}
                  </Menu.Item>
                ))}
              </Menu.Dropdown>
            </Menu>
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar p="md" className={classes.navbar}>
        <AppShell.Section grow>
          {primaryLinks.map(renderLink)}

          <Text className={classes.sectionLabel}>Settings</Text>
          {configLinks.map(renderLink)}
        </AppShell.Section>
        <AppShell.Section>
          <Divider mb="xs" />
          {renderLink(systemLink)}
          <Group gap={7} px="sm" pt="md" pb={4}>
            <span className={classes.statusDot} data-healthy={healthy} />
            <Tooltip label="Connection to the Sento server. Download services have their own status on Connections.">
              <Text size="xs" c="dimmed">{status.isPending ? 'Connecting' : healthy ? 'Server connected' : 'Server unavailable'}</Text>
            </Tooltip>
          </Group>
          <Text size="xs" c="dimmed" px="sm" py={4}>
            {status.data ? versionLabel(status.data) : '…'}
            {!healthy && ' · backend unreachable'}
          </Text>
        </AppShell.Section>
      </AppShell.Navbar>
      <AppShell.Main className={classes.main}><div className={classes.content}>
        <Suspense
          fallback={
            <Center mih="60vh">
              <Loader />
            </Center>
          }
        >
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
        </Suspense>
      </div></AppShell.Main>
        <Spotlight
          actions={spotlightActions}
          nothingFound="Nothing found…"
          highlightQuery
          limit={8}
          scrollable
          searchProps={{
            leftSection: <IconSearch size={18} />,
            placeholder: 'Search pages and series…',
          }}
        />
      </AppShell>
      <Modal opened={helpOpen} onClose={closeHelp} title="Keyboard shortcuts" centered>
        <Table verticalSpacing="xs">
          <Table.Tbody>
            {SHORTCUTS.map(([keys, desc]) => (
              <Table.Tr key={desc}>
                <Table.Td w={140}>
                  <Group gap={6}>
                    {keys.map((k) => (
                      <Kbd key={k} size="sm">
                        {k}
                      </Kbd>
                    ))}
                  </Group>
                </Table.Td>
                <Table.Td>
                  <Text size="sm">{desc}</Text>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Modal>
      <Modal opened={addOpen} onClose={closeAdd} title="Add series" size="lg">
        <AddSeriesForm
          onDone={(id) => {
            closeAdd()
            navigate(`/series/${id}`)
          }}
          onOpenImporter={() => {
            closeAdd()
            navigate('/add?tab=import')
          }}
        />
      </Modal>
    </AddSeriesCtx.Provider>
  )
}
