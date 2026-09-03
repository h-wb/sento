import {
  ActionIcon,
  AppShell,
  Avatar,
  Badge,
  Burger,
  Center,
  ColorSwatch,
  Divider,
  Group,
  Indicator,
  Kbd,
  Loader,
  Menu,
  Modal,
  NavLink,
  Stack,
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
} from '@tabler/icons-react'
import { useIsFetching, useQuery } from '@tanstack/react-query'
import { Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { api } from './api'
import classes from './App.module.css'
import { AddSeriesCtx } from './addSeries'
import { AddSeriesForm } from './components/AddSeriesForm'
import { PRESETS, useThemePreset } from './theme'
import LibraryPage from './pages/LibraryPage'

// Lazy-loaded routes so heavy deps (e.g. recharts on Activity/System) land in
// their own chunks, fetched only when the route is visited. Library stays
// eager since it's the landing page.
const AddPage = lazy(() => import('./pages/AddPage'))
const SeriesPage = lazy(() => import('./pages/SeriesPage'))
const CalendarPage = lazy(() => import('./pages/CalendarPage'))
const QueuePage = lazy(() => import('./pages/QueuePage'))
const IndexersPage = lazy(() => import('./pages/IndexersPage'))
const SourcesPage = lazy(() => import('./pages/SourcesPage'))
const MediaPage = lazy(() => import('./pages/MediaPage'))
const ProfilesPage = lazy(() => import('./pages/ProfilesPage'))
const SystemPage = lazy(() => import('./pages/SystemPage'))

// Day-to-day pages, always on top.
const primaryLinks = [
  { to: '/', label: 'Library', icon: IconBooks },
  { to: '/calendar', label: 'Calendar', icon: IconCalendar },
  { to: '/queue', label: 'Activity', icon: IconDownload },
]
// Set-up-once config, ordered to mirror the pipeline: search → download →
// quality decision → import.
const configLinks = [
  { to: '/indexers', label: 'Indexers', icon: IconWorldSearch },
  { to: '/sources', label: 'Sources', icon: IconPlug },
  { to: '/profiles', label: 'Profiles', icon: IconAdjustmentsHorizontal },
  { to: '/media', label: 'Media', icon: IconFiles },
]
const systemLink = { to: '/system', label: 'System', icon: IconServerBolt }
type NavItem = (typeof primaryLinks)[number]

const MODES = [
  { value: 'light', label: 'Light', icon: IconSun },
  { value: 'dark', label: 'Dark', icon: IconMoon },
  { value: 'auto', label: 'System', icon: IconDeviceDesktop },
] as const

// `g` then one of these keys jumps to a page (Gmail-style sequence).
const GO_TO: Record<string, string> = {
  l: '/',
  c: '/calendar',
  a: '/queue',
  i: '/indexers',
  o: '/sources',
  p: '/profiles',
  m: '/media',
  s: '/system',
}

const SHORTCUTS: [string, string][] = [
  ['⌘K  /  /', 'Open command palette'],
  ['g then l', 'Go to Library'],
  ['g then c', 'Go to Calendar'],
  ['g then a', 'Go to Activity'],
  ['g then n', 'Add Series'],
  ['g then i / o / p / m', 'Indexers / Sources / Profiles / Media'],
  ['g then s', 'Go to System'],
  ['?', 'Show this help'],
]

export default function App() {
  const location = useLocation()
  const navigate = useNavigate()
  const [navOpened, { toggle: toggleNav, close: closeNav }] = useDisclosure()
  const [helpOpen, { open: openHelp, close: closeHelp }] = useDisclosure(false)
  const [addOpen, { open: openAdd, close: closeAdd }] = useDisclosure(false)
  const { colorScheme, setColorScheme } = useMantineColorScheme()
  const { themeId, setThemeId, preset } = useThemePreset()

  // Light background polling so the shell can hint at what's going on.
  const status = useQuery({ queryKey: ['status'], queryFn: api.status, refetchInterval: 60_000 })
  const queue = useQuery({
    queryKey: ['queue', 'active-count'],
    queryFn: () => api.queue(false),
    refetchInterval: 15_000,
  })
  // Cached library list, shared with LibraryPage — powers ⌘K series search.
  const seriesList = useQuery({ queryKey: ['series'], queryFn: api.listSeries })
  const active = queue.data?.length ?? 0
  const healthy = !status.isError && (status.data?.database.ok ?? true)

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
      } else if (e.timeStamp - gAt < 1000) {
        if (e.key === 'n') {
          e.preventDefault()
          openAdd()
          gAt = 0
        } else if (GO_TO[e.key]) {
          e.preventDefault()
          navigate(GO_TO[e.key])
          gAt = 0
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, openHelp, openAdd])

  const isActive = (to: string) =>
    to === '/' ? location.pathname === '/' : location.pathname.startsWith(to)

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
      label: 'Add Series',
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
          <Badge size="xs" circle color="brand">
            {active}
          </Badge>
        ) : undefined
      }
      active={isActive(l.to)}
      onClick={closeNav}
    />
  )

  return (
    <AddSeriesCtx.Provider value={openAdd}>
      <NavigationProgress color="brand" />
      <AppShell
        header={{ height: 56 }}
        navbar={{ width: 220, breakpoint: 'sm', collapsed: { mobile: !navOpened } }}
        padding="lg"
      >
      <AppShell.Header className={classes.header}>
        <Group h="100%" px="md" gap="md">
          <Burger opened={navOpened} onClick={toggleNav} hiddenFrom="sm" size="sm" />
          <Indicator
            color={healthy ? 'teal' : 'red'}
            size={7}
            offset={2}
            position="top-end"
            processing={!healthy}
          >
            <Title order={3} className={classes.wordmark}>
              {preset.effects.gradient ? (
                <Text
                  span
                  inherit
                  variant="gradient"
                  gradient={{ from: 'brand.4', to: `${preset.accentTo}.4`, deg: 45 }}
                >
                  sento
                </Text>
              ) : (
                'sento'
              )}
            </Title>
          </Indicator>
          <Text c="dimmed" size="sm" visibleFrom="xs">
            manga automation
          </Text>
          <Group gap="xs" ml="auto" wrap="nowrap">
            <UnstyledButton
              className={classes.searchTrigger}
              onClick={spotlight.open}
              visibleFrom="xs"
            >
              <Group gap="xs" px="sm" h={32} wrap="nowrap">
                <IconSearch size={15} />
                <Text size="sm">Search…</Text>
                <Kbd size="xs">⌘K</Kbd>
              </Group>
            </UnstyledButton>
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
      <AppShell.Navbar p="xs" className={classes.navbar}>
        <AppShell.Section grow>
          {primaryLinks.map(renderLink)}

          <Text className={classes.sectionLabel}>Configure</Text>
          {configLinks.map(renderLink)}
        </AppShell.Section>
        <AppShell.Section>
          <Divider mb="xs" />
          {renderLink(systemLink)}
          <Text size="xs" c="dimmed" px="sm" py={4}>
            {status.data ? `v${status.data.version}` : '…'}
            {!healthy && ' · backend unreachable'}
          </Text>
        </AppShell.Section>
      </AppShell.Navbar>
      <AppShell.Main>
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
            <Route path="/series/:id" element={<SeriesPage />} />
            <Route path="/calendar" element={<CalendarPage />} />
            <Route path="/queue" element={<QueuePage />} />
            <Route path="/sources" element={<SourcesPage />} />
            <Route path="/indexers" element={<IndexersPage />} />
            <Route path="/profiles" element={<ProfilesPage />} />
            <Route path="/media" element={<MediaPage />} />
            <Route path="/system" element={<SystemPage />} />
          </Routes>
        </Suspense>
      </AppShell.Main>
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
                <Table.Td w={170}>
                  <Stack gap={4} align="flex-start">
                    {keys.split('  /  ').map((k) => (
                      <Kbd key={k} size="sm">
                        {k}
                      </Kbd>
                    ))}
                  </Stack>
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
            navigate('/add')
          }}
        />
      </Modal>
    </AddSeriesCtx.Provider>
  )
}
