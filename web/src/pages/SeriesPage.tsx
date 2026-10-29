import EditionManager, { type EditionSection } from '../components/EditionManager'
import OrganizeFiles from '../components/OrganizeFiles'
import AddEdition, { type EditionStart } from '../components/AddEdition'
import { Fragment, useCallback, useState, type ReactNode } from 'react'
import {
  Anchor,
  ActionIcon,
  Badge,
  Button,
  Checkbox,
  Group,
  Image,
  Menu,
  Modal,
  Select,
  Skeleton,
  Stack,
  Switch,
  Table,
  Tabs,
  Text,
  TextInput,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import {
  IconAdjustmentsHorizontal,
  IconChevronDown,
  IconChevronLeft,
  IconChevronRight,
  IconDotsVertical,
  IconPlus,
  IconRefresh,
  IconSearch,
  IconTrash,
  IconX,
} from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, type Edition, type SeriesIndexerView } from '../api'
import BookDialog, { type BookSelection } from '../components/BookDialog'
import { fmtWhen, notify, usePageTitle } from '../ui'
import { QueryFeedback } from '../components/QueryFeedback'
import { EmptyState } from '../components/EmptyState'
import { useConfirmAction } from '../components/ConfirmAction'
import classes from './SeriesPage.module.css'

export default function SeriesPage() {
  const { id } = useParams()
  const seriesID = Number(id)
  const navigate = useNavigate()
  const { confirm, confirmation } = useConfirmAction()

  const [busy, setBusy] = useState(false)
  const [bookSel, setBookSel] = useState<BookSelection | null>(null)
  const [fullDesc, setFullDesc] = useState(false)
  // An edition start from review, or {} for a blank form; null: closed.
  const [addEdition, setAddEdition] = useState<Partial<EditionStart> | null>(null)
  // The open tab lives in ?view= so it survives a reload and can be linked
  // to; ?view=review is the deep link the global review inbox uses.
  const [searchParams, setSearchParams] = useSearchParams()
  const view = searchParams.get('view')
  const tab = view && SERIES_TABS.includes(view) ? view : 'editions'
  const setTab = (next: string | null) =>
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev)
        if (!next || next === 'editions') params.delete('view')
        else params.set('view', next)
        return params
      },
      { replace: true },
    )
  const [organize, setOrganize] = useState(false)

  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['series', seriesID],
    queryFn: () => api.getSeries(seriesID),
  })
  // Same key as the review inbox, so the tab count and the list agree.
  const review = useQuery({
    queryKey: ['review', seriesID],
    queryFn: () => api.review(seriesID),
    refetchInterval: 15000,
  })
  const reviewCount = review.data?.length ?? 0
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
  // Shares the Sources tab's query: an enabled source that was searched and
  // found no confident match needs one chosen by hand.
  const indexersQuery = useQuery({
    queryKey: ['series', seriesID, 'indexers'],
    queryFn: () => api.seriesIndexers(seriesID),
  })
  const unmatchedSources = (indexersQuery.data ?? []).filter(
    (s) =>
      s.enabled &&
      s.bindings?.some((b) => b.state === 'unmatched') &&
      !s.bindings.some((b) => b.state === 'active'),
  ).length

  // Refresh everything under this series (books, files, indexers, detail).
  const load = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['queue'] })
    return queryClient.invalidateQueries({ queryKey: ['series'] })
  }, [queryClient])

  // Editions, Review and Files are three views of the same manager: they
  // share its book, version and file-mapping dialogs.
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
          <Tooltip label="Update series information and check its folder for existing files.">
            <Button
              size="sm"
              variant="default"
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
              Refresh
            </Button>
          </Tooltip>
        </Group>
        <Group gap="md">
          <ProfileBarButton seriesID={seriesID} profileId={series.profileId ?? 0} />
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
                color="red"
                leftSection={<IconTrash size={15} />}
                disabled={busy}
                onClick={() =>
                  confirm({
                    title: 'Remove series?',
                    message: `Remove “${series.title}” from Sento, including monitoring and history. Your library files stay on disk.`,
                    label: 'Remove series',
                    onConfirm: async () => {
                      await api.deleteSeries(seriesID)
                      await queryClient.invalidateQueries({ queryKey: ['series'] })
                      navigate('/')
                    },
                  })
                }
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
      {confirmation}
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
        />
      )}

      <Tabs value={tab} onChange={setTab} keepMounted={false}>
        <Tabs.List>
          <Tabs.Tab value="editions">Editions</Tabs.Tab>
          <Tabs.Tab value="files">Files ({files.length})</Tabs.Tab>
          <Tabs.Tab
            value="review"
            aria-description={reviewCount > 0 ? `${reviewCount} to review` : undefined}
            rightSection={
              reviewCount > 0 ? (
                <Badge aria-hidden size="xs" circle color="yellow">
                  {reviewCount}
                </Badge>
              ) : undefined
            }
          >
            Review
          </Tabs.Tab>
          <Tabs.Tab
            value="sources"
            aria-description={
              unmatchedSources > 0 ? `${unmatchedSources} without a match` : undefined
            }
            rightSection={
              unmatchedSources > 0 ? (
                <Badge aria-hidden size="xs" circle color="orange">
                  {unmatchedSources}
                </Badge>
              ) : undefined
            }
          >
            Sources
          </Tabs.Tab>
          <Tabs.Tab value="history">History</Tabs.Tab>
        </Tabs.List>
        <QueryFeedback error={overviewQuery.error} onRetry={() => void overviewQuery.refetch()} />
        <QueryFeedback error={filesQuery.error} onRetry={() => void filesQuery.refetch()} />

        <Tabs.Panel value="editions" pt="md">
          {editionManager('editions')}
        </Tabs.Panel>
        <Tabs.Panel value="review" pt="md">
          {editionManager('review')}
        </Tabs.Panel>
        <Tabs.Panel value="sources" pt="md">
          <IndexersTab
            seriesID={seriesID}
            seriesTitle={series.title}
            editions={editions}
            run={run}
            busy={busy}
          />
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
        <Tabs.Panel value="history" pt="md">
          <HistoryTab seriesID={seriesID} />
        </Tabs.Panel>
      </Tabs>

      {organize && <OrganizeFiles seriesID={seriesID} onClose={() => setOrganize(false)} />}
      <BookDialog
        seriesID={seriesID}
        sel={bookSel}
        files={files}
        onClose={() => setBookSel(null)}
        onChanged={load}
      />
    </>
  )
}

