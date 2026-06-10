import { useCallback, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Image,
  Modal,
  NumberInput,
  Progress,
  Stack,
  Switch,
  Table,
  Tabs,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { IconRefresh, IconSearch, IconTrash } from '@tabler/icons-react'
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

export default function SeriesPage() {
  const { id } = useParams()
  const seriesID = Number(id)
  const navigate = useNavigate()

  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [slotSel, setSlotSel] = useState<SlotSelection | null>(null)

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
    setError(null)
    try {
      await fn()
      if (doneMsg) setNotice(doneMsg)
      load()
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  if (!series) {
    const err = error ?? (detail.error ? String(detail.error) : null)
    return err ? <Alert color="red">{err}</Alert> : null
  }

  return (
    <>
      <Group align="flex-start" wrap="nowrap" mb="md">
        <Image src={series.coverUrl} w={140} radius="sm" alt="" />
        <div style={{ flex: 1 }}>
          <Title order={2}>{series.title}</Title>
          <Group gap={6} mt={4}>
            <Badge>{series.status}</Badge>
            <Badge variant="light">{series.type}</Badge>
            {series.finalVolume > 0 && <Badge variant="outline">{series.finalVolume} volumes</Badge>}
            {series.totalChapters > 0 && (
              <Badge variant="outline">{series.totalChapters} chapters</Badge>
            )}
          </Group>
          <Text size="sm" c="dimmed" mt="xs" lineClamp={3}>
            {series.description}
          </Text>
          <Group mt="sm">
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
              onClick={() => run(() => api.deleteSeries(seriesID).then(() => navigate('/')))}
            >
              Remove
            </Button>
          </Group>
        </div>
      </Group>

      {error && (
        <Alert color="red" mb="sm" withCloseButton onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {notice && (
        <Alert color="green" mb="sm" withCloseButton onClose={() => setNotice(null)}>
          {notice}
        </Alert>
      )}

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
                        setNotice(
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
              <Text size="xs" c="dimmed">
                {d.createdAt}
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
      </Group>

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
                    {c.language} · {c.medium} · {c.publisher}
                  </Text>
                </Text>
                <Text size="xs" c="dimmed">
                  {owned}/{colSlots.length} owned
                </Text>
              </div>
              <Switch
                label="Monitored"
                checked={c.monitored}
                onChange={(e) =>
                  run(() => api.setCollectionMonitored(seriesID, c.id, e.currentTarget.checked))
                }
              />
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
      <Group gap={3}>
        {chapters.slice(-300).map((c) => (
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
        only. Extension sources are bound automatically on first search (≥0.90 title match) —
        rebind if the wrong manga matched.
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
                    s.binding ? (
                      <Text size="xs" c="dimmed">
                        manga #{s.binding.sourceMangaId}
                      </Text>
                    ) : (
                      <Text size="xs" c="dimmed">
                        auto on search
                      </Text>
                    )
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
                        {s.binding ? 'Rebind' : 'Bind'}
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
        site={rebind}
        busy={busy}
        run={run}
        onClose={() => setRebind(null)}
      />
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
        {results.map((m) => (
          <Group key={m.id} justify="space-between">
            <Text size="sm">{m.title}</Text>
            <Button
              size="compact-xs"
              onClick={() =>
                run(
                  () =>
                    api
                      .addBinding(seriesID, {
                        sourceId: site.siteId,
                        sourceMangaId: m.id,
                        sourceName: site.name,
                      })
                      .then(onClose),
                  'Source bound',
                )
              }
            >
              Bind
            </Button>
          </Group>
        ))}
      </Stack>
    </Modal>
  )
}
