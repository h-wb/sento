import { useCallback, useState } from 'react'
import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Card,
  Collapse,
  Divider,
  Group,
  Image,
  Loader,
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
  IconBookmark,
  IconBookmarkFilled,
  IconChevronRight,
  IconDotsVertical,
  IconEye,
  IconEyeOff,
  IconListSearch,
  IconRefresh,
  IconScissors,
  IconSearch,
  IconTrash,
  IconVersions,
  IconX,
} from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import {
  api,
  fmtNum,
  fmtSize,
  type ChapterLedger,
  type ChapterSlot,
  type Collection,
  type SeriesFile,
  type SeriesIndexerView,
  type Slots,
  type VolumeSlot,
} from '../api'
import SlotModal, { type SlotSelection } from '../components/SlotModal'
import { fmtWhen, notify, usePageTitle } from '../ui'

export default function SeriesPage() {
  const { id } = useParams()
  const seriesID = Number(id)
  const navigate = useNavigate()

  const [busy, setBusy] = useState(false)
  const [slotSel, setSlotSel] = useState<SlotSelection | null>(null)
  const [fullDesc, setFullDesc] = useState(false)
  const [manageEditions, setManageEditions] = useState(false)
  const [addEdition, setAddEdition] = useState(false)
  const [tab, setTab] = useState<string | null>('volumes')

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
  const ledger = detail.data?.chapterLedger ?? null
  const slots = slotsQuery.data ?? null
  const files = filesQuery.data ?? []

  // Refresh everything under this series (slots, files, indexers, detail).
  const load = useCallback(
    () => queryClient.invalidateQueries({ queryKey: ['series', seriesID] }),
    [queryClient, seriesID],
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
    if (detail.error) return <Alert color="red">{String(detail.error)}</Alert>
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

  return (
    <>
      <div
        style={{
          position: 'sticky',
          top: 'var(--app-shell-header-height, 56px)',
          zIndex: 20,
          background: 'var(--mantine-color-body)',
          // Bleed into the AppShell.Main padding on all sides so the bar
          // sits flush under the header at rest — the same place it pins
          // when stuck — instead of gaining the gutter as extra height.
          marginTop: 'calc(var(--mantine-spacing-md) * -1)',
          marginInline: 'calc(var(--mantine-spacing-md) * -1)',
          paddingInline: 'var(--mantine-spacing-md)',
          marginBottom: 'var(--mantine-spacing-md)',
        }}
      >
        <Group gap={4} py={4}>
          <BarButton
            icon={<IconRefresh size={18} />}
            label="Refresh & Scan"
            loading={busy}
            onClick={() =>
              run(async () => {
                await api.refreshSeries(seriesID)
                const r = await api.scanSeries(seriesID)
                notify.ok(`Refreshed — scan: ${r.scanned} new, ${r.linked} linked`)
              })
            }
          />
          <BarButton
            icon={<IconSearch size={18} />}
            label="Search Monitored"
            loading={busy}
            onClick={() =>
              run(async () => {
                const [v, c] = await Promise.all([
                  api.search(seriesID, { ledger: 'volume' }),
                  api.search(seriesID, { ledger: 'chapter' }),
                ])
                const queued = (v.queued?.length ?? 0) + (c.queued?.length ?? 0)
                notify.ok(
                  queued
                    ? `Queued ${queued} release${queued > 1 ? 's' : ''}`
                    : 'Nothing grabbed — no suitable release for the wanted slots',
                )
              })
            }
          />
          <Divider orientation="vertical" mx={4} />
          {tab === 'volumes' && (
            <BarButton
              icon={<IconVersions size={18} />}
              label="Editions"
              onClick={() => setManageEditions(true)}
            />
          )}
          {tab === 'chapters' && ledger && (
            <>
              <BarButton
                icon={ledger.monitored ? <IconEye size={18} /> : <IconEyeOff size={18} />}
                label={ledger.monitored ? 'Chapters: On' : 'Chapters: Off'}
                onClick={() =>
                  run(() =>
                    api.updateChapterLedger(seriesID, { monitored: !ledger.monitored }),
                  )
                }
              />
              <CutoffBarButton seriesID={seriesID} ledger={ledger} run={run} />
            </>
          )}
          <BarButton
            icon={
              series.monitored ? (
                <IconBookmarkFilled size={18} />
              ) : (
                <IconBookmark size={18} />
              )
            }
            label={series.monitored ? 'Monitored' : 'Unmonitored'}
            onClick={() => run(() => api.setSeriesMonitored(seriesID, !series.monitored))}
          />
          <Divider orientation="vertical" mx={4} />
          <BarButton
            icon={<IconTrash size={18} />}
            label="Delete"
            color="red"
            onClick={() => {
              if (
                !window.confirm(
                  `Remove "${series.title}" from Sento?\n\nLibrary files stay on disk; monitoring, history and slot links are deleted.`,
                )
              )
                return
              run(() => api.deleteSeries(seriesID).then(() => navigate('/')))
            }}
          />
        </Group>
        <Divider />
      </div>

      <Group align="flex-start" wrap="nowrap" mb="md">
        <Image src={series.coverUrl} w={140} radius="sm" alt="" visibleFrom="xs" />
        <div style={{ flex: 1 }}>
          <Title order={2}>{series.title}</Title>
          <Group gap={6} mt={4}>
            <Badge>{series.status}</Badge>
            <Badge variant="light">{series.type}</Badge>
            {series.year ? <Badge variant="default">{series.year}</Badge> : null}
            {series.finalVolume > 0 && <Badge variant="outline">{series.finalVolume} volumes</Badge>}
            {series.totalChapters > 0 && (
              <Badge variant="outline">{series.totalChapters} chapters</Badge>
            )}
            {series.contentRating && series.contentRating !== 'safe' && (
              <Badge variant="outline" color="pink">
                {series.contentRating}
              </Badge>
            )}
            {!series.monitored && (
              <Badge variant="outline" color="gray">
                unmonitored
              </Badge>
            )}
          </Group>
          {series.description && (
            <Text
              size="sm"
              c="dimmed"
              mt="xs"
              lineClamp={fullDesc ? undefined : 3}
              onClick={() => setFullDesc((v) => !v)}
              style={{ cursor: 'pointer' }}
              title={fullDesc ? 'Click to collapse' : 'Click to expand'}
            >
              {series.description}
            </Text>
          )}
        </div>
      </Group>

      <EditionsModal
        seriesID={seriesID}
        collections={volumeCollections}
        slots={slots}
        opened={manageEditions}
        run={run}
        onAdd={() => setAddEdition(true)}
        onClose={() => setManageEditions(false)}
      />
      <AddEditionModal
        seriesID={seriesID}
        collections={volumeCollections}
        opened={addEdition}
        busy={busy}
        run={run}
        onClose={() => setAddEdition(false)}
      />

      <Tabs value={tab} onChange={setTab}>
        <Tabs.List>
          <Tabs.Tab value="volumes">Volumes</Tabs.Tab>
          <Tabs.Tab value="chapters">Chapters</Tabs.Tab>
          <Tabs.Tab value="indexers">Indexers</Tabs.Tab>
          <Tabs.Tab value="files">Files ({files.length})</Tabs.Tab>
          <Tabs.Tab value="history">History</Tabs.Tab>
        </Tabs.List>

        <Tabs.Panel value="volumes" pt="md">
          <VolumesTab
            seriesID={seriesID}
            collections={collections}
            slots={slots}
            files={files}
            busy={busy}
            run={run}
            onSlot={setSlotSel}
            onManageEditions={() => setManageEditions(true)}
          />
        </Tabs.Panel>

        <Tabs.Panel value="chapters" pt="md">
          <ChaptersTab
            seriesID={seriesID}
            ledger={ledger}
            slots={slots}
            files={files}
            busy={busy}
            run={run}
            onSlot={setSlotSel}
          />
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
                Scan folder (import existing files)
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
            {files.length === 0 && <Text c="dimmed">No files yet.</Text>}
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
  const { data: events } = useQuery({
    queryKey: ['series', seriesID, 'history'],
    queryFn: () => api.history(seriesID),
  })

  if (events && events.length === 0)
    return <Text c="dimmed">Nothing yet — grabs and imports will show up here.</Text>
  return (
    <Table striped>
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
                {d.body.event}
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
  )
}

// BarButton is one entry of the Sonarr-style action bar: icon above a
// small label.
function BarButton({
  icon,
  label,
  color,
  loading,
  onClick,
}: {
  icon: React.ReactNode
  label: string
  color?: string
  loading?: boolean
  onClick: () => void
}) {
  return (
    <UnstyledButton
      onClick={onClick}
      disabled={loading}
      px="sm"
      py={4}
      style={{ borderRadius: 6, opacity: loading ? 0.5 : 1 }}
    >
      <Stack gap={2} align="center" c={color}>
        {loading ? <Loader size={18} color={color} /> : icon}
        <Text size="xs" c={color ?? 'dimmed'}>
          {label}
        </Text>
      </Stack>
    </UnstyledButton>
  )
}

// CutoffBarButton edits the chapter cutoff from the action bar.
function CutoffBarButton({
  seriesID,
  ledger,
  run,
}: {
  seriesID: number
  ledger: ChapterLedger
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const [opened, setOpened] = useState(false)
  const [cutoff, setCutoff] = useState<number | ''>(ledger.cutoffNum / 100)
  // Adjust-during-render: re-sync when the ledger (re)loads.
  const [prevCutoff, setPrevCutoff] = useState(ledger.cutoffNum)
  if (ledger.cutoffNum !== prevCutoff) {
    setPrevCutoff(ledger.cutoffNum)
    setCutoff(ledger.cutoffNum / 100)
  }
  return (
    <Popover opened={opened} onChange={setOpened} position="bottom-start" withinPortal trapFocus>
      <Popover.Target>
        <div style={{ display: 'inline-block' }}>
          <BarButton
            icon={<IconScissors size={18} />}
            label={`Cutoff: c${fmtNum(ledger.cutoffNum)}`}
            onClick={() => setOpened((o) => !o)}
          />
        </div>
      </Popover.Target>
      <Popover.Dropdown>
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
                  api.updateChapterLedger(seriesID, {
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
  autoLabel = 'Auto search — grab the best release',
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
          onClick={(e) => {
            e.stopPropagation()
            setClicked(true)
            run(async () => {
              const r = await api.search(seriesID, { ledger, slot: target })
              notify.ok(
                r.queued?.length
                  ? `Grabbed ${r.queued.length} release${r.queued.length > 1 ? 's' : ''}`
                  : 'Nothing grabbed — no suitable release found',
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
  onManageEditions,
}: {
  seriesID: number
  collections: Collection[]
  slots: Slots | null
  files: SeriesFile[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
  onManageEditions: () => void
}) {
  const volumeCollections = collections.filter((c) => c.type === 'volume')
  // Only monitored editions show here — the ones being collected. The
  // rest (reference data) live in the Editions manager on the action bar.
  const monitoredCols = volumeCollections.filter((c) => c.monitored)
  const fileById = new Map(files.map((f) => [f.id, f]))

  return (
    <Stack>
      {monitoredCols.length === 0 && (
        <Text size="sm" c="dimmed">
          No monitored edition —{' '}
          <Text
            span
            size="sm"
            c="indigo"
            style={{ cursor: 'pointer' }}
            onClick={onManageEditions}
          >
            open Editions
          </Text>{' '}
          to pick the one you collect.
        </Text>
      )}
      {monitoredCols.map((c) => (
        <EditionSection
          key={c.id}
          seriesID={seriesID}
          collection={c}
          colSlots={slots?.volumes?.[String(c.id)] ?? []}
          fileById={fileById}
          defaultOpen={monitoredCols.length === 1}
          busy={busy}
          run={run}
          onSlot={onSlot}
        />
      ))}
    </Stack>
  )
}

// EditionSection is one monitored edition as a collapsible card (Sonarr's
// seasons): header with progress always visible, the volume table behind
// the chevron — expanded by default only when it's the lone edition.
function EditionSection({
  seriesID,
  collection: c,
  colSlots,
  fileById,
  defaultOpen,
  busy,
  run,
  onSlot,
}: {
  seriesID: number
  collection: Collection
  colSlots: VolumeSlot[]
  fileById: Map<number, SeriesFile>
  defaultOpen: boolean
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
}) {
  const [open, setOpen] = useState(defaultOpen)
  const owned = colSlots.filter((s) => (s.fileIds?.length ?? 0) > 0).length

  return (
    <Card withBorder padding="sm">
      <Group
        justify="space-between"
        wrap="nowrap"
        style={{ cursor: 'pointer' }}
        onClick={() => setOpen((o) => !o)}
      >
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
        </Group>
        <SlotActions
          seriesID={seriesID}
          ledger="volume"
          target={{ collectionId: c.id, number: 0 }}
          busy={busy}
          run={run}
          autoLabel="Auto search — grab the best for each wanted volume"
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
        <Table highlightOnHover verticalSpacing={4} mt="xs">
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
            {colSlots.map((s) => {
              const open = (autoSearch = false) =>
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
                <Table.Tr
                  key={`${s.number}-${s.countType}`}
                  style={{ cursor: 'pointer' }}
                  onClick={() => open()}
                >
                  <Table.Td>
                    <Text size="sm" fw={500}>
                      {fmtNum(s.number)}
                    </Text>
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
                      onInteractive={() => open(true)}
                    />
                  </Table.Td>
                </Table.Tr>
              )
            })}
          </Table.Tbody>
        </Table>
        {colSlots.length === 0 && (
          <Text size="sm" c="dimmed" mt="xs">
            No volumes known for this edition.
          </Text>
        )}
      </Collapse>
    </Card>
  )
}

// EditionsModal manages every edition MangaBaka knows (plus customs):
// monitored ones are collected — they show on the Volumes tab and are
// searched; the rest are reference data kept out of the way.
function EditionsModal({
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
    <Modal opened={opened} onClose={onClose} title="Editions" size="lg">
      <Stack gap="sm">
        <Text size="xs" c="dimmed">
          Monitored editions show on the Volumes tab and are searched; the rest are reference.
          Most series want exactly one monitored edition — the one you collect.
        </Text>
        {collections.map((c) => {
          const cs = slots?.volumes?.[String(c.id)] ?? []
          const own = cs.filter((s) => (s.fileIds?.length ?? 0) > 0).length
          return (
            <Group key={c.id} justify="space-between" wrap="nowrap">
              <div>
                <Text size="sm" fw={500}>
                  {c.editionName}{' '}
                  <Text span size="xs" c="dimmed">
                    {[c.language, c.medium, c.publisher].filter(Boolean).join(' · ')}
                  </Text>{' '}
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
                  <CustomEditionActions
                    seriesID={seriesID}
                    collection={c}
                    others={collections.filter((o) => o.id !== c.id)}
                    run={run}
                  />
                )}
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
            No volume editions known for this series.
          </Text>
        )}
        <Button size="xs" variant="default" style={{ alignSelf: 'flex-start' }} onClick={onAdd}>
          Add custom edition
        </Button>
      </Stack>
    </Modal>
  )
}

// AddEditionModal creates a custom edition: based on an existing one
// (clones the volume numbering — language variants) or from scratch with
// a volume count (editions MangaBaka doesn't know yet). Metadata refresh
// adopts a custom edition automatically if MangaBaka later lists the same
// name/language/publisher/medium.
function AddEditionModal({
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
  const [name, setName] = useState('Standard Edition')
  const [language, setLanguage] = useState('')
  const [publisher, setPublisher] = useState('')
  const [medium, setMedium] = useState('')
  const [basedOn, setBasedOn] = useState<string | null>(null)
  const [volumes, setVolumes] = useState<number | string>(12)

  const create = () =>
    run(
      () =>
        api
          .createCollection(seriesID, {
            editionName: name,
            language,
            publisher,
            medium,
            basedOn: basedOn ? Number(basedOn) : 0,
            volumes: basedOn ? 0 : Number(volumes) || 0,
            monitored: true,
          })
          .then(onClose),
      'Edition created',
    )

  return (
    <Modal opened={opened} onClose={onClose} title="Add edition" size="md">
      <Stack gap="xs">
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
        {!basedOn && (
          <NumberInput label="Volumes" min={1} max={500} value={volumes} onChange={setVolumes} />
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
        <Button
          mt="xs"
          loading={busy}
          disabled={!name.trim() || !language.trim()}
          onClick={create}
        >
          Create (monitored)
        </Button>
      </Stack>
    </Modal>
  )
}

// CustomEditionActions: merge a custom edition into another one (links
// re-point by volume number) or delete it (files stay in the library).
function CustomEditionActions({
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
              }, 'Editions merged')
            }
          >
            {o.editionName} · {o.language} {o.medium ? `· ${o.medium}` : ''}
          </Menu.Item>
        ))}
        {others.length === 0 && <Menu.Item disabled>No other edition</Menu.Item>}
        <Menu.Divider />
        <Menu.Item
          color="red"
          leftSection={<IconTrash size={14} />}
          onClick={() =>
            run(() => api.deleteCollection(seriesID, collection.id), 'Edition deleted')
          }
        >
          Delete edition
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

function ChaptersTab({
  seriesID,
  ledger,
  slots,
  files,
  busy,
  run,
  onSlot,
}: {
  seriesID: number
  ledger: ChapterLedger | null
  slots: Slots | null
  files: SeriesFile[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
}) {
  const chapters = slots?.chapters ?? []
  const monitored = chapters.filter((c) => c.monitored)
  const owned = monitored.filter((c) => (c.fileIds?.length ?? 0) > 0).length
  const wanted = chapters.filter((c) => c.wanted)
  const [filter, setFilter] = useState<ChapterFilter>('wanted')
  const [showAll, setShowAll] = useState(false)
  const fileById = new Map(files.map((f) => [f.id, f]))

  if (!ledger) return <Text c="dimmed">No chapter ledger.</Text>

  // Newest first: the actionable chapters live at the head of the run.
  const filtered = chapters
    .filter((c) => {
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
    .reverse()
  const visible = showAll ? filtered : filtered.slice(0, 200)

  return (
    <Stack>
      <ChapterStrip chapters={chapters} />
      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          {owned}/{monitored.length} monitored chapters owned · {wanted.length} wanted
        </Text>
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

      {visible.length > 0 && (
        <Table highlightOnHover verticalSpacing={4}>
          <Table.Thead>
            <Table.Tr>
              <Table.Th w={90}>Chapter</Table.Th>
              <Table.Th w={110}>Status</Table.Th>
              <Table.Th>File</Table.Th>
              <Table.Th w={70} />
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {visible.map((c) => {
              const open = (autoSearch = false) =>
                onSlot({
                  ledger: 'chapter',
                  number: c.number,
                  monitored: c.monitored,
                  wanted: c.wanted,
                  fileIds: c.fileIds ?? [],
                  autoSearch,
                })
              return (
                <Table.Tr key={c.number} style={{ cursor: 'pointer' }} onClick={() => open()}>
                  <Table.Td>
                    <Text size="sm" fw={500}>
                      {fmtNum(c.number)}
                    </Text>
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
                      target={{ collectionId: 0, number: c.number }}
                      busy={busy}
                      run={run}
                      onInteractive={() => open(true)}
                    />
                  </Table.Td>
                </Table.Tr>
              )
            })}
          </Table.Tbody>
        </Table>
      )}
      {filtered.length > visible.length && (
        <Text size="xs" c="dimmed">
          Showing {visible.length} of {filtered.length} —{' '}
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
        <Text size="sm" c="dimmed">
          {filter === 'wanted'
            ? 'No wanted chapters — everything monitored is owned. Switch the filter to browse the rest.'
            : 'Nothing matches this filter.'}
        </Text>
      )}
    </Stack>
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
  const { data: sites } = useQuery({
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
        Defaults come from the global Indexers page; switches here override them for this series
        only. Extension sources bind automatically on first search (every entry at ≥0.90 title
        match — sites often list a series several times); all active entries are searched and the
        best release wins per slot. Disable or remove an entry if it produces garbage.
      </Text>
      <Table>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Site</Table.Th>
            <Table.Th>Enabled</Table.Th>
            <Table.Th>Volumes</Table.Th>
            <Table.Th>Chapters</Table.Th>
            <Table.Th>Binding</Table.Th>
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
                    checked={s.enabled}
                    onChange={(e) => override(s, { enabled: e.currentTarget.checked })}
                  />
                </Table.Td>
                <Table.Td>
                  <Switch
                    size="xs"
                    checked={s.useVolumes}
                    disabled={!s.enabled}
                    onChange={(e) => override(s, { useVolumes: e.currentTarget.checked })}
                  />
                </Table.Td>
                <Table.Td>
                  <Switch
                    size="xs"
                    checked={s.useChapters}
                    disabled={!s.enabled}
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
                        {s.bindings?.some((b) => b.state === 'active') ? 'Bind more' : 'Bind'}
                      </Button>
                    )}
                  </Group>
                </Table.Td>
              </Table.Tr>
            )
          })}
        </Table.Tbody>
      </Table>

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
        auto on search
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
                size="xs"
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
            <Tooltip label="Remove binding">
              <ActionIcon
                size="xs"
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
    <Modal opened onClose={onClose} title={`Bind ${site.name}`} size="lg">
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
                    'Source bound',
                  )
                }
              >
                {isBound ? 'Bound' : 'Bind'}
              </Button>
            </Group>
          )
        })}
      </Stack>
    </Modal>
  )
}
