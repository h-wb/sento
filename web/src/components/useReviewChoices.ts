import { useEffect, useState } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { notify } from '../ui'
import type {
  ReviewBookPlan,
  ReviewFileChoice,
  ReviewFileOutcome,
  ReviewPlan,
  ReviewPlanRequest,
} from '../types/service'

export const bookKey = (b: { versionId?: number; bookId?: number }) => `${b.versionId}:${b.bookId}`

// "volume 1" as the rest of the app writes it: "Vol. 1".
export const shortBook = (label: string) =>
  label.replace(/^volume /, 'Vol. ').replace(/^chapter /, 'Ch. ')

// What the server makes of a set of files and the books they hold. The
// one-file editor asks again on every edit, so it keeps the last answer on
// screen meanwhile instead of collapsing to a spinner.
export function useReviewPlan(seriesID: number, request: ReviewPlanRequest, keepPrevious = false) {
  return useQuery({
    queryKey: ['series', seriesID, 'review-choices', request],
    queryFn: () => api.previewReviewChoices(seriesID, request),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    placeholderData: keepPrevious ? keepPreviousData : undefined,
  })
}

export function initialChoices(
  plan: ReviewPlan,
  request: ReviewPlanRequest,
  missingOnly = false,
  keepCurrent = false,
): ReviewFileChoice[] {
  const selected = new Set<string>()
  return plan.files.map((f, i) => {
    const disposition =
      (f.reason && request.files[i].books === null) ||
      (f.books.length > 0 &&
        f.books.every((b) => !b.currentSelected && (!!b.reason || !b.incoming.factsKnown)))
        ? 'later'
        : 'keep'
    return {
      fileId: f.fileId,
      disposition,
      use: f.books
        .filter((b) => {
          const wanted = keepCurrent
            ? b.currentSelected
            : missingOnly
              ? b.currentSelected ||
                (!b.reason && b.incoming.factsKnown && !b.owners.some((o) => o.present))
              : b.useIncoming
          if (!wanted || f.reason || disposition === 'later' || selected.has(bookKey(b.book)))
            return false
          selected.add(bookKey(b.book))
          return true
        })
        .map((b) => b.book),
    }
  })
}

// The choices a person makes over a plan: which books each file supplies
// and what happens to the file. Both the pack review and the one-file
// editor draw them their own way.
export function useReviewChoices({
  seriesID,
  request,
  plan,
  keepCurrent,
  applied,
  onDirtyChange,
  onBusyChange,
}: {
  seriesID: number
  request: ReviewPlanRequest
  plan: ReviewPlan
  // Start from the editions that use the file now, proposing no new use:
  // for a file that is already matched.
  keepCurrent?: boolean
  applied: () => Promise<void>
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
}) {
  const defaults = initialChoices(plan, request, false, keepCurrent)
  const [choices, setChoices] = useState(defaults)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [receipt, setReceipt] = useState<ReviewFileOutcome[] | null>(null)
  const qc = useQueryClient()
  const dirty = JSON.stringify(choices) !== JSON.stringify(defaults)
  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => {
    onBusyChange?.(busy)
  }, [busy, onBusyChange])

  const using = (i: number, b: { versionId?: number; bookId?: number }) =>
    choices[i].use.some((v) => bookKey(v) === bookKey(b))
  const update = (i: number, change: Partial<ReviewFileChoice>) =>
    setChoices((v) => v.map((c, j) => (i === j ? { ...c, ...change } : c)))
  // One file per book: ticking a book for one file unticks it elsewhere.
  const selectBook = (i: number, b: ReviewBookPlan, checked: boolean) =>
    setChoices((before) =>
      before.map((c, j) => {
        const use = c.use.filter((v) => bookKey(v) !== bookKey(b.book))
        if (i === j) return { ...c, use: checked ? [...use, b.book] : use, disposition: 'keep' }
        return checked ? { ...c, use } : c
      }),
    )

  let imports = 0
  let replacements = 0
  let kept = 0
  let missing = 0
  const outcomes = new Map<
    string,
    { current: Set<number>; chosen: Set<number>; reviewed: Set<number> }
  >()
  plan.files.forEach((f, i) => {
    f.books.forEach((b) => {
      const key = bookKey(b.book)
      const outcome = outcomes.get(key) ?? {
        current: new Set<number>(),
        chosen: new Set<number>(),
        reviewed: new Set<number>(),
      }
      if (b.currentSelected) outcome.current.add(f.fileId)
      b.owners.filter((o) => o.present).forEach((o) => outcome.current.add(o.fileId))
      if (choices[i].disposition !== 'later') {
        outcome.reviewed.add(f.fileId)
        if (using(i, b.book)) outcome.chosen.add(f.fileId)
      }
      outcomes.set(key, outcome)
    })
  })
  outcomes.forEach((b) => {
    if (b.chosen.size) {
      if ([...b.chosen].some((id) => b.current.has(id))) kept++
      else if (b.current.size) replacements++
      else imports++
    } else if ([...b.current].some((id) => !b.reviewed.has(id))) kept++
    else missing++
  })
  const replacementChosen = (book: ReviewBookPlan['book']) => {
    const outcome = outcomes.get(bookKey(book))
    return !!outcome && [...outcome.chosen].some((id) => !outcome.current.has(id))
  }
  const trash = choices.filter((c) => c.disposition === 'trash').length
  const later = choices.filter((c) => c.disposition === 'later').length

  const refreshAll = () =>
    Promise.all(
      ['series', 'activity', 'review', 'queue', 'wanted'].map((k) =>
        qc.invalidateQueries({
          queryKey: [k],
          // Keep the open editor's snapshot until the saved receipt is dismissed.
          predicate: (q) =>
            !q.queryKey.includes('review-choices') && !q.queryKey.includes('file-review'),
        }),
      ),
    )
  // `release` first saves the files as used by no edition and then moves
  // them to trash: the server trashes only a file nothing uses.
  const apply = async (saved: string, release = false) => {
    setBusy(true)
    setError('')
    try {
      let result = await api.applyReviewChoices(seriesID, {
        request,
        token: plan.token,
        choices: release ? choices.map((c) => ({ ...c, disposition: 'keep' })) : choices,
      })
      if (release && !result.some((f) => f.note)) {
        try {
          const unused = await api.previewReviewChoices(seriesID, request)
          result = await api.applyReviewChoices(seriesID, { request, token: unused.token, choices })
        } catch (e) {
          await refreshAll()
          throw new Error(
            `No edition uses the file any more, but it is still on disk: ${e instanceof Error ? e.message : String(e)}`,
            { cause: e },
          )
        }
      }
      await refreshAll()
      if (result.some((f) => f.note)) {
        setReceipt(result)
        notify.err(
          'Choices saved. Some file operations still need attention; trash moves will retry automatically.',
        )
        return
      }
      notify.ok(saved)
      await applied()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return {
    choices,
    setChoices,
    busy,
    error,
    receipt,
    using,
    update,
    selectBook,
    replacementChosen,
    counts: { imports, replacements, kept, missing, trash, later },
    nothingToApply: choices.every((c) => c.disposition === 'later'),
    apply,
  }
}
