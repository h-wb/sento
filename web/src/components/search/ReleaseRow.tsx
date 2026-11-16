import {
  Anchor,
  Badge,
  Button,
  Group,
  SimpleGrid,
  Stack,
  Text,
  UnstyledButton,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { ReleaseCandidate } from '../../api'
import { fmtNum, fmtSize, plural } from '../../format'
import classes from '../CandidateList.module.css'
import { flagLabel, rangeLabel, reasonCode, reasonName, status } from './releaseModel'

export function ReleaseRow({
  c,
  queued,
  showBooks = false,
  allBooks = false,
  busy,
  loading,
  onGrab,
  onUnblock,
  unblockDisabled,
  selected,
  onToggle,
}: {
  c: ReleaseCandidate
  queued: boolean
  showBooks?: boolean
  allBooks?: boolean
  busy: boolean
  loading: boolean
  onGrab?: (c: ReleaseCandidate, force: boolean) => void
  onUnblock?: () => void
  unblockDisabled?: boolean
  selected?: boolean
  onToggle?: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const decision = c.decision
  const state = status(c)
  const volume = rangeLabel('Vol.', c.parse.volumes)
  const chapter = rangeLabel('Ch.', c.parse.chapters)
  // Edition-wide, what a release covers is the first thing to compare.
  const books = showBooks ? [volume, chapter].filter(Boolean).join(' · ') : ''
  const facts = [
    !books && volume,
    !books && chapter,
    c.parse.languages?.length
      ? c.parse.languages.map((l) => l.toUpperCase()).join('/')
      : c.parse.language?.toUpperCase() || 'Language unknown',
    c.parse.group || c.scanlator,
    c.parse.quality.digital && flagLabel('digital'),
    c.parse.quality.scan && flagLabel('scan'),
    c.parse.quality.web && flagLabel('web'),
    c.parse.edition.omnibus && flagLabel('omnibus'),
    c.parse.edition.deluxe && flagLabel('deluxe'),
    c.parse.edition.boxset && flagLabel('boxset'),
    c.parse.edition.colored && flagLabel('colored'),
    c.parse.extra && 'Bonus / special',
    c.parse.version && `Revision ${c.parse.version}`,
  ].filter(Boolean)
  const reason = decision?.reasons[0]?.message
  const more = (decision?.reasons.length ?? 1) - 1
  const matched = c.coverage?.length ?? 0
  return (
    <div className={classes.row} data-testid="release-row" data-status={queued ? 'queued' : state}>
      <Group gap="sm" align="flex-start" className={classes.rowMain}>
        <UnstyledButton
          className={classes.summary}
          aria-expanded={expanded}
          onClick={() => setExpanded(!expanded)}
        >
          <Group gap={6} wrap="nowrap" align="flex-start">
            {expanded ? (
              <IconChevronDown size={14} className={classes.chevron} />
            ) : (
              <IconChevronRight size={14} className={classes.chevron} />
            )}
            <div style={{ minWidth: 0, flex: 1 }}>
              <Group gap={6} wrap="nowrap" align="baseline">
                {books && (
                  <Badge variant="light" color="gray" className={classes.books}>
                    {books}
                  </Badge>
                )}
                <Text
                  fw={600}
                  size="sm"
                  lineClamp={expanded ? undefined : 1}
                  className="wrapAnywhere"
                >
                  {c.title}
                </Text>
              </Group>
              <Text size="xs" c="dimmed" className="wrapAnywhere">
                {[
                  c.site,
                  c.protocol === 'torrent' ? 'Torrent' : 'Direct download',
                  c.protocol === 'torrent' && (c.size ? fmtSize(c.size) : 'Size unknown'),
                  c.protocol === 'torrent' && `${c.seeders ?? 0} seeders`,
                  ...facts,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
              {state === 'blocked' ? (
                <Text size="xs" c="dimmed" className="wrapAnywhere">
                  {reason ?? 'Ruled out'}
                  {more > 0 && ` (+${more} more)`}
                </Text>
              ) : (
                <Text size="xs" c={state === 'review' ? 'orange' : 'dimmed'}>
                  {state === 'review' && `${reason}${more > 0 ? ` (+${more} more)` : ''} · `}
                  {matched > 0
                    ? `${matched}${allBooks ? '' : ' wanted'} ${plural(matched, 'book')}`
                    : decision?.kind || 'Unclassified'}
                  {state === 'ready' && ` · Rank score ${c.score}`}
                  {c.editionSnapshot && ` · ${c.editionSnapshot.editionName}`}
                  {c.versionSnapshot &&
                    !c.versionSnapshot.isDefault &&
                    ` · ${c.versionSnapshot.name}`}
                </Text>
              )}
            </div>
          </Group>
        </UnstyledButton>
        {queued ? (
          <Badge variant="light" color="blue">
            Queued
          </Badge>
        ) : onUnblock && c.rejections?.includes('rejected.blocklisted') ? (
          <Button
            size="xs"
            variant="default"
            disabled={busy || unblockDisabled}
            loading={loading}
            onClick={onUnblock}
          >
            Unblock
          </Button>
        ) : onToggle && state === 'ready' ? (
          // Inside a download plan a ready release joins the plan instead
          // of starting its own download.
          <Button
            size="xs"
            variant={selected ? 'light' : 'default'}
            disabled={busy}
            aria-pressed={selected ?? false}
            onClick={onToggle}
          >
            {selected ? 'In plan' : 'Add to plan'}
          </Button>
        ) : onGrab && state !== 'blocked' ? (
          <Button
            size="xs"
            variant={state === 'review' ? 'light' : 'filled'}
            color={state === 'review' ? 'orange' : 'brand'}
            disabled={busy || !(decision?.canDownload || decision?.canForce)}
            loading={loading}
            onClick={() => onGrab(c, state === 'review')}
          >
            {state === 'review' ? 'Review & download' : 'Download'}
          </Button>
        ) : (
          state === 'blocked' && (
            <Badge variant="light" color="gray" className={classes.reasonBadge}>
              {reasonName(reasonCode(c))}
            </Badge>
          )
        )}
      </Group>
      {expanded && (
        <Stack gap="sm" className={classes.details}>
          {!!decision?.reasons.length && (
            <Stack gap={4}>
              {decision.reasons.map((r) => (
                <Text key={r.code} size="sm" className="wrapAnywhere">
                  {r.message}{' '}
                  <Text span size="xs" c="dimmed">
                    {r.code === 'rejected.blocklisted'
                      ? 'Unblock this release to evaluate it again.'
                      : r.owner === 'book'
                        ? 'Not this book: nothing to change.'
                        : r.override
                          ? 'Can be overridden for this download.'
                          : 'Cannot be overridden.'}
                  </Text>
                  {r.owner === 'profile' && !!c.profileId && (
                    <>
                      {' '}
                      <Anchor component={Link} to={`/profiles?profile=${c.profileId}`} size="xs">
                        Open profile
                      </Anchor>
                    </>
                  )}
                </Text>
              ))}
            </Stack>
          )}
          <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="xs">
            <Text size="xs">
              Classification: {decision?.kind ?? 'Unknown'}
              {c.parse.complete ? ' · complete pack claim' : ''}
            </Text>
            <Text size="xs">Language evidence: {decision?.languageSource ?? 'unknown'}</Text>
            <Text size="xs">Number evidence: {decision?.numberSource ?? 'unknown'}</Text>
            <Text size="xs">Profile: {c.policySnapshot?.name ?? 'Built-in defaults'}</Text>
          </SimpleGrid>
          {!!c.coverage?.length && (
            <Text size="xs" className="wrapAnywhere">
              Will collect:{' '}
              {c.coverage
                .slice(0, 30)
                .map((s) => `${s.ledger === 'chapter' ? 'Ch.' : 'Vol.'} ${fmtNum(s.number)}`)
                .join(', ')}
              {c.coverage.length > 30 ? ` and ${c.coverage.length - 30} more` : ''}
            </Text>
          )}
          {!!decision?.warnings.length && (
            <Stack gap={3}>
              {decision.warnings.map((w) => (
                <Text size="xs" c="dimmed" key={w}>
                  {w}
                </Text>
              ))}
            </Stack>
          )}
          {!!c.scoreParts?.length && (
            <Stack gap={2}>
              <Text size="xs" fw={600}>
                Ranking
              </Text>
              {c.scoreParts.map((p, i) => (
                <Group justify="space-between" key={`${p.label}-${i}`}>
                  <Text size="xs">{p.label}</Text>
                  <Text size="xs">
                    {p.value > 0 ? '+' : ''}
                    {p.value}
                  </Text>
                </Group>
              ))}
            </Stack>
          )}
          {!!c.formats?.length && (
            <Text size="xs" className="wrapAnywhere">
              Matched formats:{' '}
              {c.formats.map((f) => `${f.name} (${f.score > 0 ? '+' : ''}${f.score})`).join(' · ')}
            </Text>
          )}
          {c.url && (
            <Anchor href={c.url} target="_blank" rel="noopener noreferrer" size="xs">
              Open original release
            </Anchor>
          )}
          {!!c.parseTrace?.length && (
            <details>
              <summary style={{ fontSize: 'var(--mantine-font-size-xs)', cursor: 'pointer' }}>
                Parsing details
              </summary>
              <Stack gap={3} mt="xs">
                {c.parseTrace.map((p, i) => (
                  <Text key={i} size="xs" className="wrapAnywhere">
                    {p.rule}
                    {p.text ? `: ${p.text}` : ''}
                  </Text>
                ))}
              </Stack>
            </details>
          )}
        </Stack>
      )}
    </div>
  )
}
