import { useState } from 'react'
import type { Book, CollectedVersion, Edition, File as SeriesFile } from '../../types/store'

// The edition screen's dialogs: what each one is open on, and its setter.
export function useEditionDialogs() {
  const [collectAgain, setCollectAgain] = useState<Edition | null>(null)
  const [version, setVersion] = useState<CollectedVersion | null>(null)
  const [book, setBook] = useState<Book | null>(null)
  const [edition, setEdition] = useState<Edition | null>(null)
  const [link, setLink] = useState<Edition | null>(null)
  const [file, setFile] = useState<SeriesFile | null>(null)
  const [correcting, setCorrecting] = useState<{ fileIds: number[]; hint: string } | null>(null)
  return {
    collectAgain,
    setCollectAgain,
    version,
    setVersion,
    book,
    setBook,
    edition,
    setEdition,
    link,
    setLink,
    file,
    setFile,
    correcting,
    setCorrecting,
  }
}

export type EditionDialogState = ReturnType<typeof useEditionDialogs>
