import EditionManager, { type EditionSection } from '../components/EditionManager'
import { EditorModal, Relationships } from '../components/EditionDialogs'
import SourceMappings from '../components/SourceMappings'
import OrganizeFiles from '../components/OrganizeFiles'
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
  NumberInput,
  SegmentedControl,
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
  IconChevronLeft,
  IconDotsVertical,
  IconEye,
  IconEyeOff,
  IconRefresh,
  IconSearch,
  IconTrash,
  IconX,
} from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, type Collection, type SeriesIndexerView } from '../api'
import SlotModal, { type SlotSelection } from '../components/SlotModal'
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
  const [slotSel, setSlotSel] = useState<SlotSelection | null>(null)
  const [fullDesc, setFullDesc] = useState(false)
  const [addCollection, setAddCollection] = useState(false)
  // ?view=review is the deep link the global review inbox uses.
  const [tab, setTab] = useState<string | null>(
    new URLSearchParams(window.location.search).get('view') === 'review' ? 'review' : 'editions',
  )
  const [relationships, setRelationships] = useState(false)
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
  const allSlots = useQuery({
    queryKey: ['series', seriesID, 'target-slots'],
    queryFn: () => api.targetSlots(seriesID),
    enabled: relationships,
  })
  const slotsQuery = useQuery({
    queryKey: ['series', seriesID, 'edition-overview'],
    queryFn: () => api.editionOverview(seriesID),
  })
  const filesQuery = useQuery({
    queryKey: ['series', seriesID, 'files'],
    queryFn: () => api.files(seriesID),
  })

  const series = detail.data?.series ?? null
  usePageTitle(series?.title ?? '')
  const collections = detail.data?.collections ?? []
  const slots = slotsQuery.data ?? null
  const files = filesQuery.data ?? []

  // Refresh everything under this series (slots, files, indexers, detail).
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
      collections={collections}
      files={files}
      onSlot={setSlotSel}
      onAddEdition={() => setAddCollection(true)}
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

  const owned = slots?.booksOnDisk ?? 0
  const wanted = slots?.wantedCopies ?? 0

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
            {slots && (
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
                    Copies wanted
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
              <Menu.Item onClick={() => setRelationships(true)}>Content relationships</Menu.Item>
              <Menu.Divider />
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
      {addCollection && (
        <AddEditionModal
          seriesID={seriesID}
          collections={collections}
          busy={busy}
          run={run}
          onClose={() => setAddCollection(false)}
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
          <Tabs.Tab value="indexers">Indexers</Tabs.Tab>
          <Tabs.Tab value="history">History</Tabs.Tab>
        </Tabs.List>
        <QueryFeedback error={slotsQuery.error} onRetry={() => void slotsQuery.refetch()} />
        <QueryFeedback error={filesQuery.error} onRetry={() => void filesQuery.refetch()} />

        <Tabs.Panel value="editions" pt="md">
          {editionManager('editions')}
        </Tabs.Panel>
        <Tabs.Panel value="review" pt="md">
          {editionManager('review')}
        </Tabs.Panel>
        <Tabs.Panel value="indexers" pt="md">
          <Stack gap="lg">
            <IndexersTab seriesID={seriesID} seriesTitle={series.title} run={run} busy={busy} />
            <SourceMappings seriesID={seriesID} collections={collections} />
          </Stack>
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
                          `Scan: ${r.scanned} new files, ${r.linked} linked, ${r.unrequested} unrequested`,
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
      {relationships && (
        <Relationships
          seriesID={seriesID}
          units={Object.values(allSlots.data ?? {}).flat()}
          collections={collections}
          close={() => setRelationships(false)}
        />
      )}
      <SlotModal
        seriesID={seriesID}
        sel={slotSel}
        files={files}
        onClose={() => setSlotSel(null)}
        onChanged={load}
      />
    </>
  )
}

const eventColor: Record<string, string> = {
  grabbed: 'indigo',
  imported: 'green',
  import_failed: 'red',
  download_failed: 'red',
  blocklisted: 'orange',
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

// AddCollectionModal creates a custom collection: based on an existing one
// (clones the volume numbering — language variants) or from scratch with
// a volume count (collections MangaBaka doesn't know yet). Metadata refresh
// keeps local editions independent until explicitly linked to the catalog.
function AddEditionModal({
  seriesID,
  collections,
  busy,
  run,
  onClose,
}: {
  seriesID: number
  collections: Collection[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onClose: () => void
}) {
  const [ctype, setCtype] = useState<'volume' | 'chapter'>('volume')
  const [name, setName] = useState('Standard Edition')
  const [language, setLanguage] = useState('')
  const [publisher, setPublisher] = useState('')
  const [medium, setMedium] = useState('')
  const [basedOn, setBasedOn] = useState<string | null>(null)
  const [volumes, setVolumes] = useState<number | string>(0)
  const [cutoff, setCutoff] = useState<number | string>(0)

  const create = () =>
    run(
      () =>
        api
          .createCollection(seriesID, {
            editionName: name,
            language,
            publisher,
            medium,
            type: ctype,
            basedOn: basedOn ? Number(basedOn) : 0,
            volumes: ctype === 'volume' && !basedOn ? Number(volumes) || 0 : 0,
            cutoffNum: ctype === 'chapter' ? Math.round((Number(cutoff) || 0) * 100) : 0,
            monitored: true,
          })
          .then(onClose),
      'Edition added',
    )

  return (
    <EditorModal
      dirty={
        name !== 'Standard Edition' ||
        !!language ||
        !!publisher ||
        !!medium ||
        !!basedOn ||
        Number(volumes) !== 0 ||
        Number(cutoff) !== 0 ||
        ctype !== 'volume'
      }
      busy={busy}
      onClose={onClose}
      title="Add edition"
      size="md"
    >
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          Add a language or publication edition missing from MangaBaka. To keep another release
          group for an existing edition, add a collected version instead.
        </Text>
        <SegmentedControl
          size="xs"
          value={ctype}
          onChange={(v) => {
            const t = v as 'volume' | 'chapter'
            setCtype(t)
            setBasedOn(null)
            // Swap the default name when the user hasn't typed their own.
            if (t === 'chapter' && name === 'Standard Edition') setName('Chapters')
            if (t === 'volume' && name === 'Chapters') setName('Standard Edition')
          }}
          data={[
            { value: 'volume', label: 'Volumes' },
            { value: 'chapter', label: 'Chapters' },
          ]}
        />
        {
          <Select
            label="Copy numbering from"
            description="Copy unit labels only. Dates and catalog references are kept separate."
            placeholder="From scratch"
            clearable
            data={collections
              .filter((c) => c.type === ctype)
              .map((c) => ({
                value: String(c.id),
                label: [c.editionName, c.language, c.medium].filter(Boolean).join(' · '),
              }))}
            value={basedOn}
            onChange={setBasedOn}
          />
        }
        {ctype === 'volume' && !basedOn && (
          <NumberInput
            label="Volumes"
            description="Leave at 0 to add volumes individually; only enter a count you know."
            min={0}
            max={500}
            value={volumes}
            onChange={setVolumes}
          />
        )}
        {ctype === 'chapter' && (
          <Text size="sm" c="dimmed">
            This edition starts with explicit units. Add chapters individually, or enable
            count-based numbering in Edit edition.
          </Text>
        )}
        {ctype === 'chapter' && (
          <NumberInput
            label="Skip through chapter"
            description="Only chapters above this number are collected automatically. You can collect chapter zero from its book list."
            min={0}
            value={cutoff}
            onChange={setCutoff}
          />
        )}
        <TextInput
          label="Edition name"
          value={name}
          onChange={(e) => setName(e.currentTarget.value)}
        />
        <TextInput
          label="Language"
          placeholder="fr"
          value={language}
          onChange={(e) => setLanguage(e.currentTarget.value)}
        />
        <Group grow>
          <TextInput
            label="Publisher (optional)"
            value={publisher}
            onChange={(e) => setPublisher(e.currentTarget.value)}
          />
          <TextInput
            label="Medium (optional)"
            placeholder="paperback"
            value={medium}
            onChange={(e) => setMedium(e.currentTarget.value)}
          />
        </Group>
        <Button mt="xs" loading={busy} disabled={!name.trim() || !language.trim()} onClick={create}>
          Add edition
        </Button>
      </Stack>
    </EditorModal>
  )
}

function IndexersTab({
  seriesID,
  seriesTitle,
  run,
  busy,
}: {
  seriesID: number
  seriesTitle: string
  run: (fn: () => Promise<unknown>, msg?: string) => void
  busy: boolean
}) {
  const [rebind, setRebind] = useState<SeriesIndexerView | null>(null)
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

  return (
    <Stack>
      <Text size="sm" c="dimmed">
        These settings apply to this series.{' '}
        <Anchor component={Link} to="/indexers" size="sm">
          Manage indexers
        </Anchor>{' '}
        to change the defaults. Source matches are found automatically when you search; you can also
        choose them below.
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
      {!!sites?.length && (
        <Table.ScrollContainer minWidth={700}>
          <Table>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Site</Table.Th>
                <Table.Th>Enabled</Table.Th>
                <Table.Th>Volumes</Table.Th>
                <Table.Th>Chapters</Table.Th>
                <Table.Th>Source matches</Table.Th>
                <Table.Th></Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {sites?.map((s) => {
                const overridden = !!s.override
                return (
                  <Table.Tr key={s.id} opacity={s.reachable ? 1 : 0.5}>
                    <Table.Td>
                      <Group gap={6}>
                        <Text size="sm">{s.name}</Text>
                        <Badge
                          size="xs"
                          variant="outline"
                          color={s.protocol === 'torznab' ? 'blue' : 'teal'}
                        >
                          {s.protocol}
                        </Badge>
                        {overridden && (
                          <Badge size="xs" color="grape" variant="light">
                            override
                          </Badge>
                        )}
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      <Switch
                        size="xs"
                        aria-label={`Enable ${s.name} for this series`}
                        disabled={busy}
                        checked={s.enabled}
                        onChange={(e) => override(s, { enabled: e.currentTarget.checked })}
                      />
                    </Table.Td>
                    <Table.Td>
                      <Switch
                        size="xs"
                        aria-label={`Search volumes on ${s.name}`}
                        checked={s.useVolumes}
                        disabled={!s.enabled || busy}
                        onChange={(e) => override(s, { useVolumes: e.currentTarget.checked })}
                      />
                    </Table.Td>
                    <Table.Td>
                      <Switch
                        size="xs"
                        aria-label={`Search chapters on ${s.name}`}
                        checked={s.useChapters}
                        disabled={!s.enabled || busy}
                        onChange={(e) => override(s, { useChapters: e.currentTarget.checked })}
                      />
                    </Table.Td>
                    <Table.Td>
                      {s.protocol === 'source' ? (
                        <BindingList seriesID={seriesID} site={s} run={run} />
                      ) : (
                        <Text size="xs" c="dimmed">
                          —
                        </Text>
                      )}
                    </Table.Td>
                    <Table.Td>
                      <Group gap={4} justify="flex-end">
                        {overridden && (
                          <Button
                            size="compact-xs"
                            variant="subtle"
                            onClick={() =>
                              override(s, {
                                enabled: undefined,
                                useVolumes: undefined,
                                useChapters: undefined,
                              })
                            }
                          >
                            Reset
                          </Button>
                        )}
                        {s.protocol === 'source' && (
                          <Button size="compact-xs" variant="default" onClick={() => setRebind(s)}>
                            {s.bindings?.some((b) => b.state === 'active')
                              ? 'Add match'
                              : 'Find match'}
                          </Button>
                        )}
                      </Group>
                    </Table.Td>
                  </Table.Tr>
                )
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}

      <RebindModal
        seriesID={seriesID}
        seriesTitle={seriesTitle}
        site={rebind && (sites?.find((x) => x.id === rebind.id) ?? rebind)}
        busy={busy}
        run={run}
        onClose={() => setRebind(null)}
      />
    </Stack>
  )
}

// BindingList shows a site's bound manga entries: every entry that cleared
// auto-matching is searched in parallel and the funnel picks per slot —
// disable an entry only if it produces garbage.
function BindingList({
  seriesID,
  site,
  run,
}: {
  seriesID: number
  site: SeriesIndexerView
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const bindings = site.bindings ?? []
  if (bindings.length === 0) {
    return (
      <Text size="xs" c="dimmed">
        Found on first search
      </Text>
    )
  }
  return (
    <Stack gap={2}>
      {bindings.map((b) =>
        b.state === 'unmatched' ? (
          <Text key={b.id} size="xs" c="dimmed">
            no confident match{b.mangaTitle ? ` (best: ${b.mangaTitle})` : ''}
          </Text>
        ) : (
          <Group key={b.id} gap={4} wrap="nowrap">
            <Text size="xs" c={b.state === 'active' ? undefined : 'dimmed'} truncate maw={220}>
              {b.mangaTitle || 'manga'}{' '}
              <Text span size="xs" c="dimmed">
                #{b.sourceMangaKey}
              </Text>
            </Text>
            <Tooltip label={b.state === 'active' ? 'Disable entry' : 'Enable entry'}>
              <ActionIcon
                size="md"
                aria-label={`${b.state === 'active' ? 'Disable' : 'Enable'} match ${b.mangaTitle}`}
                variant="subtle"
                color={b.state === 'active' ? 'gray' : 'yellow'}
                onClick={() =>
                  run(() =>
                    api.setBindingState(
                      seriesID,
                      b.id,
                      b.state === 'active' ? 'disabled' : 'active',
                    ),
                  )
                }
              >
                {b.state === 'active' ? <IconEye size={12} /> : <IconEyeOff size={12} />}
              </ActionIcon>
            </Tooltip>
            <Tooltip label="Remove source match">
              <ActionIcon
                size="md"
                aria-label={`Remove match ${b.mangaTitle}`}
                variant="subtle"
                color="red"
                onClick={() => run(() => api.deleteBinding(seriesID, b.id))}
              >
                <IconX size={12} />
              </ActionIcon>
            </Tooltip>
          </Group>
        ),
      )}
    </Stack>
  )
}

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
  site: SeriesIndexerView | null
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState(seriesTitle)
  const [results, setResults] = useState<{ key: string; title: string }[]>([])

  if (!site) return null

  return (
    <Modal opened onClose={onClose} title={`Find a match on ${site.name}`} size="lg">
      <Stack>
        <Group align="flex-end">
          <TextInput
            label="Search"
            value={query}
            onChange={(e) => setQuery(e.currentTarget.value)}
            style={{ flex: 1 }}
          />
          <Button
            loading={busy}
            onClick={() =>
              run(() =>
                api
                  .sourceSearch(String(site.id), query)
                  .then((r) => setResults(r.map((m) => ({ key: m.key, title: m.title })))),
              )
            }
          >
            Search
          </Button>
        </Group>
        {results.map((m) => {
          const isBound = site.bindings?.some(
            (b) => b.sourceMangaKey === m.key && b.state !== 'unmatched',
          )
          return (
            <Group key={m.key} justify="space-between">
              <Text size="sm">{m.title}</Text>
              <Button
                size="compact-xs"
                disabled={isBound}
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
            </Group>
          )
        })}
      </Stack>
    </Modal>
  )
}
