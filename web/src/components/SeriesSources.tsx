import { Fragment, useState, type ReactNode } from 'react'
import {
  Anchor,
  ActionIcon,
  Badge,
  Button,
  Checkbox,
  Group,
  Modal,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight, IconPlus, IconX } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, type Edition, type SeriesIndexerView } from '../api'
import { QueryFeedback } from './QueryFeedback'
import { EmptyState } from './EmptyState'
import classes from './SeriesSources.module.css'

// The series' sources: the indexers and extension sources its searches
// use, and what each matched this series to.
export function SeriesSources({
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
          hint="A source is searched through the entries matched to this series. Matches are found automatically when you search; untick one that returns the wrong series. Set the edition a match publishes so its releases count as evidence for that edition; searching from an edition alone is not evidence."
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
                description="Add the new chapters it lists automatically. Untrusted, they wait in Review."
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
