import { useCallback, useEffect, useMemo, useState } from 'react'
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
  Select,
  Stack,
  Switch,
  Tabs,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { IconRefresh, IconSearch, IconTrash } from '@tabler/icons-react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  api,
  fmtNum,
  fmtSize,
  type Binding,
  type ChapterLedger,
  type Collection,
  type Series,
  type SeriesFile,
  type Slots,
  type Source,
  type VolumeCandidate,
} from '../api'

export default function SeriesPage() {
  const { id } = useParams()
  const seriesID = Number(id)
  const navigate = useNavigate()

  const [series, setSeries] = useState<Series | null>(null)
  const [collections, setCollections] = useState<Collection[]>([])
  const [ledger, setLedger] = useState<ChapterLedger | null>(null)
  const [slots, setSlots] = useState<Slots | null>(null)
  const [bindings, setBindings] = useState<Binding[]>([])
  const [files, setFiles] = useState<SeriesFile[]>([])
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api
      .getSeries(seriesID)
      .then((d) => {
        setSeries(d.series)
        setCollections(d.collections)
        setLedger(d.chapterLedger)
      })
      .catch((e) => setError(String(e)))
    api.slots(seriesID).then(setSlots).catch(() => {})
    api.bindings(seriesID).then(setBindings).catch(() => {})
    api.files(seriesID).then(setFiles).catch(() => {})
  }, [seriesID])

  useEffect(load, [load])

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

  if (!series) return error ? <Alert color="red">{error}</Alert> : null

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
          <Tabs.Tab value="sources">Sources</Tabs.Tab>
          <Tabs.Tab value="files">Files ({files.length})</Tabs.Tab>
        </Tabs.List>

        <Tabs.Panel value="volumes" pt="md">
          <VolumesTab
            seriesID={seriesID}
            collections={collections}
            slots={slots}
            busy={busy}
            run={run}
          />
        </Tabs.Panel>

        <Tabs.Panel value="chapters" pt="md">
          <ChaptersTab seriesID={seriesID} ledger={ledger} slots={slots} busy={busy} run={run} />
        </Tabs.Panel>

        <Tabs.Panel value="sources" pt="md">
          <SourcesTab
            seriesID={seriesID}
            seriesTitle={series.title}
            bindings={bindings}
            busy={busy}
            run={run}
          />
        </Tabs.Panel>

        <Tabs.Panel value="files" pt="md">
          <Stack gap="xs">
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
      </Tabs>
    </>
  )
}

