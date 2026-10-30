import { Badge, Text, Tooltip, VisuallyHidden } from '@mantine/core'
import {
  IconAlertTriangle,
  IconDatabase,
  IconPencil,
  IconUser,
} from '@tabler/icons-react'
import type { Book, Edition } from '../types/store'

// Where a piece of the library's information comes from. Everything shown
// about an edition or a book is one of these, and each looks the same
// wherever it appears.
export type OriginKind = 'catalog' | 'local' | 'edited' | 'missing'

const origins: Record<
  OriginKind,
  { label: string; color: string; icon: typeof IconDatabase; tip: string }
> = {
  catalog: {
    label: 'MangaBaka',
    color: 'blue',
    icon: IconDatabase,
    tip: 'Listed in MangaBaka. A refresh updates it.',
  },
  local: {
    label: 'Added by you',
    color: 'gray',
    icon: IconUser,
    tip: 'Not from MangaBaka. A refresh never changes it.',
  },
  edited: {
    label: 'Edited',
    color: 'grape',
    icon: IconPencil,
    tip: 'You changed what MangaBaka lists. A refresh keeps your values.',
  },
  missing: {
    label: 'No longer in MangaBaka',
    color: 'orange',
    icon: IconAlertTriangle,
    tip: 'MangaBaka listed this and no longer does. Nothing was removed here.',
  },
}

// The mark for everything that comes from MangaBaka: the same two letters
// wherever its information appears, alone where space is tight.
export function MangaBakaMark({ size = 16 }: { size?: number }) {
  return (
    <span
      aria-hidden
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
        width: size * 1.35,
        height: size,
        borderRadius: 3,
        background: 'var(--mantine-color-blue-filled)',
        color: 'var(--mantine-color-white)',
        fontSize: size * 0.56,
        fontWeight: 800,
        letterSpacing: '-0.02em',
        lineHeight: 1,
        verticalAlign: 'text-bottom',
      }}
    >
      MB
    </span>
  )
}

// Origin marks where something comes from. compact shows the mark alone;
// href, for MangaBaka's, opens the page it comes from.
export function Origin({
  kind,
  label,
  tip,
  compact,
  href,
}: {
  kind: OriginKind
  label?: string
  tip?: string
  compact?: boolean
  href?: string
}) {
  const o = origins[kind]
  const text = label ?? o.label
  if (kind === 'catalog') {
    const mark = (
      <>
        <MangaBakaMark size={compact ? 14 : 16} />
        {compact ? (
          <VisuallyHidden>From MangaBaka</VisuallyHidden>
        ) : (
          <Text span size="xs" fw={600} c="dimmed">
            {text}
          </Text>
        )}
      </>
    )
    const style = { display: 'inline-flex', alignItems: 'center', gap: 5, flexShrink: 0 }
    return (
      <Tooltip
        label={`${tip ?? o.tip}${href ? ' Opens its MangaBaka page.' : ''}`}
        multiline
        maw={280}
        withArrow
      >
        {href ? (
          <a
            href={href}
            target="_blank"
            rel="noreferrer"
            style={{ ...style, textDecoration: 'none' }}
          >
            {mark}
          </a>
        ) : (
          <span style={{ ...style, cursor: 'help' }}>{mark}</span>
        )}
      </Tooltip>
    )
  }
  return (
    <Tooltip label={tip ?? o.tip} multiline maw={280} withArrow>
      <Badge
        size="xs"
        variant="light"
        color={o.color}
        leftSection={<o.icon size={10} aria-hidden />}
        style={{ cursor: 'help', flexShrink: 0 }}
      >
        {text}
      </Badge>
    </Tooltip>
  )
}

const fieldNames: Record<string, string> = {
  edition_name: 'name',
  language: 'language',
  publisher: 'publisher',
  medium: 'medium',
  sequence_num: 'number',
  sequence_string: 'label',
  count_type: 'category',
  release_date: 'date',
  isbn: 'ISBN',
  pages: 'pages',
  number_known: 'numbering',
  revision: 'revision',
}

const editedTip = (fields: string[]) =>
  `You set the ${[...new Set(fields.map((f) => fieldNames[f] ?? f))].join(', ')}. A refresh keeps your values.`

// The marker of an edition: one mark for where it comes from (the MB mark,
// or "Local edition"), plus "Edited" when you changed what MangaBaka says.
export function EditionOrigin({ edition: c, href }: { edition: Edition; href?: string }) {
  return (
    <>
      {c.syncState === 'stale' ? (
        <Origin kind="missing" label="Catalog entry missing" />
      ) : c.custom ? (
        <Origin
          kind="local"
          label="Local edition"
          tip={
            c.numberingSource === 'series_count'
              ? 'Not from MangaBaka, but numbered from it: 1 to the chapter count it gives for the series. A refresh never changes it.'
              : undefined
          }
        />
      ) : (
        <Origin kind="catalog" href={href} compact />
      )}
      {!c.custom && c.overrides.length > 0 && <Origin kind="edited" tip={editedTip(c.overrides)} />}
    </>
  )
}

// The marker of a book, shown only where it differs from its edition: in an
// edition MangaBaka lists, the books it does not list or that were edited.
export function BookOrigin({ book, edition }: { book: Book; edition: Edition }) {
  if (edition.custom) return null
  if (book.syncState === 'stale') return <Origin kind="missing" />
  if (book.copies === 0 && book.syncState !== 'retired') return <Origin kind="local" />
  if (book.overrides.length > 0) return <Origin kind="edited" tip={editedTip(book.overrides)} />
  return null
}
