import type { UseQueryResult } from '@tanstack/react-query'
import { api } from '../../api'
import type {
  AssetCoverage,
  CollectedVersion,
  Edition,
  File as SeriesFile,
} from '../../types/store'
import type { BookStatus } from '../../types/service'
import { BookEditor } from '../BookEditor'
import { BulkFactsDialog } from '../BulkFactsDialog'
import { CatalogLink } from '../CatalogLink'
import { CollectAgain } from '../CollectAgain'
import { DownloadSettings } from '../DownloadSettings'
import { EditionEditor } from '../EditionEditor'
import { EditorModal } from '../EditorModal'
import { MappingEditor } from '../MappingEditor'
import { QueryFeedback } from '../QueryFeedback'
import type { EditionDialogState } from './useEditionDialogs'

type Props = {
  seriesID: number
  editions: Edition[]
  // All editions in display order, for the link dialog.
  visible: Edition[]
  files: SeriesFile[]
  allVersions: CollectedVersion[]
  allBooks: BookStatus[]
  coverage: UseQueryResult<AssetCoverage[]>
  details: UseQueryResult<Record<string, BookStatus[]>>
  dialogs: EditionDialogState
  // The series' release profile (0: the instance default).
  seriesProfileId: number
  changed: () => Promise<void>
  onReaderNameChanged: () => void
}

export function EditionDialogs({
  seriesID,
  editions,
  visible,
  files,
  allVersions,
  allBooks,
  coverage,
  details,
  dialogs,
  seriesProfileId,
  changed,
  onReaderNameChanged,
}: Props) {
  const {
    version,
    setVersion,
    book,
    setBook,
    edition,
    setEdition,
    collectAgain,
    setCollectAgain,
    link,
    setLink,
    file,
    setFile,
    correcting,
    setCorrecting,
  } = dialogs
  return (
    <>
      {version && (
        <DownloadSettings
          key={version.id}
          value={version}
          chapter={editions.find((c) => c.id === version.editionId)?.type === 'chapter'}
          save={async (value) => {
            await api.saveVersion(seriesID, value)
            await changed()
            setVersion(null)
          }}
          close={() => setVersion(null)}
        />
      )}
      {book && (
        <BookEditor
          key={book.id}
          value={book}
          seriesID={seriesID}
          edition={editions.find((c) => c.id === book.editionId)}
          save={async (value) => {
            await api.saveBook(seriesID, value)
            await changed()
            setBook(null)
          }}
          reset={async () => {
            await api.resetBook(seriesID, book.id)
            await changed()
            setBook(null)
          }}
          remove={async () => {
            await api.deleteBook(seriesID, book.id)
            await changed()
            setBook(null)
          }}
          close={() => setBook(null)}
        />
      )}
      {edition && (
        <EditionEditor
          seriesID={seriesID}
          value={edition}
          saveReaderFields={(readerFields) =>
            api.updateEdition(seriesID, edition.id, { readerFields })
          }
          reset={async () => {
            await api.editEdition(seriesID, edition.id, {
              reset: ['edition_name', 'language', 'publisher', 'medium'],
            })
            await changed()
            setEdition(null)
          }}
          save={async (patch, readerNameChanged) => {
            await api.editEdition(seriesID, edition.id, {
              ...patch,
              reset: [],
            })
            await changed()
            setEdition(null)
            if (readerNameChanged && files.length > 0) onReaderNameChanged()
          }}
          close={() => setEdition(null)}
        />
      )}
      {collectAgain && (
        <CollectAgain
          seriesID={seriesID}
          edition={collectAgain}
          versions={allVersions.filter((t) => t.editionId === collectAgain.id)}
          seriesProfileId={seriesProfileId}
          save={async () => {
            await changed()
            setCollectAgain(null)
          }}
          close={() => setCollectAgain(null)}
        />
      )}
      {link && (
        <CatalogLink
          seriesID={seriesID}
          value={link}
          editions={visible}
          done={async () => {
            await changed()
            setLink(null)
          }}
          close={() => setLink(null)}
        />
      )}
      {file && (!details.data || !coverage.data) && (
        <EditorModal
          title="Match imported file"
          dirty={false}
          busy={false}
          onClose={() => setFile(null)}
        >
          <QueryFeedback
            pending={details.isPending || coverage.isPending}
            error={details.error || coverage.error}
            onRetry={() => {
              void details.refetch()
              void coverage.refetch()
            }}
          />
        </EditorModal>
      )}
      {file && details.data && coverage.data && (
        <MappingEditor
          seriesID={seriesID}
          value={file}
          versions={allVersions}
          existing={coverage.data?.filter((a) => a.fileId === file.id) ?? []}
          books={allBooks}
          editions={editions}
          applied={async () => {
            await changed()
            setFile(null)
          }}
          close={() => setFile(null)}
        />
      )}
      {correcting && (
        <BulkFactsDialog
          seriesID={seriesID}
          files={files.filter((f) => correcting.fileIds.includes(f.id))}
          hint={correcting.hint}
          done={changed}
          close={() => setCorrecting(null)}
        />
      )}
    </>
  )
}
