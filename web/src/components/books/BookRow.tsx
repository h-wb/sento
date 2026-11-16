import { ActionIcon, Badge, Button, Checkbox, Menu, Text, Tooltip } from '@mantine/core'
import { IconBookmark, IconBookmarkFilled, IconDots } from '@tabler/icons-react'
import type { Dispatch,MouseEvent,SetStateAction } from 'react'
import { api } from '../../api'
import type { BookStatus } from '../../types/service'
import type { AssetCoverage,Book,CollectedVersion,Edition,File as SeriesFile } from '../../types/store'
import classes from '../EditionBooks.module.css'
import { BookOrigin } from '../Origin'
import { badgeColor, labels, tone } from './bookStatus'

export function BookRow({ s, coverage, t, c, selected, flash, noun, pick, open, explained, busy, run, seriesID, onEdit, setSplit, files, onFile }: {
  s: BookStatus
  coverage: AssetCoverage[]
  t: CollectedVersion
  c: Edition
  selected: Set<number>
  flash: number | null
  noun: string
  pick: (s: BookStatus, shift: boolean) => void
  open: (e: MouseEvent, s: BookStatus) => void
  explained?: string
  busy: boolean
  run: (fn: () => Promise<unknown>) => Promise<void>
  seriesID: number
  onEdit: (w: Book) => void
  setSplit: Dispatch<SetStateAction<{ book: Book; count: string | number } | null>>
  files: SeriesFile[]
  onFile: (f: SeriesFile) => void
}) {
            const onDisk = s.onDiskFileIds ?? s.fileIds
            const pins = s.fileIds
              .map((fid) =>
                coverage.find(
                  (a) => a.fileId === fid && a.versionId === t.id && a.bookId === s.book.id,
                ),
              )
              .filter((a): a is AssetCoverage => !!a)
  return (
              <div
                key={s.book.id}
                className={classes.row}
                data-book={s.book.id}
                data-selected={selected.has(s.book.id)}
                data-flash={flash === s.book.id}
              >
                <Checkbox
                  size="xs"
                  aria-label={`Select ${noun} ${s.book.sequenceStr}`}
                  checked={selected.has(s.book.id)}
                  // Clicks drive the change, so Shift extends a range here too.
                  onChange={(e) => pick(s, (e.nativeEvent as globalThis.MouseEvent).shiftKey === true)}
                />
                <Button size="compact-sm" variant="subtle" miw={44} onClick={(e) => open(e, s)}>
                  {s.book.sequenceStr}
                </Button>
                <Text size="xs" c="dimmed" className={classes.rowDetail}>
                  {s.book.countType !== 'main' &&
                    (s.book.countType === 'extra' ? 'Bonus / special · ' : 'Other · ')}
                  {(s.reason !== explained && s.reason) || s.book.releaseDate || 'Publication date unknown'}
                  {c.collections.length > 1 &&
                    s.book.copies > 0 &&
                    s.book.copies < c.collections.length &&
                    ` · in ${s.book.copies} of ${c.collections.length} media`}
                </Text>
                <BookOrigin book={s.book} edition={c} />
                <Tooltip label={s.reason} disabled={!s.reason || s.reason !== explained} multiline maw={320}>
                  <Badge size="xs" variant="light" color={badgeColor[tone(s)]}>
                    {labels[s.state] ?? s.state}
                  </Badge>
                </Tooltip>
                <Tooltip label={s.monitored ? 'Collected · click to pause' : 'Paused · click to collect'} openDelay={300}>
                  <ActionIcon
                    size="sm"
                    variant="subtle"
                    color={s.monitored ? undefined : 'gray'}
                    aria-label={`Collect ${c.type} ${s.book.sequenceStr}`}
                    aria-pressed={s.monitored}
                    disabled={busy}
                    onClick={() => void run(() => api.bookMonitor(seriesID, t.id, s.book.id, !s.monitored))}
                  >
                    {s.monitored ? <IconBookmarkFilled size={14} /> : <IconBookmark size={14} />}
                  </ActionIcon>
                </Tooltip>
                <Menu position="bottom-end" withinPortal>
                  <Menu.Target>
                    <ActionIcon
                      size="sm"
                      variant="subtle"
                      color="gray"
                      aria-label={`Actions for ${noun} ${s.book.sequenceStr}`}
                    >
                      <IconDots size={14} />
                    </ActionIcon>
                  </Menu.Target>
                  <Menu.Dropdown>
                    <Menu.Item onClick={() => onEdit(s.book)}>Edit</Menu.Item>
                    {c.type === 'volume' && (
                      <Menu.Item onClick={() => setSplit({ book: s.book, count: 2 })}>
                        This is a box of several volumes…
                      </Menu.Item>
                    )}
                    {onDisk.length > 0 && (
                      <Menu.Item
                        onClick={() => {
                          const f = files.find((f) => f.id === onDisk[0])
                          if (f) onFile(f)
                        }}
                      >
                        Edit file
                      </Menu.Item>
                    )}
                    {pins.map((a) => (
                      <Menu.Item
                        key={a.fileId}
                        disabled={busy}
                        onClick={() =>
                          void run(() => api.pinCoverage(seriesID, { ...a, pinned: !a.pinned }))
                        }
                      >
                        {a.pinned ? 'Unpin file' : 'Pin file'}
                      </Menu.Item>
                    ))}
                  </Menu.Dropdown>
                </Menu>
              </div>
  )
}