const eventColor: Record<string, string> = {
  grabbed: 'indigo',
  imported: 'green',
  imported_after_review: 'green',
  import_failed: 'red',
  download_failed: 'red',
  blocklisted: 'orange',
  editions_grouped: 'blue',
}

function HistoryTab({ seriesID }: { seriesID: number }) {
  const {
    data: events,
    error,
    isPending,
    refetch,
  } = useQuery({
    queryKey: ['series', seriesID, 'history'],
    queryFn: () => api.history(seriesID),
  })

  if (isPending || error)
    return <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
  if (events && events.length === 0)
    return (
      <EmptyState
        title="No history yet"
        description="Downloads, imports, and errors for this series will appear here."
      />
    )
  return (
    <Table.ScrollContainer minWidth={580}>
      <Table highlightOnHover verticalSpacing="sm">
        <Table.Thead>
          <Table.Tr>
            <Table.Th>When</Table.Th>
            <Table.Th>Event</Table.Th>
            <Table.Th>Release</Table.Th>
            <Table.Th>Site</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {events?.map((d) => (
            <Table.Tr key={d.id}>
              <Table.Td>
                <Text size="xs" c="dimmed" title={d.createdAt}>
                  {fmtWhen(d.createdAt)}
                </Text>
              </Table.Td>
              <Table.Td>
                <Badge size="xs" color={eventColor[d.body.event] ?? 'gray'}>
                  {d.body.event.replaceAll('_', ' ')}
                  {d.body.force ? ' (forced)' : ''}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Text size="sm" lineClamp={1} title={d.body.title}>
                  {d.body.title}
                </Text>
                {d.body.error && (
                  <Text size="xs" c="red" lineClamp={1} title={d.body.error}>
                    {d.body.error}
                  </Text>
                )}
                {d.body.note && (
                  <Text size="xs" c="dimmed">
                    {d.body.note}
                  </Text>
                )}
              </Table.Td>
              <Table.Td>
                <Text size="xs" c="dimmed">
                  {d.body.provider}
                  {d.body.site ? `/${d.body.site}` : ''}
                </Text>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  )
}

// ProfileBarButton shows the series' release profile in the action bar
// and switches it from a dropdown. profileId 0 = the instance default.
function ProfileBarButton({ seriesID, profileId }: { seriesID: number; profileId: number }) {
  const qc = useQueryClient()
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  const def = useQuery({ queryKey: ['default-profile'], queryFn: api.defaultProfile })
  if (!profiles.data?.length) return null
  const effective = profileId || def.data?.profileId || 0
  const current = profiles.data.find((p) => p.id === effective)
  const assign = (id: number) =>
    api
      .setSeriesProfile(seriesID, id)
      .then(() => qc.invalidateQueries({ queryKey: ['series', seriesID] }))
      .catch(notify.err)
  return (
    <Menu position="bottom-start" withinPortal>
      <Menu.Target>
        <Button
          size="sm"
          variant="subtle"
          color="gray"
          leftSection={<IconAdjustmentsHorizontal size={15} />}
          aria-label="Release profile"
        >
          {current?.name ?? 'Profile'}
          {profileId === 0 && ' (default)'}
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Release profile</Menu.Label>
        {profiles.data.map((p) => (
          <Menu.Item key={p.id} onClick={() => void assign(p.id)}>
            {p.name}
            {p.id === effective ? ' ✓' : ''}
          </Menu.Item>
        ))}
        {profileId !== 0 && (
          <Menu.Item onClick={() => void assign(0)}>Use instance default</Menu.Item>
        )}
      </Menu.Dropdown>
    </Menu>
  )
}

const SERIES_TABS = ['editions', 'files', 'review', 'sources', 'history']

function IndexersTab({
  seriesID,
  seriesTitle,
  editions,
  run,
  busy,
}: {
  seriesID: number
  seriesTitle: string
  editions: Edition[]
  run: (fn: () => Promise<unknown>, msg?: string) => void
  busy: boolean
}) {
  const [rebind, setRebind] = useState<SeriesIndexerView | null>(null)
  const [showDefaults, setShowDefaults] = useState(false)
  const {
    data: sites,
    error,
    isPending,
    refetch,
  } = useQuery({
    queryKey: ['series', seriesID, 'indexers'],
    queryFn: () => api.seriesIndexers(seriesID),
  })

  // Omitted fields decode to nil pointers server-side (= inherit), so
  // undefined and an explicit null are equivalent here. run() invalidates
  // the series queries, which re-fetches this tab.
  const override = (s: SeriesIndexerView, patch: Record<string, boolean | undefined>) =>
    run(() =>
      api.setSeriesIndexerOverride(seriesID, s.id, {
        enabled: s.override?.enabled ?? undefined,
        useVolumes: s.override?.useVolumes ?? undefined,
        useChapters: s.override?.useChapters ?? undefined,
        ...patch,
      }),
    )

  const sources = sites?.filter((s) => s.protocol === 'source') ?? []
  const torrents = sites?.filter((s) => s.protocol !== 'source') ?? []
  // Torrent indexers have nothing series-specific until one is overridden,
  // so the ones still on their defaults fold into a single line.
  const defaults = torrents.filter((s) => !s.override)
  const defaultsOn = defaults.filter((s) => s.enabled).length
  const row = (s: SeriesIndexerView) => (
    <IndexerRow
      key={s.id}
      seriesID={seriesID}
      site={s}
      editions={editions}
      busy={busy}
      run={run}
      override={override}
      onFindMatch={() => setRebind(s)}
    />
  )

  return (
    <Stack gap="lg">
      <Text size="sm" c="dimmed">
        These settings apply to this series only.{' '}
        <Anchor component={Link} to="/indexers" size="sm">
          Manage indexers
        </Anchor>{' '}
        to change the defaults.
      </Text>
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {sites?.length === 0 && !error && (
        <EmptyState
          title="No indexers configured"
          description="Add a torrent indexer or an extension source to find releases."
          action={
            <Button component={Link} to="/indexers" size="sm" variant="default">
              Set up indexers
            </Button>
          }
        />
      )}
      {sources.length > 0 && (
        <IndexerSection
          title="Extension sources"
          hint="A source is searched through the entries matched to this series. Matches are found automatically when you search; untick one that returns the wrong series. Set the edition a match publishes so its releases count as evidence for that edition; a selected download version alone is not evidence."
        >
          {sources.map(row)}
        </IndexerSection>
      )}
      {torrents.length > 0 && (
        <IndexerSection title="Torrent indexers" hint="Searched by title, so there is nothing to match.">
          {torrents.filter((s) => s.override || showDefaults).map(row)}
          {defaults.length > 0 && (
            <UnstyledButton
              className={classes.siteFold}
              aria-expanded={showDefaults}
              onClick={() => setShowDefaults((v) => !v)}
            >
              {showDefaults ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />}
              <Text size="sm" c="dimmed">
                {showDefaults
                  ? 'Hide indexers on their defaults'
                  : `${defaults.length} on ${defaults.length === 1 ? 'its' : 'their'} defaults (${defaultsOn} enabled)`}
              </Text>
            </UnstyledButton>
          )}
        </IndexerSection>
      )}

      {rebind && (
        <RebindModal
          key={rebind.id}
          seriesID={seriesID}
          seriesTitle={seriesTitle}
          site={sites?.find((x) => x.id === rebind.id) ?? rebind}
          busy={busy}
          run={run}
          onClose={() => setRebind(null)}
        />
      )}
    </Stack>
  )
}

function IndexerSection({
  title,
  hint,
  children,
}: {
  title: string
  hint: string
  children: ReactNode
}) {
  return (
    <Stack gap="xs">
      <div>
        <Title order={3} size="h5">
          {title}
        </Title>
        <Text size="xs" c="dimmed">
          {hint}
        </Text>
      </div>
      <div className={classes.sites}>{children}</div>
    </Stack>
  )
}

function IndexerRow({
  seriesID,
  site: s,
  editions,
  busy,
  run,
  override,
  onFindMatch,
}: {
  seriesID: number
  site: SeriesIndexerView
  editions: Edition[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  override: (s: SeriesIndexerView, patch: Record<string, boolean | undefined>) => void
  onFindMatch: () => void
}) {
  return (
    <div className={classes.site} data-off={!s.enabled || undefined}>
      <Switch
        size="sm"
        aria-label={`Enable ${s.name} for this series`}
        disabled={busy}
        checked={s.enabled}
        onChange={(e) => override(s, { enabled: e.currentTarget.checked })}
      />
      <Group gap={6} className={classes.siteName}>
        <Text size="sm" fw={500}>
          {s.name}
        </Text>
        {!s.reachable && (
          <Badge size="xs" color="orange" variant="light">
            unreachable
          </Badge>
        )}
        {s.override && (
          <>
            <Tooltip label="Differs from the indexer's defaults">
              <Badge size="xs" color="grape" variant="light">
                custom
              </Badge>
            </Tooltip>
            <Button
              size="compact-xs"
              variant="subtle"
              disabled={busy}
              onClick={() =>
                override(s, { enabled: undefined, useVolumes: undefined, useChapters: undefined })
              }
            >
              Reset
            </Button>
          </>
        )}
      </Group>
      <Group gap="sm" className={classes.siteLedgers} role="group" aria-label={`Search ${s.name} for`}>
        <Checkbox
          size="xs"
          label="Volumes"
          checked={s.useVolumes}
          disabled={!s.enabled || busy}
          onChange={(e) => override(s, { useVolumes: e.currentTarget.checked })}
        />
        <Checkbox
          size="xs"
          label="Chapters"
          checked={s.useChapters}
          disabled={!s.enabled || busy}
          onChange={(e) => override(s, { useChapters: e.currentTarget.checked })}
        />
      </Group>
      {s.protocol === 'source' && (
        <div className={classes.matches}>
          <BindingList seriesID={seriesID} site={s} editions={editions} run={run} busy={busy} />
          <Button
            size="compact-xs"
            variant="subtle"
            leftSection={<IconPlus size={12} />}
            onClick={onFindMatch}
          >
            {s.bindings?.some((b) => b.state === 'active') ? 'Add match' : 'Find match'}
          </Button>
        </div>
      )}
    </div>
  )
}

// bindingPath makes a binding's key readable. The key is opaque, but sources
// commonly mint {"url": …}; that path is what tells same-titled entries apart.
function bindingPath(key: string): string {
  try {
    const v: unknown = JSON.parse(key)
    if (v && typeof v === 'object' && 'url' in v && typeof v.url === 'string') {
      return decodeURI(v.url)
    }
  } catch {
    /* not JSON: show the key as is */
  }
  return key
}

// BindingList shows a site's bound manga entries: every entry that cleared
// auto-matching is searched in parallel and the funnel picks per book —
// disable an entry only if it produces garbage. The edition picked on an
// entry is the verified mapping that lets its releases count as evidence
// for that edition.
function BindingList({
  seriesID,
  site,
  editions,
  run,
  busy,
}: {
  seriesID: number
  site: SeriesIndexerView
  editions: Edition[]
  run: (fn: () => Promise<unknown>, msg?: string) => void
  busy: boolean
}) {
  const bindings = site.bindings ?? []
  const editionChoices = editions.map((c) => ({
    value: String(c.id),
    label: [c.language, c.editionName, c.publisher, c.medium].filter(Boolean).join(' · '),
  }))
  if (bindings.length === 0) {
    return (
      <Text size="xs" c="dimmed">
        No match yet. One is looked for on the first search.
      </Text>
    )
  }
  return (
    <>
      {bindings.map((b) => {
        if (b.state === 'unmatched') {
          return (
            <Text key={b.id} size="xs" c="orange">
              No confident match{b.mangaTitle ? ` (best: ${b.mangaTitle})` : ''}
            </Text>
          )
        }
        const active = b.state === 'active'
        const path = bindingPath(b.sourceMangaKey)
        return (
          <Fragment key={b.id}>
            <div className={classes.match} data-off={!active || undefined}>
              <Tooltip label={active ? 'Searched. Untick to skip this entry' : 'Skipped. Tick to search it'}>
                <Checkbox
                  size="xs"
                  aria-label={`Search ${b.mangaTitle || 'entry'} ${path}`}
                  checked={active}
                  disabled={busy}
                  onChange={() =>
                    run(() => api.setBindingState(seriesID, b.id, active ? 'disabled' : 'active'))
                  }
                />
              </Tooltip>
              <Text size="sm" className={classes.matchTitle}>
                {b.mangaTitle || 'manga'}
              </Text>
              <Text size="xs" c="dimmed" truncate title={path} className={classes.matchPath}>
                {path}
              </Text>
              {active && (
                <Select
                  size="xs"
                  className={classes.matchEdition}
                  aria-label={`Edition for ${b.mangaTitle} ${path}`}
                  placeholder="Edition not verified"
                  clearable
                  data={editionChoices}
                  value={b.editionId ? String(b.editionId) : null}
                  disabled={busy}
                  onChange={(v) => run(() => api.setBindingEdition(seriesID, b.id, Number(v) || 0))}
                />
              )}
              <Tooltip label="Remove match">
                <ActionIcon
                  size="sm"
                  aria-label={`Remove match ${b.mangaTitle} ${path}`}
                  variant="subtle"
                  color="gray"
                  disabled={busy}
                  onClick={() => run(() => api.deleteBinding(seriesID, b.id))}
                >
                  <IconX size={14} />
                </ActionIcon>
              </Tooltip>
            </div>
            {active && editions.find((c) => c.id === b.editionId)?.type === 'chapter' && (
              <Checkbox
                size="xs"
                className={classes.matchTrust}
                label="Trust new chapters from this listing"
                description="Add observed chapters automatically when chapter discovery is enabled for this edition."
                checked={b.trustChapters}
                disabled={busy}
                onChange={(e) => run(() => api.trustBinding(seriesID, b.id, e.currentTarget.checked))}
              />
            )}
          </Fragment>
        )
      })}
    </>
  )
}

// RebindModal is mounted per site (keyed by its id), so one site's query and
// results can never be offered as matches for another.
function RebindModal({
  seriesID,
  seriesTitle,
  site,
  busy,
  run,
  onClose,
}: {
  seriesID: number
  seriesTitle: string
  site: SeriesIndexerView
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState(seriesTitle)
  // The submitted query drives the search; it starts as the series title so
  // the dialog opens on results.
  const [submitted, setSubmitted] = useState(seriesTitle.trim())
  const search = useQuery({
    queryKey: ['source-search', site.id, submitted],
    queryFn: () => api.sourceSearch(String(site.id), submitted),
    enabled: submitted !== '',
  })
  const results = search.data ?? []

  return (
    <Modal opened onClose={onClose} title={`Find a match on ${site.name}`} size="lg">
      <Stack>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (query.trim() === submitted) void search.refetch()
            else setSubmitted(query.trim())
          }}
        >
          <Group align="flex-end">
            <TextInput
              label="Search"
              data-autofocus
              value={query}
              onChange={(e) => setQuery(e.currentTarget.value)}
              style={{ flex: 1 }}
            />
            <Button type="submit" loading={search.isFetching} disabled={!query.trim()}>
              Search
            </Button>
          </Group>
        </form>
        <QueryFeedback
          pending={search.isLoading}
          error={search.error}
          onRetry={() => void search.refetch()}
        />
        {search.isSuccess && results.length === 0 && (
          <Text size="sm" c="dimmed">
            Nothing on {site.name} for “{submitted}”. Try a shorter or alternative title.
          </Text>
        )}
        {results.length > 0 && (
          <div className={classes.sites}>
            {results.map((m) => {
              const isBound = site.bindings?.some(
                (b) => b.sourceMangaKey === m.key && b.state !== 'unmatched',
              )
              const path = bindingPath(m.key)
              return (
                <div key={m.key} className={classes.result}>
                  <div className={classes.resultBody}>
                    <Text size="sm">{m.title}</Text>
                    <Text size="xs" c="dimmed" className={classes.resultPath}>
                      {path}
                    </Text>
                  </div>
                  <Button
                    size="compact-xs"
                    variant={isBound ? 'default' : 'filled'}
                    disabled={isBound || busy}
                    aria-label={`${isBound ? 'Matched' : 'Use this match'}: ${m.title} ${path}`}
                    onClick={() =>
                      run(
                        () =>
                          api.addBinding(seriesID, {
                            sourceId: String(site.id),
                            sourceMangaKey: m.key,
                            sourceName: site.name,
                            mangaTitle: m.title,
                          }),
                        'Source match added',
                      )
                    }
                  >
                    {isBound ? 'Matched' : 'Use this match'}
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </Stack>
    </Modal>
  )
}