function VolumesTab({
  seriesID,
  collections,
  slots,
  busy,
  run,
}: {
  seriesID: number
  collections: Collection[]
  slots: Slots | null
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const [funnel, setFunnel] = useState<VolumeCandidate[] | null>(null)
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
              () => api.searchVolumes(seriesID, false).then((r) => setFunnel(r.candidates)),
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
          onClick={() => run(() => api.searchVolumes(seriesID, true).then((r) => setFunnel(r.candidates)))}
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
                  }`}
                >
                  <Badge
                    size="sm"
                    variant={s.fileIds?.length ? 'filled' : s.wanted ? 'outline' : 'light'}
                    color={s.fileIds?.length ? 'green' : s.wanted ? 'yellow' : 'gray'}
                  >
                    {fmtNum(s.number)}
                  </Badge>
                </Tooltip>
              ))}
            </Group>
          </Card>
        )
      })}

      {funnel && (
        <Card withBorder padding="sm">
          <Text fw={600} mb="xs">
            Search funnel ({funnel.length} releases)
          </Text>
          <Stack gap={4}>
            {funnel.slice(0, 30).map((c) => (
              <Group key={c.guid} justify="space-between" wrap="nowrap">
                <Text size="xs" lineClamp={1} style={{ flex: 1 }}>
                  {c.title}
                </Text>
                <Group gap={6} wrap="nowrap">
                  <Text size="xs" c="dimmed">
                    {fmtSize(c.size)} · {c.seeders}S
                  </Text>
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
      )}
    </Stack>
  )
}

function ChaptersTab({
  seriesID,
  ledger,
  slots,
  busy,
  run,
}: {
  seriesID: number
  ledger: ChapterLedger | null
  slots: Slots | null
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const chapters = slots?.chapters ?? []
  const monitored = chapters.filter((c) => c.monitored)
  const owned = monitored.filter((c) => (c.fileIds?.length ?? 0) > 0).length
  const wanted = chapters.filter((c) => c.wanted)
  const [cutoff, setCutoff] = useState<number | ''>('')

  useEffect(() => {
    if (ledger) setCutoff(ledger.cutoffNum / 100)
  }, [ledger])

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
          onClick={() => run(() => api.searchChapters(seriesID, false), 'Chapter search queued')}
        >
          Search &amp; grab wanted
        </Button>
      </Group>
      <Text size="sm" c="dimmed">
        {owned}/{monitored.length} monitored chapters owned · {wanted.length} wanted · source has
        seen up to {fmtNum(ledger.sourceSeenMax)}
      </Text>
      <Group gap={3}>
        {chapters.slice(-300).map((c) => (
          <Tooltip key={c.number} label={`c${fmtNum(c.number)}${c.wanted ? ' · wanted' : ''}`}>
            <Badge
              size="xs"
              variant={c.fileIds?.length ? 'filled' : c.wanted ? 'outline' : 'light'}
              color={c.fileIds?.length ? 'green' : c.wanted ? 'yellow' : 'gray'}
            >
              {fmtNum(c.number)}
            </Badge>
          </Tooltip>
        ))}
      </Group>
    </Stack>
  )
}

function SourcesTab({
  seriesID,
  seriesTitle,
  bindings,
  busy,
  run,
}: {
  seriesID: number
  seriesTitle: string
  bindings: Binding[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
}) {
  const [sources, setSources] = useState<Source[]>([])
  const [modalOpen, setModalOpen] = useState(false)
  const [sourceID, setSourceID] = useState<string | null>(null)
  const [query, setQuery] = useState(seriesTitle)
  const [results, setResults] = useState<{ id: number; title: string }[]>([])

  useEffect(() => {
    api.sources().then(setSources).catch(() => {})
  }, [])

  const sourceName = useMemo(
    () => Object.fromEntries(sources.map((s) => [s.id, `${s.name} (${s.lang})`])),
    [sources],
  )

  return (
    <Stack>
      <Group>
        <Button size="xs" onClick={() => setModalOpen(true)}>
          Bind a source
        </Button>
      </Group>
      {bindings.map((b) => (
        <Card key={b.id} withBorder padding="xs">
          <Group justify="space-between">
            <Text size="sm">
              {b.sourceName || sourceName[b.sourceId] || b.sourceId}{' '}
              <Text span size="xs" c="dimmed">
                manga #{b.sourceMangaId} · {b.state}
              </Text>
            </Text>
            <Button
              size="compact-xs"
              color="red"
              variant="subtle"
              onClick={() => run(() => api.deleteBinding(seriesID, b.id))}
            >
              Unbind
            </Button>
          </Group>
        </Card>
      ))}
      {bindings.length === 0 && (
        <Text c="dimmed" size="sm">
          No source bindings — bind a Suwayomi source to enable chapter downloads.
        </Text>
      )}

      <Modal opened={modalOpen} onClose={() => setModalOpen(false)} title="Bind a source" size="lg">
        <Stack>
          <Select
            label="Source"
            data={sources.map((s) => ({ value: s.id, label: `${s.name} (${s.lang})` }))}
            value={sourceID}
            onChange={setSourceID}
            searchable
          />
          <Group align="flex-end">
            <TextInput
              label="Search"
              value={query}
              onChange={(e) => setQuery(e.currentTarget.value)}
              style={{ flex: 1 }}
            />
            <Button
              loading={busy}
              disabled={!sourceID}
              onClick={() =>
                run(() =>
                  api.sourceSearch(sourceID!, query).then((r) =>
                    setResults(r.mangas.map((m) => ({ id: m.id, title: m.title }))),
                  ),
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
                          sourceId: sourceID!,
                          sourceMangaId: m.id,
                          sourceName: sourceName[sourceID!],
                        })
                        .then(() => setModalOpen(false)),
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
    </Stack>
  )
}
