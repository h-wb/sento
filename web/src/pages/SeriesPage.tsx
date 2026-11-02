import EditionManager, { type EditionSection } from '../components/EditionManager'
import FileCleanup from '../components/FileCleanup'
import OrganizeFiles from '../components/OrganizeFiles'
import AddEdition, { type EditionStart } from '../components/AddEdition'
import { useCallback, useState } from 'react'
import {
  Anchor,
  ActionIcon,
  Badge,
  Button,
  Group,
  Image,
  Menu,
  Modal,
  Skeleton,
  Stack,
  Switch,
  Tabs,
  Text,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import {
  IconAdjustmentsHorizontal,
  IconChevronLeft,
  IconDotsVertical,
  IconPlugConnected,
  IconRefresh,
  IconSearch,
  IconTrash,
} from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, type Edition } from '../api'
import BookDialog, { type BookSelection } from '../components/BookDialog'
import { notify, usePageTitle } from '../ui'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { cardLabel } from '../components/editionText'
import { RemoveSeries } from '../components/RemoveSeries'
import classes from './SeriesPage.module.css'
import ActivityPanel from '../components/ActivityPanel'
import { useActivity } from '../components/useActivity'
import { activityCounts } from '../components/activityModel'
import { ProfileBarButton, ProfileMenuItems } from '../components/SeriesProfile'
import { SeriesSources } from '../components/SeriesSources'

