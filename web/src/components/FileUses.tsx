import { Alert, Button, Checkbox, Group, Radio, Stack, Text } from '@mantine/core'
import type { FileScore, ReviewBookPlan, ReviewPlan, ReviewPlanRequest } from '../types/service'
import { fileName } from './editionText'
import { QueryFeedback } from './QueryFeedback'
import { ReviewReceipt } from './ReviewChoices'
import { bookKey, shortBook, useReviewChoices, useReviewPlan } from './useReviewChoices'
import classes from './FileDialog.module.css'

type Props = {
  seriesID: number
  // One file, with the books it holds.
  request: ReviewPlanRequest
  // A matched file changes hands only where a person says so.
  keepCurrent?: boolean
  applied: () => Promise<void>
  cancel: () => void
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
}

const formats = (s: FileScore) =>
  s.formats.map((f) => `${f.name}: ${f.score > 0 ? '+' : ''}${f.score}`).join(' · ') ||
  'No custom formats matched.'

// Which editions use one file, a line per edition and book it holds, and
// what happens to the file when none does.
export function FileUses(props: Props) {
  const preview = useReviewPlan(props.seriesID, props.request, true)
  if (!preview.data)
    return (
      <Stack gap="sm">
        {preview.isPending && (
          <Text size="sm" c="dimmed">
            Checking the file…
          </Text>
        )}
        <QueryFeedback
          pending={preview.isPending}
          error={preview.error}
          onRetry={() => void preview.refetch()}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={props.cancel}>
            Cancel
          </Button>
        </Group>
      </Stack>
    )
  return (
    <Uses
      key={preview.data.token}
      {...props}
      plan={preview.data}
      // Still the answer to the edit before this one.
      stale={preview.isPlaceholderData}
      refresh={() => preview.refetch()}
    />
  )
}

function Uses({
  seriesID,
  request,
  keepCurrent,
  applied,
  cancel,
  onDirtyChange,
  onBusyChange,
  plan,
  stale,
  refresh,
}: Props & { plan: ReviewPlan; stale: boolean; refresh: () => Promise<unknown> }) {
  const { choices, busy, error, receipt, using, update, selectBook, nothingToApply, apply } =
    useReviewChoices({ seriesID, request, plan, keepCurrent, applied, onDirtyChange, onBusyChange })
  if (receipt) return <ReviewReceipt plan={plan} receipt={receipt} done={() => void applied()} />
  const file = plan.files[0]
  const choice = choices[0]
  const locked = busy || stale

  // What the edition does with the book, in the words of the outcome.
  const state = (b: ReviewBookPlan) => {
    const other = b.owners.some((o) => o.present)
    if (using(0, b.book))
      return b.currentSelected
        ? 'Uses this file'
        : other
          ? 'Will use this file instead of its other copy'
          : 'Will use this file'
    if (b.currentSelected)
      return other ? 'Will use its other copy instead' : 'Will stop using this file'
    return other ? 'Uses its other copy' : 'Has no file for this book'
  }
  const trash = choice.disposition === 'trash'
  // A file an edition uses is first saved as unused, then trashed.
  const canTrash = !file.trashReason || file.trashWhenUnused
  return (
    <Stack gap="sm">
      {file.reason && (
        <Text size="xs" c="orange">
          {file.reason}
        </Text>
      )}
      {file.books.length === 0 ? (
        <Text size="sm" c="dimmed">
          Pick the books it holds first: an edition uses a file for its books.
        </Text>
      ) : (
        <div className={classes.uses}>
          {file.books.map((b) => {
            const other = b.owners.find((o) => o.present) ?? b.owners[0]
            const refused = !!b.reason && !b.currentSelected
            return (
              <div key={bookKey(b.book)} className={classes.use}>
                <Checkbox
                  label={`${shortBook(b.label)} · ${b.card}`}
                  description={state(b)}
                  checked={using(0, b.book)}
                  disabled={locked || refused || !!file.reason}
                  onChange={(e) => selectBook(0, b, e.currentTarget.checked)}
                />
                <div className={classes.useDetail}>
                  {b.reason && (
                    <Text size="xs" c="orange">
                      {b.reason}
                      {b.currentSelected &&
                        using(0, b.book) &&
                        '. The edition keeps using it until you untick it.'}
                    </Text>
                  )}
                  {b.requestedReplacement && (
                    <Text size="xs" c="dimmed">
                      You asked for a replacement when you picked this download.
                    </Text>
                  )}
                  <details className="more">
                    <summary>
                      Score {b.incoming.score} · {b.incoming.profileName || 'No profile'}
                      {other && ` · the other copy ${other.score.score}`}
                    </summary>
                    <Text size="xs" c="dimmed">
                      {formats(b.incoming)}
                    </Text>
                    {!b.incoming.factsKnown && (
                      <Text size="xs" c="orange">
                        What this file is could not be read; check it before using it.
                      </Text>
                    )}
                    {b.owners.map((o) => (
                      <Text key={o.fileId} size="xs" c="dimmed" style={{ overflowWrap: 'anywhere' }}>
                        The other copy, {fileName(o.path)}
                        {!o.present && ' (missing)'}
                        {o.pinned && ' (pinned)'}, scores {o.score.score}. {formats(o.score)}
                      </Text>
                    ))}
                  </details>
                </div>
              </div>
            )
          })}
        </div>
      )}
      {choice.use.length === 0 && (
        <Radio.Group
          label="No edition will use this file"
          value={choice.disposition === 'later' ? '' : choice.disposition}
          onChange={(v) => update(0, { disposition: v })}
        >
          <Stack gap={8} mt={8}>
            <Radio value="keep" disabled={locked} label="Keep it on disk" />
            <Radio
              value="trash"
              disabled={locked || !canTrash}
              label="Move it to trash"
              description={canTrash ? 'Recoverable for 30 days.' : file.trashReason}
            />
          </Stack>
        </Radio.Group>
      )}
      {error && (
        <Alert color="red">
          {error}
          <Button size="xs" variant="subtle" onClick={() => void refresh()}>
            Check again
          </Button>
        </Alert>
      )}
      <Group justify="flex-end" gap="sm" mt={4}>
        <Button variant="default" disabled={busy} onClick={cancel}>
          Cancel
        </Button>
        <Button
          loading={busy}
          disabled={nothingToApply || stale}
          color={trash ? 'red' : undefined}
          onClick={() =>
            void apply(trash ? 'File moved to trash' : 'File saved', trash && file.trashWhenUnused)
          }
        >
          {trash ? 'Move to trash' : 'Save'}
        </Button>
      </Group>
    </Stack>
  )
}
