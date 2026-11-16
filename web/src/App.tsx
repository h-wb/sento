import {
  ActionIcon,
  AppShell,
  Burger,
  Center,
  Divider,
  Group,
  Kbd,
  Loader,
  Modal,
  NavLink,
  Text,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { NavigationProgress, nprogress } from '@mantine/nprogress'
import { Spotlight, spotlight } from '@mantine/spotlight'
import { IconBooks, IconSearch } from '@tabler/icons-react'
import { useIsFetching, useQuery } from '@tanstack/react-query'
import { Suspense, useEffect } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import classes from './App.module.css'
import { AddSeriesCtx } from './addSeries'
import { api } from './api'
import { AddSeriesForm } from './components/AddSeriesForm'
import { activityCounts } from './components/activityModel'
import { useActivity } from './components/useActivity'
import { plural } from './format'
import { AppRoutes } from './routes'
import { AppearanceMenu } from './shell/AppearanceMenu'
import { ActivityBadge, HealthBadge } from './shell/NavBadges'
import { ShortcutsHelp } from './shell/ShortcutsHelp'
import { configLinks, modKey, primaryLinks, systemLink, type NavItem } from './shell/navigation'
import { useShortcuts } from './shell/useShortcuts'
import { useSpotlightActions } from './shell/useSpotlightActions'
import { versionLabel } from './ui'

export default function App() {
  const location = useLocation()
  const navigate = useNavigate()
  const [navOpened, { toggle: toggleNav, close: closeNav }] = useDisclosure()
  const [helpOpen, { open: openHelp, close: closeHelp }] = useDisclosure(false)
  const [addOpen, { open: openAdd, close: closeAdd }] = useDisclosure(false)

  // Light background polling so the shell can hint at what's going on.
  const status = useQuery({ queryKey: ['status'], queryFn: api.status, refetchInterval: 60_000 })
  const health = useQuery({ queryKey: ['health'], queryFn: api.healthChecks, refetchInterval: 60_000 })
  const activity = useActivity()
  const activityCount = activityCounts(activity.data)
  const active = activityCount.attention || activityCount.progress
  const healthy = !status.isError && status.data?.database.ok === true

  // Drive the top progress bar off real network activity.
  const fetching = useIsFetching()
  useEffect(() => {
    if (fetching > 0) nprogress.start()
    else nprogress.complete()
  }, [fetching])

  useShortcuts(openHelp, openAdd)

  const isActive = (to: string) =>
    to === '/' ? location.pathname === '/' || location.pathname.startsWith('/series/') : location.pathname.startsWith(to)

  const spotlightActions = useSpotlightActions(openAdd)

  const healthIssues = health.data?.issues ?? []
  const healthSummary = `${healthIssues.length} health ${plural(healthIssues.length, 'issue')}`

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
          <ActivityBadge active={active} attention={activityCount.attention} />
        ) : l.to === '/system' && healthIssues.length > 0 ? (
          <HealthBadge issues={healthIssues} summary={healthSummary} />
        ) : undefined
      }
      aria-description={l.to === '/system' && healthIssues.length > 0 ? healthSummary : l.to === '/queue' && activityCount.attention > 0 ? `${activityCount.attention} ${plural(activityCount.attention, 'issue needs', 'issues need')} attention` : undefined}
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
            <AppearanceMenu />
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
          <AppRoutes />
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
      <ShortcutsHelp helpOpen={helpOpen} closeHelp={closeHelp} />
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
