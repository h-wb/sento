import { useCallback, useState } from 'react'
import {
  Anchor,
  ActionIcon,
  Badge,
  Button,
  Card,
  Collapse,
  Group,
  Image,
  Menu,
  Modal,
  NumberInput,
  Popover,
  Progress,
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
  IconChevronRight,
  IconDotsVertical,
  IconEye,
  IconEyeOff,
  IconLayoutGrid,
  IconListSearch,
  IconRefresh,
  IconScissors,
  IconSearch,
  IconTable,
  IconTrash,
  IconVersions,
  IconX,
} from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  api,
  fmtNum,
  fmtSize,
  type ChapterSlot,
  type Collection,
  type SeriesFile,
  type SeriesIndexerView,
  type Slots,
  type VolumeSlot,
} from '../api'
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
  const [manageCollections, setManageCollections] = useState(false)
  const [addCollection, setAddCollection] = useState(false)
  const [tab, setTab] = useState<string | null>('volumes')
  const [volumesView, setVolumesView] = useLedgerView('sento.volumesView')
  const [chaptersView, setChaptersView] = useLedgerView('sento.chaptersView')

  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['series', seriesID],
    queryFn: () => api.getSeries(seriesID),
  })
  const slotsQuery = useQuery({
    queryKey: ['series', seriesID, 'slots'],
    queryFn: () => api.slots(seriesID),
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
  const load = useCallback(
    () => {
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      return queryClient.invalidateQueries({ queryKey: ['series'] })
    },
    [queryClient],
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
    if (detail.error) return <QueryFeedback error={detail.error} onRetry={() => void detail.refetch()} />
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

  const volumeCollections = collections.filter((c) => c.type === 'volume')
  const allSlots = [...Object.values(slots?.volumes ?? {}).flat(), ...Object.values(slots?.chapters ?? {}).flat()]
  const owned = allSlots.filter((s) => s.fileIds?.length).length
  const wanted = allSlots.filter((s) => s.wanted).length

  return (
    <>
      <Anchor component={Link} to="/" size="xs" c="dimmed" underline="hover" className={classes.back}>
        <IconChevronLeft size={13} /> Library
      </Anchor>
      <div className={classes.hero}>
        <Image src={series.coverUrl} className={classes.cover} alt={series.title}
          fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='240'%3E%3Crect width='160' height='240' fill='%2352524e'/%3E%3C/svg%3E" />
        <div className={classes.details}>
          <Title order={2} className={classes.title}>{series.title}</Title>
          <Group gap={8} mt={10}>
            <Text size="sm" c="dimmed" tt="capitalize">{[series.type, series.status?.replaceAll('_', ' '), series.year].filter(Boolean).join(' · ')}</Text>
            {series.contentRating && series.contentRating !== 'safe' && <Badge size="xs" color="gray">{series.contentRating}</Badge>}
          </Group>
          {series.description && <>
            <Text size="sm" c="dimmed" mt="md" lh={1.65} lineClamp={fullDesc ? undefined : 3} id="series-description">{series.description}</Text>
            <UnstyledButton onClick={() => setFullDesc((v) => !v)} aria-expanded={fullDesc} aria-controls="series-description" mt={5}>
              <Text size="xs" fw={500}>{fullDesc ? 'Show less' : 'Read more'}</Text>
            </UnstyledButton>
          </>}
          <Group gap={24} mt="lg" className={classes.stats}>
            {series.finalVolume > 0 && <div><Text size="lg" fw={500}>{series.finalVolume}</Text><Text size="xs" c="dimmed">Volumes</Text></div>}
            {series.totalChapters > 0 && <div><Text size="lg" fw={500}>{series.totalChapters}</Text><Text size="xs" c="dimmed">Chapters</Text></div>}
            {slots && <><div><Text size="lg" fw={500}>{owned}</Text><Text size="xs" c="dimmed">Owned</Text></div>
              <div><Text size="lg" fw={500}>{wanted}</Text><Text size="xs" c="dimmed">Wanted</Text></div></>}
          </Group>
        </div>
      </div>
      <div className={classes.actions}>
        <Group gap="xs">
          <Tooltip label="Search monitored collections and download the best matching releases automatically.">
            <Button size="sm" leftSection={<IconSearch size={15} />} loading={busy} disabled={!series.monitored}
              onClick={() => run(async () => {
                const [v, c] = await Promise.all([api.search(seriesID, { ledger: 'volume' }), api.search(seriesID, { ledger: 'chapter' })])
                const queued = (v.queued?.length ?? 0) + (c.queued?.length ?? 0)
                notify.ok(queued ? `${queued} ${queued === 1 ? 'release' : 'releases'} queued` : 'No matching releases to download')
              })}>Search & download</Button>
          </Tooltip>
          <Tooltip label="Update series information and check its folder for existing files.">
            <Button size="sm" variant="default" leftSection={<IconRefresh size={15} />} disabled={busy}
              onClick={() => run(async () => {
                await api.refreshSeries(seriesID)
                const r = await api.scanSeries(seriesID)
                notify.ok(`Series refreshed · ${r.linked} files linked`)
              })}>Refresh</Button>
          </Tooltip>
          <Button size="sm" variant="default" leftSection={<IconVersions size={15} />} onClick={() => setManageCollections(true)}>Collections</Button>
        </Group>
        <Group gap="md">
          <ProfileBarButton seriesID={seriesID} profileId={series.profileId ?? 0} />
          <Tooltip label="Allow Sento to search for and download missing releases for this series.">
            <Switch size="sm" label="Monitor series" checked={series.monitored} disabled={busy}
              onChange={(e) => { const monitored = e.currentTarget.checked; void run(() => api.setSeriesMonitored(seriesID, monitored)) }} />
          </Tooltip>
          <Menu position="bottom-end">
            <Menu.Target><ActionIcon size="lg" variant="subtle" color="gray" aria-label="Series actions"><IconDotsVertical size={18} /></ActionIcon></Menu.Target>
            <Menu.Dropdown><Menu.Item color="red" leftSection={<IconTrash size={15} />} disabled={busy}
              onClick={() => confirm({ title: 'Remove series?', message: `Remove “${series.title}” from Sento, including monitoring and history. Your library files stay on disk.`, label: 'Remove series',
                onConfirm: async () => { await api.deleteSeries(seriesID); await queryClient.invalidateQueries({ queryKey: ['series'] }); navigate('/') } })}>Remove series</Menu.Item></Menu.Dropdown>
          </Menu>
        </Group>
      </div>
      {!series.monitored && <Text size="sm" c="dimmed" mb="lg">Automatic downloads are paused for this series. You can still browse releases from its collections.</Text>}
      {confirmation}
      <CollectionsModal
        seriesID={seriesID}
        collections={collections}
        slots={slots}
        opened={manageCollections}
        run={run}
        onAdd={() => setAddCollection(true)}
        onClose={() => setManageCollections(false)}
      />
      <AddCollectionModal
        seriesID={seriesID}
        collections={volumeCollections}
        opened={addCollection}
        busy={busy}
        run={run}
        onClose={() => setAddCollection(false)}
      />

      <Tabs value={tab} onChange={setTab} keepMounted={false}>
        <Tabs.List>
          <Tabs.Tab value="volumes">Volumes</Tabs.Tab>
          <Tabs.Tab value="chapters">Chapters</Tabs.Tab>
          <Tabs.Tab value="indexers">Indexers</Tabs.Tab>
          <Tabs.Tab value="files">Files ({files.length})</Tabs.Tab>
          <Tabs.Tab value="history">History</Tabs.Tab>
        </Tabs.List>
        {(tab === 'volumes' || tab === 'chapters') && <Group justify="flex-end" mt="md">
          <ViewToggle value={tab === 'volumes' ? volumesView : chaptersView} onChange={tab === 'volumes' ? setVolumesView : setChaptersView} />
        </Group>}
        <QueryFeedback error={slotsQuery.error} onRetry={() => void slotsQuery.refetch()} />
        <QueryFeedback error={filesQuery.error} onRetry={() => void filesQuery.refetch()} />

        <Tabs.Panel value="volumes" pt="md">
          <QueryFeedback pending={slotsQuery.isPending} />
          {!slotsQuery.isPending && !slotsQuery.error && <VolumesTab
            seriesID={seriesID}
            collections={collections}
            slots={slots}
            files={files}
            busy={busy}
            run={run}
            onSlot={setSlotSel}
            onManageCollections={() => setManageCollections(true)}
            view={volumesView}
          />}
        </Tabs.Panel>

        <Tabs.Panel value="chapters" pt="md">
          <QueryFeedback pending={slotsQuery.isPending} />
          {!slotsQuery.isPending && !slotsQuery.error && <ChaptersTab
            seriesID={seriesID}
            collections={collections}
            slots={slots}
            files={files}
            busy={busy}
            run={run}
            onSlot={setSlotSel}
            onManageCollections={() => setManageCollections(true)}
            view={chaptersView}
          />}
        </Tabs.Panel>

        <Tabs.Panel value="indexers" pt="md">
          <IndexersTab seriesID={seriesID} seriesTitle={series.title} run={run} busy={busy} />
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
            </Group>
            {files.map((f) => (
              <Card key={f.id} withBorder padding="xs">
                <Group justify="space-between" wrap="nowrap">
                  <Text size="xs" style={{ wordBreak: 'break-all' }}>
                    {f.path}
                  </Text>
                  <Group gap={6} wrap="nowrap">
                    <Badge size="xs" variant="outline">
                      {f.provenance}
                    </Badge>
                    <Text size="xs" c="dimmed">
                      {fmtSize(f.size)}
                    </Text>
                  </Group>
                </Group>
              </Card>
            ))}
            {filesQuery.isPending && <QueryFeedback pending />}
            {!filesQuery.isPending && !filesQuery.error && files.length === 0 && <EmptyState title="No files yet" description="Download a release or scan the series folder to add existing files." />}
          </Stack>
        </Tabs.Panel>
        <Tabs.Panel value="history" pt="md">
          <HistoryTab seriesID={seriesID} />
        </Tabs.Panel>
      </Tabs>

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
  const { data: events, error, isPending, refetch } = useQuery({
    queryKey: ['series', seriesID, 'history'],
    queryFn: () => api.history(seriesID),
  })

  if (isPending || error) return <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
  if (events && events.length === 0)
    return <EmptyState title="No history yet" description="Downloads, imports, and errors for this series will appear here." />
  return (
    <Table.ScrollContainer minWidth={580}><Table highlightOnHover verticalSpacing="sm">
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
    </Table></Table.ScrollContainer>
  )
}

// CutoffButton edits a chapter collection's cutoff ("monitor from N
// onward") from the section header.
function CutoffButton({
  seriesID,
  collection: c,
  run,
}: {
  seriesID: number
  collection: Collection
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const [opened, setOpened] = useState(false)
  const [cutoff, setCutoff] = useState<number | ''>(c.cutoffNum / 100)
  // Adjust-during-render: re-sync when the collection (re)loads.
  const [prevCutoff, setPrevCutoff] = useState(c.cutoffNum)
  if (c.cutoffNum !== prevCutoff) {
    setPrevCutoff(c.cutoffNum)
    setCutoff(c.cutoffNum / 100)
  }
  return (
    <Popover opened={opened} onChange={setOpened} position="bottom-start" withinPortal trapFocus>
      <Popover.Target>
        <Button
          size="compact-xs"
          variant="subtle"
          color="gray"
          leftSection={<IconScissors size={14} />}
          onClick={(e) => {
            e.stopPropagation()
            setOpened((o) => !o)
          }}
        >
          Cutoff: c{fmtNum(c.cutoffNum)}
        </Button>
      </Popover.Target>
      <Popover.Dropdown onClick={(e) => e.stopPropagation()}>
        <Stack gap="xs" w={220}>
          <NumberInput
            label="Monitor from chapter"
            description="Chapters below the cutoff are ignored"
            size="xs"
            value={cutoff}
            onChange={(v) => setCutoff(typeof v === 'number' ? v : '')}
          />
          <Button
            size="xs"
            onClick={() => {
              setOpened(false)
              run(
                () =>
                  api.updateCollection(seriesID, c.id, {
                    cutoffNum: Math.round(Number(cutoff || 0) * 100),
                  }),
                'Cutoff updated',
              )
            }}
          >
            Save
          </Button>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}

// Both ledgers render as a detail table or the compact chip grid; the
// pick persists per ledger in localStorage.
type LedgerView = 'table' | 'chips'

function useLedgerView(key: string): [LedgerView, (v: LedgerView) => void] {
  const [view, setView] = useState<LedgerView>(() =>
    localStorage.getItem(key) === 'chips' ? 'chips' : 'table',
  )
  return [
    view,
    (v: LedgerView) => {
      localStorage.setItem(key, v)
      setView(v)
    },
  ]
}

function ViewToggle({ value, onChange }: { value: LedgerView; onChange: (v: LedgerView) => void }) {
  return (
    <SegmentedControl
      size="xs"
      value={value}
      onChange={(v) => onChange(v as LedgerView)}
      aria-label="Collection layout"
      data={[
        { value: 'table', label: <Group gap={6}><IconTable size={14} /><span>Table</span></Group> },
        { value: 'chips', label: <Group gap={6}><IconLayoutGrid size={14} /><span>Grid</span></Group> },
      ]}
    />
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
        <Button size="sm" variant="subtle" color="gray" leftSection={<IconAdjustmentsHorizontal size={15} />} aria-label="Release profile">
          {current?.name ?? 'Profile'}{profileId === 0 && ' (default)'}
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

// chipProps maps a slot's state to the badge styling of the compact grid.
function chipProps(s: { fileIds?: number[]; wanted: boolean }) {
  return {
    variant: s.fileIds?.length ? 'filled' : s.wanted ? 'outline' : 'light',
    color: s.fileIds?.length ? 'green' : s.wanted ? 'yellow' : 'gray',
  } as const
}

// StatusBadge renders a slot's state in the ledger tables.
function StatusBadge({
  slot,
}: {
  slot: { fileIds?: number[]; wanted: boolean; monitored: boolean; released?: boolean }
}) {
  if (slot.fileIds?.length)
    return (
      <Badge size="xs" variant="filled" color="green">
        owned
      </Badge>
    )
  if (slot.wanted)
    return (
      <Badge size="xs" variant="outline" color="yellow">
        wanted
      </Badge>
    )
  if (!slot.monitored)
    return (
      <Badge size="xs" variant="light" color="gray">
        unmonitored
      </Badge>
    )
  if (slot.released === false)
    return (
      <Badge size="xs" variant="light" color="gray">
        unreleased
      </Badge>
    )
  return (
    <Badge size="xs" variant="light" color="gray">
      missing
    </Badge>
  )
}

// slotFileText summarizes a slot's linked files ("2 files · 312.4 MiB").
function slotFileText(fileIds: number[] | undefined, fileById: Map<number, SeriesFile>): string {
  if (!fileIds?.length) return ''
  const size = fileIds.reduce((t, id) => t + (fileById.get(id)?.size ?? 0), 0)
  return `${fileIds.length > 1 ? `${fileIds.length} files · ` : ''}${fmtSize(size)}`
}

// SlotActions: Sonarr's per-row pair — auto search (run the funnel for
// this slot and grab the best release) and interactive search (pick
// yourself in the slot modal).
function SlotActions({
  seriesID,
  ledger,
  target,
  busy,
  run,
  onInteractive,
  autoLabel = 'Search and download the best matching release',
}: {
  seriesID: number
  ledger: 'volume' | 'chapter'
  target: { collectionId: number; number: number }
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onInteractive: () => void
  autoLabel?: string
}) {
  // Only the clicked row's button spins (busy is page-wide).
  const [clicked, setClicked] = useState(false)
  if (!busy && clicked) setClicked(false)
  return (
    <Group gap={2} wrap="nowrap" justify="flex-end">
      <Tooltip label={autoLabel}>
        <ActionIcon
          size="sm"
          variant="subtle"
          loading={busy && clicked}
          disabled={busy}
          aria-label={`Search and download ${ledger} ${target.number ? fmtNum(target.number) : 'collection'}`}
          onClick={(e) => {
            e.stopPropagation()
            setClicked(true)
            run(async () => {
              const r = await api.search(seriesID, { ledger, slot: target })
              notify.ok(
                r.queued?.length
                  ? `Grabbed ${r.queued.length} release${r.queued.length > 1 ? 's' : ''}`
                  : 'No matching releases to download',
              )
            })
          }}
        >
          <IconSearch size={14} />
        </ActionIcon>
      </Tooltip>
      <Tooltip label="Interactive search & files">
        <ActionIcon
          size="sm"
          variant="subtle"
          aria-label={`Browse releases for ${ledger} ${target.number ? fmtNum(target.number) : 'collection'}`}
          onClick={(e) => {
            e.stopPropagation()
            onInteractive()
          }}
        >
          <IconListSearch size={14} />
        </ActionIcon>
      </Tooltip>
    </Group>
  )
}

function VolumesTab({
  seriesID,
  collections,
  slots,
  files,
  busy,
  run,
  onSlot,
  onManageCollections,
  view,
}: {
  seriesID: number
  collections: Collection[]
  slots: Slots | null
  files: SeriesFile[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
  onManageCollections: () => void
  view: LedgerView
}) {
  const volumeCollections = collections.filter((c) => c.type === 'volume')
  // Only monitored collections show here — the ones being collected. The
  // rest (reference data) live in the Collections manager on the action bar.
  const monitoredCols = volumeCollections.filter((c) => c.monitored)
  const fileById = new Map(files.map((f) => [f.id, f]))

  return (
    <Stack>
      {monitoredCols.length === 0 && (
        <Text size="sm" c="dimmed">
          No monitored collection —{' '}
          <Anchor component="button" type="button" size="sm" onClick={onManageCollections}>choose a collection</Anchor>{' '}
          to pick the one you collect.
        </Text>
      )}
      {monitoredCols.map((c) => (
        <CollectionSection
          key={c.id}
          seriesID={seriesID}
          collection={c}
          colSlots={slots?.volumes?.[String(c.id)] ?? []}
          fileById={fileById}
          defaultOpen={monitoredCols.length === 1}
          view={view}
          busy={busy}
          run={run}
          onSlot={onSlot}
        />
      ))}
    </Stack>
  )
}

// CollectionSection is one monitored collection as a collapsible card (Sonarr's
// seasons): header with progress always visible, the volume table behind
// the chevron — expanded by default only when it's the lone collection.
function CollectionSection({
  seriesID,
  collection: c,
  colSlots,
  fileById,
  defaultOpen,
  view,
  busy,
  run,
  onSlot,
}: {
  seriesID: number
  collection: Collection
  colSlots: VolumeSlot[]
  fileById: Map<number, SeriesFile>
  defaultOpen: boolean
  view: LedgerView
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
}) {
  const [open, setOpen] = useState(defaultOpen)
  const owned = colSlots.filter((s) => (s.fileIds?.length ?? 0) > 0).length
  const openSlot = (s: VolumeSlot, autoSearch = false) =>
    onSlot({
      ledger: 'volume',
      number: s.number,
      collectionId: c.id,
      collectionName: `${c.editionName} · ${c.language} · ${c.medium}`,
      releaseDate: s.releaseDate,
      monitored: s.monitored,
      wanted: s.wanted,
      fileIds: s.fileIds ?? [],
      autoSearch,
    })

  return (
    <Card withBorder padding="sm">
      <Group justify="space-between" align="flex-start">
        <UnstyledButton onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        <Group gap={8} wrap="nowrap">
          <IconChevronRight
            size={16}
            style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform 150ms' }}
          />
          <div>
            <Text fw={600}>
              {c.editionName}{' '}
              <Text span c="dimmed" size="sm">
                {[c.language, c.medium, c.publisher].filter(Boolean).join(' · ')}
              </Text>{' '}
              {c.custom && (
                <Badge size="xs" color="grape" variant="light">
                  custom
                </Badge>
              )}
            </Text>
            <Text size="xs" c="dimmed">
              {owned}/{colSlots.length} owned
            </Text>
          </div>
        </Group></UnstyledButton>
        <SlotActions
          seriesID={seriesID}
          ledger="volume"
          target={{ collectionId: c.id, number: 0 }}
          busy={busy}
          run={run}
          autoLabel="Search and download wanted volumes"
          onInteractive={() =>
            onSlot({
              ledger: 'volume',
              number: 0,
              collectionId: c.id,
              collectionName: `${c.editionName} · ${c.language}${c.medium ? ` · ${c.medium}` : ''}`,
              monitored: c.monitored,
              wanted: false,
              fileIds: [],
              autoSearch: true,
            })
          }
        />
      </Group>
      {colSlots.length > 0 && (
        <Progress size="xs" value={(owned / Math.max(colSlots.length, 1)) * 100} mt="xs" />
      )}
      <Collapse expanded={open}>
        {view === 'chips' ? (
          <Group gap={4} mt="sm">
            {colSlots.map((s) => (
              <Tooltip
                key={`${s.number}-${s.countType}`}
                label={`v${fmtNum(s.number)}${s.releaseDate ? ' · ' + s.releaseDate : ''}${
                  s.wanted ? ' · wanted' : ''
                } — click for details`}
              >
                <Button
                  size="compact-xs"
                  aria-label={`Volume ${fmtNum(s.number)} details`}
                  {...chipProps(s)}
                  onClick={() => openSlot(s)}
                >
                  {fmtNum(s.number)}
                </Button>
              </Tooltip>
            ))}
          </Group>
        ) : (
          <Table.ScrollContainer minWidth={520}><Table highlightOnHover verticalSpacing={8} mt="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th w={70}>Volume</Table.Th>
                <Table.Th w={120}>Released</Table.Th>
                <Table.Th w={110}>Status</Table.Th>
                <Table.Th>File</Table.Th>
                <Table.Th w={70} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {colSlots.map((s) => (
                <Table.Tr
                  key={`${s.number}-${s.countType}`}
                  style={{ cursor: 'pointer' }}
                  onClick={() => openSlot(s)}
                >
                  <Table.Td>
                    <UnstyledButton onClick={(e) => { e.stopPropagation(); openSlot(s) }} aria-label={`Volume ${fmtNum(s.number)} details`}><Text size="sm" fw={500}>{fmtNum(s.number)}</Text></UnstyledButton>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {s.releaseDate || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <StatusBadge slot={s} />
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {slotFileText(s.fileIds, fileById)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <SlotActions
                      seriesID={seriesID}
                      ledger="volume"
                      target={{ collectionId: c.id, number: s.number }}
                      busy={busy}
                      run={run}
                      onInteractive={() => openSlot(s, true)}
                    />
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table></Table.ScrollContainer>
        )}
        {colSlots.length === 0 && (
          <Text size="sm" c="dimmed" mt="xs">
            No volumes known for this collection.
          </Text>
        )}
      </Collapse>
    </Card>
  )
}

// CollectionsModal manages every collection MangaBaka knows (plus customs):
// monitored ones are collected — they show on the Volumes tab and are
// searched; the rest are reference data kept out of the way.
function CollectionsModal({
  seriesID,
  collections,
  slots,
  opened,
  run,
  onAdd,
  onClose,
}: {
  seriesID: number
  collections: Collection[]
  slots: Slots | null
  opened: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onAdd: () => void
  onClose: () => void
}) {
  return (
    <Modal opened={opened} onClose={onClose} title="Collections" size="lg">
      <Stack gap="sm">
        <Text size="xs" c="dimmed">
          Choose the editions and languages you collect. Sento searches monitored collections for missing releases.
        </Text>
        {collections.map((c) => {
          const isChapter = c.type === 'chapter'
          const cs = (isChapter ? slots?.chapters : slots?.volumes)?.[String(c.id)] ?? []
          const own = cs.filter((s) => (s.fileIds?.length ?? 0) > 0).length
          return (
            <Group key={c.id} justify="space-between" wrap="nowrap">
              <div>
                <Text size="sm" fw={500}>
                  {c.editionName}{' '}
                  <Text span size="xs" c="dimmed">
                    {[c.language, c.medium, c.publisher].filter(Boolean).join(' · ')}
                  </Text>{' '}
                  {isChapter && (
                    <Badge size="xs" color="teal" variant="light">
                      chapters
                    </Badge>
                  )}{' '}
                  {c.custom && (
                    <Badge size="xs" color="grape" variant="light">
                      custom
                    </Badge>
                  )}
                </Text>
                <Text size="xs" c="dimmed">
                  {own}/{cs.length} owned
                </Text>
              </div>
              <Group gap="xs" wrap="nowrap">
                {c.custom && (
                  <CustomCollectionActions
                    seriesID={seriesID}
                    collection={c}
                    others={collections.filter((o) => o.id !== c.id && o.type === c.type)}
                    run={run}
                  />
                )}
                <CollectionProfileSelect seriesID={seriesID} collection={c} run={run} />
                <Switch
                  size="xs"
                  label="Monitored"
                  checked={c.monitored}
                  onChange={(e) =>
                    run(() => api.setCollectionMonitored(seriesID, c.id, e.currentTarget.checked))
                  }
                />
              </Group>
            </Group>
          )
        })}
        {collections.length === 0 && (
          <Text size="sm" c="dimmed">
            No collections known for this series.
          </Text>
        )}
        <Button size="xs" variant="default" style={{ alignSelf: 'flex-start' }} onClick={onAdd}>
          Add custom collection
        </Button>
      </Stack>
    </Modal>
  )
}

// CollectionProfileSelect overrides one collection's release profile
// (empty = inherit the series profile).
function CollectionProfileSelect({
  seriesID,
  collection,
  run,
}: {
  seriesID: number
  collection: Collection
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.listProfiles })
  if (!profiles.data?.length) return null
  return (
    <Select
      size="xs"
      w={150}
      title="Release profile (overrides the series profile)"
      placeholder="Series profile"
      clearable
      data={profiles.data.map((p) => ({ value: String(p.id), label: p.name }))}
      value={collection.profileId ? String(collection.profileId) : null}
      onChange={(v) =>
        run(() => api.setCollectionProfile(seriesID, collection.id, v ? Number(v) : 0))
      }
    />
  )
}

// AddCollectionModal creates a custom collection: based on an existing one
// (clones the volume numbering — language variants) or from scratch with
// a volume count (collections MangaBaka doesn't know yet). Metadata refresh
// adopts a custom collection automatically if MangaBaka later lists the same
// name/language/publisher/medium.
function AddCollectionModal({
  seriesID,
  collections,
  opened,
  busy,
  run,
  onClose,
}: {
  seriesID: number
  collections: Collection[]
  opened: boolean
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
  const [volumes, setVolumes] = useState<number | string>(12)
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
            basedOn: ctype === 'volume' && basedOn ? Number(basedOn) : 0,
            volumes: ctype === 'volume' && !basedOn ? Number(volumes) || 0 : 0,
            cutoffNum: ctype === 'chapter' ? Math.round((Number(cutoff) || 0) * 100) : 0,
            monitored: true,
          })
          .then(onClose),
      'Collection created',
    )

  return (
    <Modal opened={opened} onClose={onClose} title="Add collection" size="md">
      <Stack gap="xs">
        <SegmentedControl
          size="xs"
          value={ctype}
          onChange={(v) => {
            const t = v as 'volume' | 'chapter'
            setCtype(t)
            // Swap the default name when the user hasn't typed their own.
            if (t === 'chapter' && name === 'Standard Edition') setName('Chapters')
            if (t === 'volume' && name === 'Chapters') setName('Standard Edition')
          }}
          data={[
            { value: 'volume', label: 'Volumes' },
            { value: 'chapter', label: 'Chapters' },
          ]}
        />
        {ctype === 'volume' && (
          <Select
            label="Based on"
            description="Clones the volume numbering (release dates stay unknown); pick nothing to start from scratch"
            placeholder="From scratch"
            clearable
            data={collections.map((c) => ({
              value: String(c.id),
              label: `${c.editionName} · ${c.language} · ${c.medium || '?'} (${c.countMain} vols)`,
            }))}
            value={basedOn}
            onChange={setBasedOn}
          />
        )}
        {ctype === 'volume' && !basedOn && (
          <NumberInput label="Volumes" min={1} max={500} value={volumes} onChange={setVolumes} />
        )}
        {ctype === 'chapter' && (
          <NumberInput
            label="Monitor from chapter"
            description="Chapters below the cutoff are ignored (0 = collect everything)"
            min={0}
            value={cutoff}
            onChange={setCutoff}
          />
        )}
        <TextInput
          label="Collection name"
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
        <Button
          mt="xs"
          loading={busy}
          disabled={!name.trim() || !language.trim()}
          onClick={create}
        >
          Create collection
        </Button>
      </Stack>
    </Modal>
  )
}

// CustomCollectionActions: merge a custom edition into another one (links
// re-point by volume number) or delete it (files stay in the library).
function CustomCollectionActions({
  seriesID,
  collection,
  others,
  run,
}: {
  seriesID: number
  collection: Collection
  others: Collection[]
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  return (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <Button size="compact-xs" variant="subtle" px={4}>
          <IconDotsVertical size={14} />
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Merge into</Menu.Label>
        {others.map((o) => (
          <Menu.Item
            key={o.id}
            onClick={() =>
              run(async () => {
                const r = await api.mergeCollection(seriesID, collection.id, o.id)
                if (r.unmatched?.length) {
                  window.alert(
                    `Merged, but ${r.unmatched.map((n) => 'v' + n / 100).join(', ')} had no matching slot in "${o.editionName}" — the files stay linked but won't show on its grid.`,
                  )
                }
              }, 'Collections merged')
            }
          >
            {o.editionName} · {o.language} {o.medium ? `· ${o.medium}` : ''}
          </Menu.Item>
        ))}
        {others.length === 0 && <Menu.Item disabled>No other collection</Menu.Item>}
        <Menu.Divider />
        <Menu.Item
          color="red"
          leftSection={<IconTrash size={14} />}
          onClick={() =>
            run(() => api.deleteCollection(seriesID, collection.id), 'Collection deleted')
          }
        >
          Delete collection
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}

// ChapterStrip compresses the whole chapter run into one thin bar:
// contiguous same-state chapters become proportional segments, so 1,100
// chapters stay readable at a glance.
function ChapterStrip({ chapters }: { chapters: ChapterSlot[] }) {
  if (chapters.length === 0) return null
  const state = (c: ChapterSlot) => (c.fileIds?.length ? 'owned' : c.wanted ? 'wanted' : 'rest')
  type Run = { state: string; from: number; to: number; count: number }
  const runs: Run[] = []
  for (const c of chapters) {
    const s = state(c)
    const last = runs[runs.length - 1]
    if (last && last.state === s) {
      last.to = c.number
      last.count++
    } else {
      runs.push({ state: s, from: c.number, to: c.number, count: 1 })
    }
  }
  const colors: Record<string, string> = {
    owned: 'var(--mantine-color-green-6)',
    wanted: 'var(--mantine-color-yellow-5)',
    rest: 'var(--mantine-color-default-border)',
  }
  const labels: Record<string, string> = {
    owned: 'owned',
    wanted: 'wanted',
    rest: 'not monitored / missing',
  }
  return (
    <div style={{ display: 'flex', gap: 1, height: 10, borderRadius: 3, overflow: 'hidden' }}>
      {runs.map((r, i) => (
        <Tooltip
          key={i}
          label={`c${fmtNum(r.from)}${r.count > 1 ? `–c${fmtNum(r.to)}` : ''} · ${labels[r.state]} (${r.count})`}
        >
          <div style={{ flexGrow: r.count, minWidth: 2, background: colors[r.state] }} />
        </Tooltip>
      ))}
    </div>
  )
}

type ChapterFilter = 'wanted' | 'missing' | 'owned' | 'all'

// ChaptersTab mirrors the Volumes tab: one section per monitored chapter
// collection (language variants, colored runs, a specific team — each with
// its own profile and cutoff).
function ChaptersTab({
  seriesID,
  collections,
  slots,
  files,
  busy,
  run,
  onSlot,
  onManageCollections,
  view,
}: {
  seriesID: number
  collections: Collection[]
  slots: Slots | null
  files: SeriesFile[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
  onManageCollections: () => void
  view: LedgerView
}) {
  const monitoredCols = collections.filter((c) => c.type === 'chapter' && c.monitored)
  const fileById = new Map(files.map((f) => [f.id, f]))

  return (
    <Stack>
      {monitoredCols.length === 0 && (
        <Text size="sm" c="dimmed">
          No monitored chapter collection —{' '}
          <Anchor component="button" type="button" size="sm" onClick={onManageCollections}>choose a collection</Anchor>{' '}
          to enable one.
        </Text>
      )}
      {monitoredCols.map((c) => (
        <ChapterCollectionSection
          key={c.id}
          seriesID={seriesID}
          collection={c}
          chapters={slots?.chapters?.[String(c.id)] ?? []}
          fileById={fileById}
          defaultOpen={monitoredCols.length === 1}
          view={view}
          busy={busy}
          run={run}
          onSlot={onSlot}
        />
      ))}
    </Stack>
  )
}

// ChapterCollectionSection is one chapter collection as a collapsible card:
// strip + counters in the header, the filterable chapter list inside.
function ChapterCollectionSection({
  seriesID,
  collection: col,
  chapters,
  fileById,
  defaultOpen,
  view,
  busy,
  run,
  onSlot,
}: {
  seriesID: number
  collection: Collection
  chapters: ChapterSlot[]
  fileById: Map<number, SeriesFile>
  defaultOpen: boolean
  view: LedgerView
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
}) {
  const [open, setOpen] = useState(defaultOpen)
  const [filter, setFilter] = useState<ChapterFilter>('wanted')
  const [showAll, setShowAll] = useState(false)

  const collectionName = [col.editionName, col.language].filter(Boolean).join(' · ')
  const monitored = chapters.filter((c) => c.monitored)
  const owned = monitored.filter((c) => (c.fileIds?.length ?? 0) > 0).length
  const wanted = chapters.filter((c) => c.wanted)

  const openSlot = (c: ChapterSlot, autoSearch = false) =>
    onSlot({
      ledger: 'chapter',
      number: c.number,
      collectionId: col.id,
      collectionName,
      monitored: c.monitored,
      wanted: c.wanted,
      fileIds: c.fileIds ?? [],
      autoSearch,
    })

  // Newest first in the table; the chip grid reads in story order.
  const filtered = chapters.filter((c) => {
    switch (filter) {
      case 'wanted':
        return c.wanted
      case 'missing':
        return !c.fileIds?.length
      case 'owned':
        return !!c.fileIds?.length
      default:
        return true
    }
  })
  const tableRows = [...filtered].reverse()
  const visible = showAll ? tableRows : tableRows.slice(0, 200)
  const chipSlots = showAll ? filtered : filtered.slice(-200)

  return (
    <Card withBorder padding="sm">
      <Group justify="space-between" align="flex-start">
        <UnstyledButton onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        <Group gap={8} wrap="nowrap">
          <IconChevronRight
            size={16}
            style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform 150ms' }}
          />
          <div>
            <Text fw={600}>
              {col.editionName}{' '}
              <Text span c="dimmed" size="sm">
                {[col.language, col.publisher].filter(Boolean).join(' · ')}
              </Text>{' '}
              {col.custom && (
                <Badge size="xs" color="grape" variant="light">
                  custom
                </Badge>
              )}
            </Text>
            <Text size="xs" c="dimmed">
              {owned}/{monitored.length} monitored owned · {wanted.length} wanted
            </Text>
          </div>
        </Group></UnstyledButton>
        <Group gap={4} wrap="nowrap">
          <CutoffButton seriesID={seriesID} collection={col} run={run} />
          <SlotActions
            seriesID={seriesID}
            ledger="chapter"
            target={{ collectionId: col.id, number: 0 }}
            busy={busy}
            run={run}
            autoLabel="Search and download wanted chapters"
            onInteractive={() =>
              onSlot({
                ledger: 'chapter',
                number: 0,
                collectionId: col.id,
                collectionName,
                monitored: col.monitored,
                wanted: false,
                fileIds: [],
                autoSearch: true,
              })
            }
          />
        </Group>
      </Group>
      <div style={{ marginTop: 8 }}>
        <ChapterStrip chapters={chapters} />
      </div>
      <Collapse expanded={open}>
        <Group justify="flex-end" mt="xs">
          <SegmentedControl
            size="xs"
            value={filter}
            onChange={(v) => {
              setFilter(v as ChapterFilter)
              setShowAll(false)
            }}
            data={[
              { value: 'wanted', label: `Wanted (${wanted.length})` },
              { value: 'missing', label: 'Missing' },
              { value: 'owned', label: 'Owned' },
              { value: 'all', label: `All (${chapters.length})` },
            ]}
          />
        </Group>

        {view === 'chips' && chipSlots.length > 0 && (
          <Group gap={3} mt="xs">
            {chipSlots.map((c) => (
              <Tooltip
                key={c.number}
                label={`c${fmtNum(c.number)}${c.wanted ? ' · wanted' : ''} — click for details`}
              >
                <Button
                  size="compact-xs"
                  aria-label={`Chapter ${fmtNum(c.number)} details`}
                  {...chipProps(c)}
                  onClick={() => openSlot(c)}
                >
                  {fmtNum(c.number)}
                </Button>
              </Tooltip>
            ))}
          </Group>
        )}
        {view === 'table' && visible.length > 0 && (
          <Table.ScrollContainer minWidth={520}><Table highlightOnHover verticalSpacing={8} mt="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th w={90}>Chapter</Table.Th>
                <Table.Th w={110}>Status</Table.Th>
                <Table.Th>File</Table.Th>
                <Table.Th w={70} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {visible.map((c) => (
                <Table.Tr key={c.number} style={{ cursor: 'pointer' }} onClick={() => openSlot(c)}>
                  <Table.Td>
                    <UnstyledButton onClick={(e) => { e.stopPropagation(); openSlot(c) }} aria-label={`Chapter ${fmtNum(c.number)} details`}><Text size="sm" fw={500}>{fmtNum(c.number)}</Text></UnstyledButton>
                  </Table.Td>
                  <Table.Td>
                    <StatusBadge slot={c} />
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {slotFileText(c.fileIds, fileById)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <SlotActions
                      seriesID={seriesID}
                      ledger="chapter"
                      target={{ collectionId: col.id, number: c.number }}
                      busy={busy}
                      run={run}
                      onInteractive={() => openSlot(c, true)}
                    />
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table></Table.ScrollContainer>
        )}
        {!showAll && filtered.length > 200 && (
          <Text size="xs" c="dimmed" mt="xs">
            Showing {view === 'chips' ? 'the latest' : ''} 200 of {filtered.length} —{' '}
            <Text
              span
              size="xs"
              c="indigo"
              style={{ cursor: 'pointer' }}
              onClick={() => setShowAll(true)}
            >
              show all
            </Text>
          </Text>
        )}
        {filtered.length === 0 && (
          <Text size="sm" c="dimmed" mt="xs">
            {filter === 'wanted'
              ? 'No wanted chapters. Choose All to browse the rest.'
              : 'Nothing matches this filter.'}
          </Text>
        )}
      </Collapse>
    </Card>
  )
}

// IndexersTab: per-series view of every site with override switches and
// binding status (auto-bound by search; rebind here when it matched wrong).
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
  const { data: sites, error, isPending, refetch } = useQuery({
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
        These settings apply to this series. <Anchor component={Link} to="/indexers" size="sm">Manage indexers</Anchor> to change the defaults.
        Source matches are found automatically when you search; you can also choose them below.
      </Text>
      <QueryFeedback pending={isPending} error={error} onRetry={() => void refetch()} />
      {sites?.length === 0 && !error && <EmptyState title="No indexers configured" description="Add a torrent indexer or an extension source to find releases." action={<Button component={Link} to="/indexers" size="sm" variant="default">Set up indexers</Button>} />}
      {!!sites?.length && <Table.ScrollContainer minWidth={700}><Table>
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
                    <Badge size="xs" variant="outline" color={s.protocol === 'torznab' ? 'blue' : 'teal'}>
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
                        {s.bindings?.some((b) => b.state === 'active') ? 'Add match' : 'Find match'}
                      </Button>
                    )}
                  </Group>
                </Table.Td>
              </Table.Tr>
            )
          })}
        </Table.Tbody>
      </Table></Table.ScrollContainer>}

      <RebindModal
        seriesID={seriesID}
        seriesTitle={seriesTitle}
        site={
          rebind &&
          (sites?.find((x) => x.id === rebind.id) ??
            rebind)
        }
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