export default function SeriesPage() {
  const { id } = useParams()
  const seriesID = Number(id)
  const navigate = useNavigate()

  const [busy, setBusy] = useState(false)
  const [bookSel, setBookSel] = useState<BookSelection | null>(null)
  const [fullDesc, setFullDesc] = useState(false)
  const [removing, setRemoving] = useState(false)
  // Add edition: a start from the search (which reopens on the new
  // edition), or {} for a blank form; null: closed.
  const [addEdition, setAddEdition] = useState<(Partial<EditionStart> & { search?: boolean }) | null>(null)
  const searchEdition = async (e: Edition) => {
    const versions = await api.versions(seriesID)
    const v = versions.find((v) => v.editionId === e.id && v.isDefault)
    if (!v) return
    setBookSel({
      versionId: v.id,
      all: true,
      ledger: e.type === 'chapter' ? 'chapter' : 'volume',
      number: 0,
      editionId: e.id,
      editionLabel: cardLabel(e, v, versions),
      monitored: v.monitored,
      wanted: true,
      fileIds: [],
      autoSearch: true,
    })
  }
  // The open tab lives in ?view= so it survives a reload and can be linked
  // to; ?view=review is the deep link the global review inbox uses.
  const [searchParams, setSearchParams] = useSearchParams()
  const view = searchParams.get('view')
  const tab = view === 'review' || view === 'history' ? 'activity' : view && SERIES_TABS.includes(view) ? view : 'editions'
  const setParam = (key: string, value: string | null) =>
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev)
        if (value === null) params.delete(key)
        else params.set(key, value)
        return params
      },
      { replace: true },
    )
  const setTab = (next: string | null) => setParam('view', !next || next === 'editions' ? null : next)
  // Sources are per-series settings, opened from the menu or a review
  // item; ?sources=1 (and the old ?view=sources) link to them.
  const sourcesOpen = searchParams.get('sources') === '1' || view === 'sources'
  const setSourcesOpen = (open: boolean) => {
    setParam('sources', open ? '1' : null)
    if (!open && view === 'sources') setTab(null)
  }
  const [cleanup, setCleanup] = useState(false)
  const [organize, setOrganize] = useState(false)

  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['series', seriesID],
    queryFn: () => api.getSeries(seriesID),
  })
  const activity = useActivity(seriesID)
  const attentionCount = activityCounts(activity.data).attention
  const overviewQuery = useQuery({
    queryKey: ['series', seriesID, 'edition-overview'],
    queryFn: () => api.editionOverview(seriesID),
  })
  const filesQuery = useQuery({
    queryKey: ['series', seriesID, 'files'],
    queryFn: () => api.files(seriesID),
  })

  const series = detail.data?.series ?? null
  usePageTitle(series?.title ?? '')
  const editions = detail.data?.editions ?? []
  const overview = overviewQuery.data ?? null
  const files = filesQuery.data ?? []

  // Refresh everything under this series (books, files, indexers, detail).
  const load = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['queue'] })
    void queryClient.invalidateQueries({ queryKey: ['activity'] })
    return queryClient.invalidateQueries({ queryKey: ['series'] })
  }, [queryClient])

  // Editions and Files share their book, version and file-mapping dialogs.
  const editionManager = (section: EditionSection) => (
    <EditionManager
      section={section}
      seriesID={seriesID}
      editions={editions}
      volumeCount={series?.finalVolume ?? 0}
      catalogUrl={`https://mangabaka.org/${series?.mangabakaId ?? ''}`}
      files={files}
      onBook={setBookSel}
      onAddEdition={(start) => setAddEdition(start ?? {})}
      seriesProfileId={series?.profileId ?? 0}
    />
  )

  const run = async (fn: () => Promise<unknown>, doneMsg?: string) => {
    setBusy(true)
    try {
      await fn()
      if (doneMsg) notify.ok(doneMsg)
      load()
    } catch (e) {
      notify.err(e)
    } finally {
      setBusy(false)
    }
  }

  if (!series) {
    if (detail.error)
      return <QueryFeedback error={detail.error} onRetry={() => void detail.refetch()} />
    return (
      <Group align="flex-start">
        <Skeleton w={140} h={200} radius="sm" />
        <Stack style={{ flex: 1 }}>
          <Skeleton h={28} w="40%" />
          <Skeleton h={16} w="60%" />
          <Skeleton h={60} />
        </Stack>
      </Group>
    )
  }

  const owned = overview?.booksOnDisk ?? 0
  const wanted = overview?.wantedCopies ?? 0

  return (
    <>
      <Anchor
        component={Link}
        to="/"
        size="xs"
        c="dimmed"
        underline="hover"
        className={classes.back}
      >
        <IconChevronLeft size={13} /> Library
      </Anchor>
      <div className={classes.hero}>
        <Image
          src={series.coverUrl}
          className={classes.cover}
          alt={series.title}
          fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='240'%3E%3Crect width='160' height='240' fill='%2352524e'/%3E%3C/svg%3E"
        />
        <div className={classes.details}>
          <Title order={2} className={classes.title}>
            {series.title}
          </Title>
          <Group gap={8} mt={10}>
            <Text size="sm" c="dimmed" tt="capitalize">
              {[series.type, series.status?.replaceAll('_', ' '), series.year]
                .filter(Boolean)
                .join(' · ')}
            </Text>
            {series.contentRating && series.contentRating !== 'safe' && (
              <Badge size="xs" color="gray">
                {series.contentRating}
              </Badge>
            )}
          </Group>
          {series.description && (
            <>
              <Text
                size="sm"
                c="dimmed"
                mt="md"
                lh={1.65}
                lineClamp={fullDesc ? undefined : 3}
                id="series-description"
              >
                {series.description}
              </Text>
              <UnstyledButton
                onClick={() => setFullDesc((v) => !v)}
                aria-expanded={fullDesc}
                aria-controls="series-description"
                mt={5}
              >
                <Text size="xs" fw={500}>
                  {fullDesc ? 'Show less' : 'Read more'}
                </Text>
              </UnstyledButton>
            </>
          )}
          <Group gap={24} mt="lg" className={classes.stats}>
            {series.finalVolume > 0 && (
              <div>
                <Text size="lg" fw={500}>
                  {series.finalVolume}
                </Text>
                <Text size="xs" c="dimmed">
                  Volumes
                </Text>
              </div>
            )}
            {series.totalChapters > 0 && (
              <div>
                <Text size="lg" fw={500}>
                  {series.totalChapters}
                </Text>
                <Text size="xs" c="dimmed">
                  Chapters
                </Text>
              </div>
            )}
            {overview && (
              <>
                <div>
                  <Text size="lg" fw={500}>
                    {owned}
                  </Text>
                  <Text size="xs" c="dimmed">
                    Books on disk
                  </Text>
                </div>
                <div>
                  <Text size="lg" fw={500}>
                    {wanted}
                  </Text>
                  <Text size="xs" c="dimmed">
                    Books wanted
                  </Text>
                </div>
              </>
            )}
          </Group>
        </div>
      </div>
      <div className={classes.actions}>
        <Group gap="xs">
          <Tooltip label="Search collected editions and download the best matching releases automatically.">
            <Button
              size="sm"
              leftSection={<IconSearch size={15} />}
              loading={busy}
              disabled={!series.monitored}
              onClick={() =>
                run(async () => {
                  const [v, c] = await Promise.all([
                    api.search(seriesID, { ledger: 'volume' }),
                    api.search(seriesID, { ledger: 'chapter' }),
                  ])
                  const queued = (v.queued?.length ?? 0) + (c.queued?.length ?? 0)
                  notify.ok(
                    queued
                      ? `${queued} ${queued === 1 ? 'release' : 'releases'} queued`
                      : 'No matching releases to download',
                  )
                })
              }
            >
              Search & download
            </Button>
          </Tooltip>
        </Group>
        <Group gap="md">
          {/* The profile is worth a word only when this series chose one. */}
          {!!series.profileId && (
            <ProfileBarButton seriesID={seriesID} profileId={series.profileId} />
          )}
          <Tooltip label="Allow Sento to search for and download missing releases for this series.">
            <Switch
              size="sm"
              label="Monitor series"
              checked={series.monitored}
              disabled={busy}
              onChange={(e) => {
                const monitored = e.currentTarget.checked
                void run(() => api.setSeriesMonitored(seriesID, monitored))
              }}
            />
          </Tooltip>
          <Menu position="bottom-end">
            <Menu.Target>
              <ActionIcon size="lg" variant="subtle" color="gray" aria-label="Series actions">
                <IconDotsVertical size={18} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item
                leftSection={<IconRefresh size={15} />}
                disabled={busy}
                onClick={() =>
                  run(async () => {
                    await api.refreshSeries(seriesID)
                    const r = await api.scanSeries(seriesID)
                    notify.ok(`Series refreshed · ${r.linked} files linked`)
                  })
                }
              >
                Refresh information and files
              </Menu.Item>
              <Menu.Sub position="left-start">
                <Menu.Sub.Target>
                  <Menu.Sub.Item leftSection={<IconAdjustmentsHorizontal size={15} />}>
                    Release profile
                  </Menu.Sub.Item>
                </Menu.Sub.Target>
                <Menu.Sub.Dropdown>
                  <ProfileMenuItems seriesID={seriesID} profileId={series.profileId ?? 0} />
                </Menu.Sub.Dropdown>
              </Menu.Sub>
              <Menu.Item leftSection={<IconPlugConnected size={15} />} onClick={() => setSourcesOpen(true)}>
                Sources…
              </Menu.Item>
              <Menu.Divider />
              <Menu.Item
                color="red"
                leftSection={<IconTrash size={15} />}
                disabled={busy}
                onClick={() => setRemoving(true)}
              >
                Remove series
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
        </Group>
      </div>
      {!series.monitored && (
        <Text size="sm" c="dimmed" mb="lg">
          Automatic downloads are paused for this series. You can still browse releases from its
          editions.
        </Text>
      )}
      {removing && (
        <RemoveSeries
          ids={[seriesID]}
          name={series.title}
          close={() => setRemoving(false)}
          removed={() => {
            void queryClient.invalidateQueries({ queryKey: ['series'] })
            navigate('/')
          }}
        />
      )}
      {addEdition && (
        <AddEdition
          seriesID={seriesID}
          series={series}
          publishers={detail.data?.publishers ?? []}
          editions={editions}
          busy={busy}
          run={run}
          onClose={() => setAddEdition(null)}
          initial={addEdition.editionName ? (addEdition as EditionStart) : undefined}
          onCreated={addEdition.search ? (e) => void searchEdition(e) : undefined}
        />
      )}

      <Tabs value={tab} onChange={setTab} keepMounted={false}>
        <Tabs.List>
          <Tabs.Tab value="editions">Editions</Tabs.Tab>
          <Tabs.Tab value="files">Files ({files.length})</Tabs.Tab>
          <Tabs.Tab value="activity" rightSection={attentionCount > 0 ? <Badge aria-hidden size="xs" circle color="yellow">{attentionCount}</Badge> : undefined} aria-description={attentionCount > 0 ? `${attentionCount} needs attention` : undefined}>Activity</Tabs.Tab>
        </Tabs.List>
        <QueryFeedback error={overviewQuery.error} onRetry={() => void overviewQuery.refetch()} />
        <QueryFeedback error={filesQuery.error} onRetry={() => void filesQuery.refetch()} />

        <Tabs.Panel value="editions" pt="md">
          {editionManager('editions')}
        </Tabs.Panel>
        <Tabs.Panel value="activity" pt="md">
          <ActivityPanel key={seriesID} seriesID={seriesID} onSources={() => setSourcesOpen(true)} legacyFilter={view === 'review' ? 'attention' : view === 'history' ? 'history' : undefined} />
        </Tabs.Panel>

        <Tabs.Panel value="files" pt="md">
          <Stack gap="xs">
            <Group>
              <Button
                size="xs"
                variant="default"
                loading={busy}
                onClick={() =>
                  run(() =>
                    api
                      .scanSeries(seriesID)
                      .then((r) =>
                        notify.ok(
                          `Scan: ${r.scanned} new files, ${r.linked} linked, ${r.unrequested} unrequested` +
                            (r.changed ? `, ${r.changed} replaced on disk and matched again` : ''),
                        ),
                      ),
                  )
                }
              >
                Scan for files
              </Button>
              <Tooltip label="Move, rename and re-tag existing files to match the current Media settings.">
                <Button size="xs" variant="default" disabled={files.length === 0} onClick={() => setOrganize(true)}>
                  Organize files
                </Button>
              </Tooltip>
              <Button size="xs" variant="default" onClick={() => setCleanup(true)}>Clean up files</Button>
            </Group>
            {editionManager('files')}
            {filesQuery.isPending && <QueryFeedback pending />}
            {!filesQuery.isPending && !filesQuery.error && files.length === 0 && (
              <EmptyState
                title="No files yet"
                description="Download a release or scan the series folder to add existing files."
              />
            )}
          </Stack>
        </Tabs.Panel>

      </Tabs>

      <Modal opened={sourcesOpen} onClose={() => setSourcesOpen(false)} title="Sources" size="xl">
        <SeriesSources
          seriesID={seriesID}
          seriesTitle={series.title}
          editions={editions}
          run={run}
          busy={busy}
        />
      </Modal>
      {cleanup && <FileCleanup seriesID={seriesID} onClose={() => setCleanup(false)} />}
      {organize && <OrganizeFiles seriesID={seriesID} onClose={() => setOrganize(false)} />}
      <BookDialog
        seriesID={seriesID}
        sel={bookSel}
        files={files}
        onClose={() => setBookSel(null)}
        onChanged={load}
        onAddEdition={(start) => {
          setBookSel(null)
          setAddEdition({ ...start, search: true })
        }}
      />
    </>
  )
}

const SERIES_TABS = ['editions', 'files', 'activity']
