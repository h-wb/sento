import { useQuery } from '@tanstack/react-query'
import { seriesQueries } from '../../queries'
import type { Edition } from '../../types/store'
import { refusedFiles } from '../refusal'

// One series' edition screen state: its queries plus the yours/others
// split of the edition list, shared by the Editions and Files sections.
export function useSeriesEditions(seriesID: number, editions: Edition[], seriesTitle: string) {
  const versions = useQuery(seriesQueries.versions(seriesID))
  const overview = useQuery(seriesQueries.overview(seriesID))
  const coverage = useQuery(seriesQueries.coverage(seriesID))
  const readerNames = useQuery(seriesQueries.readerNames(seriesID))
  const details = useQuery(seriesQueries.versionBooks(seriesID))
  const allVersions = versions.data ?? []
  const allBooks = Object.values(details.data ?? {}).flat()
  // A card's series name in readers, when it is not just the series title.
  const readerName = (versionId: number) => {
    const name = readerNames.data?.find((n) => n.versionId === versionId)?.fields.series
    return name && name !== seriesTitle ? name : undefined
  }
  const refused = (versionId: number) => refusedFiles(versionId, allBooks)
  const visible = [...editions].sort(
    (a, b) =>
      Number(b.monitored) - Number(a.monitored) ||
      Number(a.type === 'chapter') - Number(b.type === 'chapter') ||
      a.editionName.localeCompare(b.editionName),
  )
  const summary = (id: number) => overview.data?.versions.find((t) => t.versionId === id)
  const isYours = (c: Edition) =>
    (c.custom && c.numberingSource === 'manual') ||
    allVersions.some(
      (t) => t.editionId === c.id && (t.monitored || (summary(t.id)?.onDisk ?? 0) > 0),
    )
  const yours = visible.filter(isYours)
  const cardCount = allVersions.filter((t) => yours.some((c) => c.id === t.editionId)).length
  const others = visible.filter((c) => !isYours(c))
  const pending = versions.isPending || overview.isPending
  return {
    versions,
    overview,
    coverage,
    details,
    allVersions,
    allBooks,
    readerName,
    refused,
    visible,
    summary,
    yours,
    cardCount,
    others,
    pending,
  }
}
