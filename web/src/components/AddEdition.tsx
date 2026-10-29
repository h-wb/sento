import { useState } from 'react'
import {
  Alert,
  Autocomplete,
  Button,
  Divider,
  Group,
  NumberInput,
  SegmentedControl,
  Select,
  Stack,
  Text,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { api, type Edition } from '../api'
import type { Series } from '../types/store'
import type { SeriesPublisher } from '../types/service'
import { EditorModal } from './EditionDialogs'
import { editionDetails } from './editionText'
import { Origin } from './Origin'

const languages = ['en', 'fr', 'es', 'de', 'it', 'pt-br', 'ja', 'ko', 'zh']

// AddEdition is the one place an edition is added to a series. It first
// offers what MangaBaka lists and nobody collects yet, then the form for
// an edition MangaBaka does not list. What the form pre-fills from
// MangaBaka's series record (the publisher, the volume count) says so.
// volumes: the count releases name, when they name one.
export type EditionStart = { editionName: string; language: string; volumes?: number }

export default function AddEdition({
  seriesID,
  series,
  publishers,
  editions,
  busy,
  run,
  onClose,
  initial,
  onCreated,
}: {
  seriesID: number
  series: Series
  publishers: SeriesPublisher[]
  editions: Edition[]
  busy: boolean
  run: (fn: () => Promise<unknown>, msg?: string) => void
  onClose: () => void
  // Where to start, when review proposes an edition releases state.
  initial?: EditionStart
  onCreated?: (e: Edition) => void
}) {
  const startName = initial?.editionName ?? 'Standard Edition'
  const startLanguage = initial?.language ?? ''
  const hasVolumes = editions.some((c) => c.type === 'volume')
  const [ctype, setCtype] = useState<'volume' | 'chapter'>('volume')
  const [name, setName] = useState(startName)
  const [language, setLanguage] = useState(startLanguage)
  // null: follow MangaBaka's suggestion for the language.
  const [publisherInput, setPublisher] = useState<string | null>(null)
  const [medium, setMedium] = useState<string | null>(null)
  const [basedOn, setBasedOn] = useState<string | null>(null)
  // null: follow MangaBaka's volume count.
  const startVolumes = initial?.volumes ?? null
  const [volumesInput, setVolumes] = useState<number | string | null>(startVolumes)
  const [cutoff, setCutoff] = useState<number | string>(0)
  const [more, setMore] = useState(false)

  const versions = useQuery({
    queryKey: ['series', seriesID, 'versions'],
    queryFn: () => api.versions(seriesID),
  })
  // The catalog's edition names, when it can be reached; any name works.
  const catalogEditions =
    useQuery({
      queryKey: ['catalog', 'editions'],
      queryFn: api.catalogEditions,
      staleTime: Infinity,
    }).data ?? []

  const listed = editions.filter((c) => !c.custom && !c.monitored && c.syncState !== 'stale')
  const lang = language.trim().toLowerCase()
  // Named in the summary while the Add button waits on them.
  const missing = [!language.trim() && 'a language', !name.trim() && 'an edition name'].filter(
    (m) => !!m,
  )
  const suggested = publishers.find((p) => p.language.toLowerCase() === lang)
  const publisher = publisherInput ?? suggested?.name ?? ''
  const counted = series.finalVolume > 0 ? series.finalVolume : 0
  const volumes = volumesInput ?? counted
  const count = ctype === 'volume' && !basedOn ? Number(volumes) || 0 : 0
  const source = editions.find((c) => String(c.id) === basedOn)

  const create = () =>
    run(
      () =>
        api
          .createEdition(seriesID, {
            editionName: name,
            providerEditionId: catalogEditions.find((e) => e.name === name)?.id,
            language,
            publisher,
            medium: medium ?? '',
            type: ctype,
            basedOn: basedOn ? Number(basedOn) : 0,
            volumes: count,
            cutoffNum: ctype === 'chapter' ? Math.round((Number(cutoff) || 0) * 100) : 0,
            monitored: true,
          })
          .then((e) => {
            onClose()
            onCreated?.(e)
          }),
      'Edition added',
    )
  const collect = (c: Edition) => {
    const t = versions.data?.find((t) => t.editionId === c.id && t.isDefault)
    if (t)
      run(
        () => api.saveVersion(seriesID, { ...t, monitored: true }).then(onClose),
        'Edition collected',
      )
  }

  const books =
    ctype === 'chapter'
      ? 'no chapters yet'
      : source
        ? `the volume numbers of ${source.editionName} (${source.language})`
        : count > 0
          ? `volumes 1 to ${count}`
          : 'no volumes yet'

  return (
    <EditorModal
      dirty={
        name !== startName ||
        language !== startLanguage ||
        publisherInput !== null ||
        !!medium ||
        !!basedOn ||
        volumesInput !== startVolumes ||
        Number(cutoff) !== 0 ||
        ctype !== 'volume'
      }
      busy={busy}
      onClose={onClose}
      title="Add edition"
      size="lg"
    >
      <Stack gap="md">
        <Stack gap="xs">
          <Group gap="xs">
            <Origin kind="catalog" compact />
            <Text fw={600} size="sm">
              Listed in MangaBaka
            </Text>
          </Group>
          {listed.length === 0 && (
            <Text size="sm" c="dimmed">
              {hasVolumes || editions.some((c) => !c.custom)
                ? 'You already collect every edition MangaBaka lists for this series.'
                : 'MangaBaka lists no editions for this series yet. Add the one you want below; you will be offered to link it when MangaBaka lists it.'}
            </Text>
          )}
          {listed.map((c) => (
            <Group key={c.id} justify="space-between" wrap="nowrap" gap="sm">
              <div style={{ minWidth: 0 }}>
                <Text size="sm" fw={500}>
                  {c.editionName}
                </Text>
                <Text size="xs" c="dimmed">
                  {editionDetails(
                    c,
                    `${c.countMain} ${c.type === 'chapter' ? 'chapters' : 'volumes'}`,
                  )}
                </Text>
              </div>
              <Button
                size="xs"
                variant="default"
                disabled={busy || !versions.data}
                aria-label={`Collect ${c.editionName} ${c.language}`}
                onClick={() => collect(c)}
              >
                Collect
              </Button>
            </Group>
          ))}
        </Stack>

        <Divider />

        <Stack gap="sm">
          <Group gap="xs">
            <Text fw={600} size="sm">
              Not listed: add it yourself
            </Text>
            <Origin kind="local" />
          </Group>
          <Text size="sm" c="dimmed">
            For a language or publication MangaBaka does not list. To keep another release group of
            an edition you already have, add a collected version instead.
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
          <Group grow align="flex-start">
            <Autocomplete
              label="Language"
              required
              placeholder="fr"
              description="Language code"
              data={[...new Set([...publishers.map((p) => p.language), ...languages])]}
              value={language}
              onChange={setLanguage}
            />
            <Autocomplete
              label="Edition name"
              required
              description="MangaBaka’s name for it, when it has one"
              data={catalogEditions.map((e) => e.name)}
              value={name}
              onChange={setName}
            />
          </Group>
          {ctype === 'volume' && (
            <Group grow align="flex-start">
              <Select
                label="Number the volumes like"
                description="Copies the numbers only, not dates or ISBNs"
                placeholder="None: count them"
                clearable
                data={editions
                  .filter((c) => c.type === 'volume')
                  .map((c) => ({
                    value: String(c.id),
                    label: [c.editionName, c.language, c.medium].filter(Boolean).join(' · '),
                  }))}
                value={basedOn}
                onChange={setBasedOn}
              />
              <NumberInput
                label="Volumes"
                description={
                  startVolumes && Number(volumes) === startVolumes ? (
                    `The releases go up to volume ${startVolumes}.`
                  ) : counted > 0 && Number(volumes) === counted ? (
                    <>
                      <Origin kind="catalog" compact /> The original run has {counted}. A
                      translation may have fewer.
                    </>
                  ) : (
                    'Leave at 0 to add volumes one by one'
                  )
                }
                min={0}
                max={500}
                disabled={!!basedOn}
                value={basedOn ? '' : volumes}
                onChange={setVolumes}
              />
            </Group>
          )}
          {ctype === 'chapter' && (
            <NumberInput
              label="Skip through chapter"
              description="Collect chapters after this number. The edition starts empty: add chapters one by one, or number them from MangaBaka’s chapter count in Edit edition."
              min={0}
              decimalScale={2}
              value={cutoff}
              onChange={setCutoff}
            />
          )}
          <Button
            variant="subtle"
            size="compact-sm"
            w="fit-content"
            leftSection={more ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />}
            aria-expanded={more}
            onClick={() => setMore(!more)}
          >
            Publisher and medium
            {!more && (publisher || medium)
              ? `: ${[publisher, medium].filter(Boolean).join(' · ')}`
              : ''}
          </Button>
          {more && (
            <Group grow align="flex-start">
              <Autocomplete
                label="Publisher"
                description={
                  suggested && publisher === suggested.name ? (
                    <>
                      <Origin kind="catalog" compact /> Its {suggested.language} publisher
                      {suggested.note ? ` (“${suggested.note}”)` : ''}
                    </>
                  ) : (
                    'Optional'
                  )
                }
                data={[...new Set(publishers.map((p) => p.name))]}
                value={publisher}
                onChange={setPublisher}
              />
              <Select
                label="Medium"
                description="Optional"
                placeholder="Any"
                clearable
                data={['digital', 'paperback', 'hardcover']}
                value={medium}
                onChange={setMedium}
              />
            </Group>
          )}
          <Alert variant="light" color="gray" p="xs">
            {missing.length ? (
              <Text size="sm">Enter {missing.join(' and ')} to add this edition.</Text>
            ) : (
              <Text size="sm">
                Adds{' '}
                <b>
                  {[name.trim() || 'an edition', language.trim(), publisher.trim(), medium]
                    .filter(Boolean)
                    .join(' · ')}
                </b>{' '}
                as a local edition with {books}. MangaBaka does not update a local edition; when
                it lists this one, Review offers to link them.
              </Text>
            )}
          </Alert>
          <Group justify="flex-end">
            <Button loading={busy} disabled={missing.length > 0} onClick={create}>
              Add edition
            </Button>
          </Group>
        </Stack>
      </Stack>
    </EditorModal>
  )
}
