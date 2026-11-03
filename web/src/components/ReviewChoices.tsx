import { useEffect, useState } from 'react'
import { Alert, Badge, Button, Card, Checkbox, Group, Pagination, Select, Stack, Text } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { notify } from '../ui'
import type { FileScore, ReviewPlan, ReviewPlanRequest, ReviewFileChoice, ReviewFileOutcome } from '../types/service'
import { QueryFeedback } from './QueryFeedback'
import { fileName } from './editionText'

type Props = {
  seriesID: number
  request: ReviewPlanRequest
  applied: () => Promise<void>
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
}

function Score({ value }: { value: FileScore }) {
  return (
    <details>
      <summary style={{ fontSize: 'var(--mantine-font-size-xs)', cursor: 'pointer' }}>
        Score {value.score} · {value.profileName || 'No profile'}
      </summary>
      <Text size="xs" c="dimmed">
        {value.formats.map((f) => `${f.name}: ${f.score > 0 ? '+' : ''}${f.score}`).join(' · ') || 'No custom formats matched.'}
      </Text>
      {!value.factsKnown && <Text size="xs" c="orange">File facts are unknown; choose explicitly after checking the archive.</Text>}
    </details>
  )
}

export function ReviewChoices(props: Props) {
  const preview = useQuery({
    queryKey: ['series', props.seriesID, 'review-choices', props.request],
    queryFn: () => api.previewReviewChoices(props.seriesID, props.request),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  if (!preview.data || preview.isFetching) return (
    <QueryFeedback pending={preview.isPending || preview.isFetching} error={preview.error} onRetry={() => void preview.refetch()} />
  )
  return <Choices key={preview.data.token} {...props} plan={preview.data} refresh={() => preview.refetch()} />
}

function Choices({ seriesID, request, applied, onDirtyChange, onBusyChange, plan, refresh }: Props & { plan: ReviewPlan; refresh: () => Promise<unknown> }) {
  const defaults: ReviewFileChoice[] = plan.files.map((f, i) => ({
    fileId: f.fileId,
    use: f.books.filter((b) => b.useIncoming && !f.reason).map((b) => b.book),
    disposition: (f.reason && request.files[i].books === null) || (f.books.length > 0 && f.books.every((b) => !!b.reason || !b.incoming.factsKnown)) ? 'later' : 'keep',
  }))
  const [choices, setChoices] = useState(defaults)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [page, setPage] = useState(1)
  const [receipt, setReceipt] = useState<ReviewFileOutcome[] | null>(null)
  const qc = useQueryClient()
  const dirty = JSON.stringify(choices) !== JSON.stringify(defaults)
  useEffect(() => { onDirtyChange?.(dirty) }, [dirty, onDirtyChange])
  useEffect(() => { onBusyChange?.(busy) }, [busy, onBusyChange])
  const key = (b: { versionId?: number; bookId?: number }) => `${b.versionId}:${b.bookId}`
  const using = (i: number, b: { versionId?: number; bookId?: number }) => choices[i].use.some((v) => key(v) === key(b))
  const update = (i: number, change: Partial<ReviewFileChoice>) =>
    setChoices((v) => v.map((c, j) => i === j ? { ...c, ...change } : c))
  let imports = 0, replacements = 0, kept = 0
  plan.files.forEach((f, i) => {
    if (choices[i].disposition === 'later') return
    f.books.forEach((b) => {
      if (using(i, b.book)) {
        if (b.owners.some((o) => o.present)) replacements++
        else imports++
      } else if (b.owners.some((o) => o.present)) kept++
    })
  })
  const trash = choices.filter((c) => c.disposition === 'trash').length
  const apply = async () => {
    setBusy(true)
    setError('')
    try {
      const outcomes = await api.applyReviewChoices(seriesID, { request, token: plan.token, choices })
      await Promise.all(['series', 'activity', 'review', 'queue', 'wanted'].map((k) => qc.invalidateQueries({
        queryKey: [k],
        // Keep the open editor's snapshot until the saved receipt is dismissed.
        predicate: (q) => !q.queryKey.includes('review-choices') && !q.queryKey.includes('file-review'),
      })))
      const warnings = outcomes.filter((f) => f.note)
      if (warnings.length) {
        setReceipt(outcomes)
        notify.err('Choices saved. Some file operations still need attention; trash moves will retry automatically.')
        return
      }
      notify.ok(`Imported ${imports}; kept existing ${kept}; replaced ${replacements}; moved ${trash} to trash`)
      await applied()
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  if (receipt) return (
    <Stack gap="sm">
      <Text size="sm" fw={500}>Choices saved</Text>
      {receipt.map((f, i) => <Text key={`${f.fileId}-${i}`} size="sm" style={{ overflowWrap: 'anywhere' }}>
        {fileName(plan.files.find((p) => p.fileId === f.fileId)?.path ?? '')}: {{ imported: 'Imported', kept: 'Kept archive', trashed: 'Moved to trash', trash_pending: 'Trash move pending', later: 'Left for later', settlement_pending: 'Finishing download pending' }[f.status] ?? f.status}{f.note && ` · ${f.note}`}
      </Text>)}
      <Text size="xs" c="dimmed">Pending trash moves will retry automatically. Archives already imported or retained keep their saved choices.</Text>
      <Button onClick={() => void applied()}>Done</Button>
    </Stack>
  )
  return (
    <Stack gap="sm">
      <Text size="sm">Choose which copies to use. Existing copies stay selected unless you explicitly choose a replacement.</Text>
      {plan.files.length > 1 && <Group gap="xs">
        <Button size="compact-xs" variant="default" disabled={busy} onClick={() => setChoices(plan.files.map((f) => ({
          fileId: f.fileId, disposition: f.reason ? 'later' : 'keep',
          use: f.books.filter((b) => !b.reason && b.incoming.factsKnown && !b.owners.some((o) => o.present)).map((b) => b.book),
        })))}>Import missing; keep existing</Button>
        <Button size="compact-xs" variant="default" disabled={busy} onClick={() => setChoices((v) => v.map((c, i) =>
          !c.use.length && !plan.files[i].trashReason ? { ...c, disposition: 'trash' } : c,
        ))}>Trash unused incoming files</Button>
      </Group>}
      {plan.files.slice((page - 1) * 25, page * 25).map((f, offset) => {
        const i = (page - 1) * 25 + offset
        return (
        <Card key={f.fileId} withBorder padding="sm" style={{ minWidth: 0 }}>
          <Stack gap="xs">
            <Text size="sm" fw={500} style={{ overflowWrap: 'anywhere' }}>{fileName(f.path)}</Text>
            <Text size="xs" c="dimmed">{[f.source, f.group].filter(Boolean).join(' · ') || 'Source/group unknown'}</Text>
            {f.reason && <Text size="xs" c="orange">{f.reason}</Text>}
            {f.books.map((b) => (
              <Stack key={key(b.book)} gap={4}>
                <Checkbox
                  label={b.owners.some((o) => o.present) ? `Use incoming instead for ${b.label}` : `Import ${b.label}`}
                  checked={using(i, b.book)}
                  disabled={busy || !!b.reason || !!f.reason || choices[i].disposition === 'later'}
                  onChange={(event) => update(i, {
                    use: event.currentTarget.checked ? [...choices[i].use, b.book] : choices[i].use.filter((v) => key(v) !== key(b.book)),
                    disposition: 'keep',
                  })}
                />
                <Score value={b.incoming} />
                {b.owners.map((o) => (
                  <div key={o.fileId}>
                    <Text size="xs" style={{ overflowWrap: 'anywhere' }}>
                      {using(i, b.book) ? 'Replace' : 'Keep existing'}: {fileName(o.path)}{!o.present && ' (missing)'}
                      {[o.source, o.group].filter(Boolean).length > 0 && ` · ${[o.source, o.group].filter(Boolean).join(' · ')}`}
                      {o.pinned && <Badge size="xs" ml={4}>Pinned</Badge>}
                    </Text>
                    <Score value={o.score} />
                  </div>
                ))}
                {b.requestedReplacement && <Text size="xs" c="blue">Replacement requested when this download was selected.</Text>}
                {b.reason && <Text size="xs" c="orange">{b.reason}</Text>}
              </Stack>
            ))}
            <Select
              label={`After review: ${fileName(f.path)}`}
              value={choices[i].disposition}
              disabled={busy}
              data={[
                { value: 'keep', label: choices[i].use.length ? 'Keep archive for selected books' : 'Keep unused file' },
                { value: 'trash', label: 'Move incoming file to trash', disabled: choices[i].use.length > 0 || !!f.trashReason },
                { value: 'later', label: 'Leave for later' },
              ]}
              onChange={(v) => v && update(i, { disposition: v, ...(v === 'later' ? { use: [] } : {}) })}
            />
            {f.trashReason && <Text size="xs" c="dimmed">{f.trashReason}</Text>}
            {f.books.length > 1 && <Text size="xs" c="dimmed">Choices select which books this archive supplies. The archive stays intact.</Text>}
          </Stack>
        </Card>
        )
      })}
      {plan.files.length > 25 && <Pagination total={Math.ceil(plan.files.length / 25)} value={page} onChange={setPage} size="sm" />}
      <Text size="sm" fw={500} role="status">Import {imports} · Keep existing {kept} · Replace {replacements} · Trash {trash}</Text>
      {trash > 0 && <Text size="xs" c="dimmed">Only the selected incoming archives move to recoverable trash for 30 days. Download-client payloads stay in place.</Text>}
      {error && <Alert color="red">{error}<Button size="xs" variant="subtle" onClick={() => void refresh()}>Refresh preview</Button></Alert>}
      <Button loading={busy} disabled={choices.every((c) => c.disposition === 'later')} onClick={() => void apply()}>Apply choices</Button>
    </Stack>
  )
}
