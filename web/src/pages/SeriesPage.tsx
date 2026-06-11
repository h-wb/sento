import { useCallback, useState } from 'react'
import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Image,
  Menu,
  Modal,
  NumberInput,
  Progress,
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
} from '@mantine/core'
import {
  IconDotsVertical,
  IconEye,
  IconEyeOff,
  IconRefresh,
  IconSearch,
  IconTrash,
  IconX,
} from '@tabler/icons-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import {
  api,
  fmtNum,
  fmtSize,
  type ChapterLedger,
  type Collection,
  type ReleaseCandidate,
  type SearchResponse,
  type SeriesSiteView,
  type Slots,
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

  return (
    <>
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
          <Group mt="sm">
            <Switch
              size="sm"
              label="Monitored"
              description="Off pauses all automatic searches"
              checked={series.monitored}
              onChange={(e) =>
                run(() => api.setSeriesMonitored(seriesID, e.currentTarget.checked))
              }
            />
            <Button
              size="xs"
              variant="default"
              leftSection={<IconRefresh size={14} />}
              onClick={() => run(() => api.refreshSeries(seriesID), 'Metadata refreshed')}
              loading={busy}
            >
              Refresh
            </Button>
            <Button
              size="xs"
              color="red"
              variant="outline"
              leftSection={<IconTrash size={14} />}
              onClick={() => {
                if (
                  !window.confirm(
                    `Remove "${series.title}" from Sento?\n\nLibrary files stay on disk; monitoring, history and slot links are deleted.`,
                  )
                )
                  return
                run(() => api.deleteSeries(seriesID).then(() => navigate('/')))
              }}
            >
              Remove
            </Button>
          </Group>
        </div>
      </Group>

      <Tabs defaultValue="volumes">
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
            busy={busy}
            run={run}
            onSlot={setSlotSel}
          />
        </Tabs.Panel>

        <Tabs.Panel value="chapters" pt="md">
          <ChaptersTab
            seriesID={seriesID}
            ledger={ledger}
            slots={slots}
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

// SlotLegend decodes the slot badge colors (shared by both ledgers).
function SlotLegend() {
  return (
    <Group gap="md">
      <Group gap={4}>
        <Badge size="sm" variant="filled" color="green">
          1
        </Badge>
        <Text size="xs" c="dimmed">
          owned
        </Text>
      </Group>
      <Group gap={4}>
        <Badge size="sm" variant="outline" color="yellow">
          2
        </Badge>
        <Text size="xs" c="dimmed">
          wanted
        </Text>
      </Group>
      <Group gap={4}>
        <Badge size="sm" variant="light" color="gray">
          3
        </Badge>
        <Text size="xs" c="dimmed">
          not monitored / future
        </Text>
      </Group>
      <Text size="xs" c="dimmed">
        — click any slot for files &amp; interactive search
      </Text>
    </Group>
  )
}

function VolumesTab({
  seriesID,
  collections,
  slots,
  busy,
  run,
  onSlot,
}: {
  seriesID: number
  collections: Collection[]
  slots: Slots | null
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
}) {
  const [funnel, setFunnel] = useState<SearchResponse | null>(null)
  const [addEdition, setAddEdition] = useState(false)
  const volumeCollections = collections.filter((c) => c.type === 'volume')

  return (
    <Stack>
      <Group>
        <Button
          size="xs"
          leftSection={<IconSearch size={14} />}
          loading={busy}
          onClick={() =>
            run(
              () => api.search(seriesID, { ledger: 'volume' }).then(setFunnel),
              'Volume search done',
            )
          }
        >
          Search &amp; grab wanted
        </Button>
        <Button
          size="xs"
          variant="default"
          loading={busy}
          onClick={() =>
            run(() => api.search(seriesID, { ledger: 'volume', dryRun: true }).then(setFunnel))
          }
        >
          Search (dry run)
        </Button>
        <Button size="xs" variant="default" onClick={() => setAddEdition(true)}>
          Add edition
        </Button>
      </Group>
      <SlotLegend />

      <AddEditionModal
        seriesID={seriesID}
        collections={volumeCollections}
        opened={addEdition}
        busy={busy}
        run={run}
        onClose={() => setAddEdition(false)}
      />

      {volumeCollections.map((c) => {
        const colSlots = slots?.volumes?.[String(c.id)] ?? []
        const owned = colSlots.filter((s) => (s.fileIds?.length ?? 0) > 0).length
        return (
          <Card key={c.id} withBorder padding="sm">
            <Group justify="space-between" mb="xs">
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
              <Group gap="xs">
                {c.custom && (
                  <CustomEditionActions
                    seriesID={seriesID}
                    collection={c}
                    others={volumeCollections.filter((o) => o.id !== c.id)}
                    run={run}
                  />
                )}
                <Switch
                  label="Monitored"
                  checked={c.monitored}
                  onChange={(e) =>
                    run(() => api.setCollectionMonitored(seriesID, c.id, e.currentTarget.checked))
                  }
                />
              </Group>
            </Group>
            {colSlots.length > 0 && (
              <Progress value={(owned / Math.max(colSlots.length, 1)) * 100} mb="xs" />
            )}
            <Group gap={4}>
              {colSlots.map((s) => (
                <Tooltip
                  key={`${s.number}-${s.countType}`}
                  label={`v${fmtNum(s.number)}${s.releaseDate ? ' · ' + s.releaseDate : ''}${
                    s.wanted ? ' · wanted' : ''
                  } — click for details`}
                >
                  <Badge
                    size="sm"
                    style={{ cursor: 'pointer' }}
                    variant={s.fileIds?.length ? 'filled' : s.wanted ? 'outline' : 'light'}
                    color={s.fileIds?.length ? 'green' : s.wanted ? 'yellow' : 'gray'}
                    onClick={() =>
                      onSlot({
                        ledger: 'volume',
                        number: s.number,
                        collectionId: c.id,
                        collectionName: `${c.editionName} · ${c.language} · ${c.medium}`,
                        releaseDate: s.releaseDate,
                        monitored: s.monitored,
                        wanted: s.wanted,
                        fileIds: s.fileIds ?? [],
                      })
                    }
                  >
                    {fmtNum(s.number)}
                  </Badge>
                </Tooltip>
              ))}
            </Group>
          </Card>
        )
      })}

      {funnel && <FunnelCard funnel={funnel} />}
    </Stack>
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

function FunnelCard({ funnel }: { funnel: SearchResponse }) {
  return (
    <Card withBorder padding="sm">
      <Text fw={600} mb={4}>
        Search funnel ({funnel.candidates.length} releases)
      </Text>
      <Text size="xs" c="dimmed" mb="xs">
        Searched {funnel.searched.join(', ') || 'nothing'}
        {funnel.dropped > 0 && ` · ${funnel.dropped} entries for the other ledger skipped`}
      </Text>
      <Stack gap={4}>
        {funnel.candidates.slice(0, 30).map((c: ReleaseCandidate) => (
          <Group key={c.guid} justify="space-between" wrap="nowrap">
            <Text size="xs" lineClamp={1} style={{ flex: 1 }}>
              {c.title}
            </Text>
            <Group gap={6} wrap="nowrap">
              <Badge size="xs" variant="outline" color={c.protocol === 'torrent' ? 'blue' : 'teal'}>
                {c.provider}/{c.site}
              </Badge>
              {c.protocol === 'torrent' && (
                <Text size="xs" c="dimmed">
                  {fmtSize(c.size ?? 0)} · {c.seeders ?? 0}S
                </Text>
              )}
              {c.rejections?.length ? (
                <Badge size="xs" color="red" variant="light">
                  {c.rejections[0].replace('rejected.', '')}
                </Badge>
              ) : (
                <Badge size="xs" color="green" variant="light">
                  score {c.score}
                </Badge>
              )}
            </Group>
          </Group>
        ))}
      </Stack>
    </Card>
  )
}

function ChaptersTab({
  seriesID,
  ledger,
  slots,
  busy,
  run,
  onSlot,
}: {
  seriesID: number
  ledger: ChapterLedger | null
  slots: Slots | null
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onSlot: (s: SlotSelection) => void
}) {
  const chapters = slots?.chapters ?? []
  const monitored = chapters.filter((c) => c.monitored)
  const owned = monitored.filter((c) => (c.fileIds?.length ?? 0) > 0).length
  const wanted = chapters.filter((c) => c.wanted)
  const [showAll, setShowAll] = useState(false)
  const [cutoff, setCutoff] = useState<number | ''>('')
  // Adjust-during-render: re-sync the input when the ledger (re)loads.
  const [prevCutoff, setPrevCutoff] = useState<number | null>(null)
  if (ledger && ledger.cutoffNum !== prevCutoff) {
    setPrevCutoff(ledger.cutoffNum)
    setCutoff(ledger.cutoffNum / 100)
  }

  if (!ledger) return <Text c="dimmed">No chapter ledger.</Text>

  return (
    <Stack>
      <Group>
        <Switch
          label="Monitor chapters"
          checked={ledger.monitored}
          onChange={(e) =>
            run(() => api.updateChapterLedger(seriesID, { monitored: e.currentTarget.checked }))
          }
        />
        <NumberInput
          label="Cutoff (monitor from chapter)"
          size="xs"
          value={cutoff}
          onChange={(v) => setCutoff(typeof v === 'number' ? v : '')}
          w={180}
        />
        <Button
          size="xs"
          variant="default"
          mt={20}
          onClick={() =>
            run(
              () =>
                api.updateChapterLedger(seriesID, {
                  cutoffNum: Math.round(Number(cutoff || 0) * 100),
                }),
              'Cutoff updated',
            )
          }
        >
          Save cutoff
        </Button>
        <Button
          size="xs"
          mt={20}
          leftSection={<IconSearch size={14} />}
          loading={busy}
          onClick={() =>
            run(() => api.search(seriesID, { ledger: 'chapter' }), 'Chapter search queued')
          }
        >
          Search &amp; grab wanted
        </Button>
      </Group>
      <Text size="sm" c="dimmed">
        {owned}/{monitored.length} monitored chapters owned · {wanted.length} wanted
      </Text>
      <SlotLegend />
      {!showAll && chapters.length > 300 && (
        <Text size="xs" c="dimmed">
          Showing the last 300 of {chapters.length} chapters —{' '}
          <Text span size="xs" c="indigo" style={{ cursor: 'pointer' }} onClick={() => setShowAll(true)}>
            show all
          </Text>
        </Text>
      )}
      <Group gap={3}>
        {(showAll ? chapters : chapters.slice(-300)).map((c) => (
          <Tooltip
            key={c.number}
            label={`c${fmtNum(c.number)}${c.wanted ? ' · wanted' : ''} — click for details`}
          >
            <Badge
              size="xs"
              style={{ cursor: 'pointer' }}
              variant={c.fileIds?.length ? 'filled' : c.wanted ? 'outline' : 'light'}
              color={c.fileIds?.length ? 'green' : c.wanted ? 'yellow' : 'gray'}
              onClick={() =>
                onSlot({
                  ledger: 'chapter',
                  number: c.number,
                  monitored: c.monitored,
                  wanted: c.wanted,
                  fileIds: c.fileIds ?? [],
                })
              }
            >
              {fmtNum(c.number)}
            </Badge>
          </Tooltip>
        ))}
      </Group>
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
  const [rebind, setRebind] = useState<SeriesSiteView | null>(null)
  const { data: sites } = useQuery({
    queryKey: ['series', seriesID, 'indexers'],
    queryFn: () => api.seriesIndexers(seriesID),
  })

  // Omitted fields decode to nil pointers server-side (= inherit), so
  // undefined and an explicit null are equivalent here. run() invalidates
  // the series queries, which re-fetches this tab.
  const override = (s: SeriesSiteView, patch: Record<string, boolean | undefined>) =>
    run(() =>
      api.setSeriesIndexerOverride(seriesID, s.provider, s.siteId, {
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
      <Table maw={860}>
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
              <Table.Tr key={`${s.provider}-${s.siteId}`} opacity={s.reachable ? 1 : 0.5}>
                <Table.Td>
                  <Group gap={6}>
                    <Text size="sm">{s.name}</Text>
                    <Badge size="xs" variant="outline" color={s.protocol === 'torrent' ? 'blue' : 'teal'}>
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
                  {s.provider === 'suwayomi' ? (
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
                    {s.provider === 'suwayomi' && (
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
          (sites?.find((x) => x.provider === rebind.provider && x.siteId === rebind.siteId) ??
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
  site: SeriesSiteView
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
                #{b.sourceMangaId}
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
  site: SeriesSiteView | null
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState(seriesTitle)
  const [results, setResults] = useState<{ id: number; title: string }[]>([])

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
                  .sourceSearch(site.siteId, query)
                  .then((r) => setResults(r.mangas.map((m) => ({ id: m.id, title: m.title })))),
              )
            }
          >
            Search
          </Button>
        </Group>
        {results.map((m) => {
          const isBound = site.bindings?.some(
            (b) => b.sourceMangaId === m.id && b.state !== 'unmatched',
          )
          return (
            <Group key={m.id} justify="space-between">
              <Text size="sm">{m.title}</Text>
              <Button
                size="compact-xs"
                disabled={isBound}
                onClick={() =>
                  run(
                    () =>
                      api.addBinding(seriesID, {
                        sourceId: site.siteId,
                        sourceMangaId: m.id,
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
