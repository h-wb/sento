import { useState } from 'react'
import {
  Alert,
  Anchor,
  Button,
  Group,
  Stack,
  Text,
} from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import { invalidateLibrary } from '../queries'
import type {
  Edition,
  CollectedVersion,
  Book,
  File as SeriesFile,
} from '../types/store'
import type { BookSelection } from './BookDialog'
import type { EditionStart } from './AddEdition'
import { QueryFeedback } from './QueryFeedback'
import { useConfirmAction } from './ConfirmAction'
import { MangaBakaMark } from './Origin'
import SeriesFiles from './SeriesFiles'
import { useEditor } from './useEditor'
import { EditionCard } from './editions/EditionCard'
import { OtherEditions } from './editions/OtherEditions'
import { EditionDialogs } from './editions/EditionDialogs'
import { useSeriesEditions } from './editions/useSeriesEditions'

// Editions and Files share their book, version and file-mapping dialogs.
export type EditionSection = 'editions' | 'files'
type Props = {
  section: EditionSection
  seriesID: number
  // Cards named otherwise in readers say so.
  seriesTitle: string
  editions: Edition[]
  // The volume count MangaBaka gives for the series, 0 when it gives none.
  volumeCount: number
  // The series' page on MangaBaka.
  catalogUrl: string
  files: SeriesFile[]
  onBook: (s: BookSelection) => void
  onAddEdition: (start?: EditionStart) => void
  // The series' release profile (0: the instance default).
  seriesProfileId: number
  // Open the organize preview: an edition's name in readers changed, and
  // the files on disk still carry the old one.
  onReaderNameChanged: () => void
}
export default function EditionManager({
  section,
  seriesID,
  seriesTitle,
  editions,
  volumeCount,
  catalogUrl,
  files,
  onBook,
  onAddEdition,
  seriesProfileId,
  onReaderNameChanged,
}: Props) {
  const qc = useQueryClient()
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const {
    versions,
    overview,
    coverage,
    details,
    allVersions,
    readerName,
    refused,
    visible,
    summary,
    yours,
    cardCount,
    others,
    pending,
  } = useSeriesEditions(seriesID, editions, seriesTitle)
  const [collectAgain, setCollectAgain] = useState<Edition | null>(null)
  const [version, setVersion] = useState<CollectedVersion | null>(null)
  const [book, setBook] = useState<Book | null>(null)
  const [edition, setEdition] = useState<Edition | null>(null)
  const [link, setLink] = useState<Edition | null>(null)
  const [file, setFile] = useState<SeriesFile | null>(null)
  const editor = useEditor()
  const { confirm, confirmation } = useConfirmAction()
  const changed = async () => {
    await invalidateLibrary(qc)
  }
  const run = (fn: () => Promise<unknown>) =>
    editor.run(async () => {
      await fn()
      await changed()
    })
  const [correcting, setCorrecting] = useState<{ fileIds: number[]; hint: string } | null>(null)
  const [otherEditions, setOtherEditions] = useState(false)
  const [language, setLanguage] = useState<string | null>(null)
  const [layout, setLayout] = useState<string | null>(null)

  return (
    <Stack gap="lg">
      {section === 'editions' && (
        <>
          <QueryFeedback
            pending={pending}
            error={versions.error || overview.error}
            onRetry={() => {
              void versions.refetch()
              void overview.refetch()
            }}
          />
          {editor.error && <Alert color="red">{editor.error}</Alert>}
          {!pending && !editions.some((c) => c.type === 'volume') && (
            <Alert
              color="blue"
              variant="light"
              icon={<MangaBakaMark />}
              title="MangaBaka lists no volumes for this series"
            >
              <Group justify="space-between" align="center" gap="sm">
                <Text size="sm" style={{ flex: 1, minWidth: 220 }}>
                  Its volumes are not in MangaBaka’s catalog yet, so there is nothing to collect
                  from.{' '}
                  {volumeCount > 0 &&
                    `MangaBaka only gives a count for the original run: ${volumeCount} volumes. `}
                  You can add the edition yourself and link it when MangaBaka lists it.{' '}
                  <Anchor href={catalogUrl} target="_blank" rel="noreferrer" size="sm">
                    See the series on MangaBaka
                  </Anchor>
                </Text>
                <Button size="xs" onClick={() => onAddEdition()}>
                  Add a volume edition
                </Button>
              </Group>
            </Alert>
          )}
          {!pending && yours.length === 0 && (
            <Text size="sm" c="dimmed">
              No edition is being collected yet. Pick one from the other editions below, or add your
              own.
            </Text>
          )}
          {!pending &&
            // One card per way of collecting an edition: the edition with
            // a release profile. Two cards of one edition are the same
            // books wanted as different releases (colored, black-and-white).
            yours.flatMap((c) => {
              const versions = allVersions.filter((t) => t.editionId === c.id)
              return versions.map((t) => (
                <EditionCard
                  key={t.id}
                  seriesID={seriesID}
                  edition={c}
                  version={t}
                  editionVersions={versions}
                  allVersions={allVersions}
                  stats={summary(t.id)}
                  cardCount={cardCount}
                  refusal={refused(t.id)}
                  catalogUrl={catalogUrl}
                  visible={visible}
                  readerName={readerName}
                  files={files}
                  coverage={coverage.data ?? []}
                  busy={editor.busy}
                  expanded={expanded}
                  setExpanded={setExpanded}
                  run={run}
                  changed={changed}
                  onBook={onBook}
                  onEdit={setBook}
                  onFile={setFile}
                  confirm={confirm}
                  onLink={setLink}
                  onEditEdition={setEdition}
                  onDownloadSettings={setVersion}
                  onCollectAgain={setCollectAgain}
                  onCorrecting={setCorrecting}
                />
              ))
            })}
          <OtherEditions
            others={others}
            otherEditions={otherEditions}
            setOtherEditions={setOtherEditions}
            language={language}
            setLanguage={setLanguage}
            layout={layout}
            setLayout={setLayout}
            busy={editor.busy}
            run={run}
            seriesID={seriesID}
            allVersions={allVersions}
            onAddEdition={() => onAddEdition()}
          />
        </>
      )}
      {section === 'files' && files.length > 0 && (
        <Stack gap="sm">
          {editor.error && <Alert color="red">{editor.error}</Alert>}
          <SeriesFiles
            seriesID={seriesID}
            files={files}
            editions={editions}
            versions={allVersions}
            coverage={coverage.data}
            books={details.data ? Object.values(details.data).flat() : undefined}
            onOpen={setFile}
            changed={changed}
          />
        </Stack>
      )}
      <EditionDialogs
        seriesID={seriesID}
        editions={editions}
        visible={visible}
        files={files}
        allVersions={allVersions}
        allBooks={Object.values(details.data ?? {}).flat()}
        coverage={coverage}
        details={details}
        version={version}
        setVersion={setVersion}
        book={book}
        setBook={setBook}
        edition={edition}
        setEdition={setEdition}
        collectAgain={collectAgain}
        setCollectAgain={setCollectAgain}
        link={link}
        setLink={setLink}
        file={file}
        setFile={setFile}
        correcting={correcting}
        setCorrecting={setCorrecting}
        seriesProfileId={seriesProfileId}
        changed={changed}
        onReaderNameChanged={onReaderNameChanged}
        confirmation={confirmation}
      />
    </Stack>
  )
}
