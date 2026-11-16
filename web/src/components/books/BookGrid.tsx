import type { MouseEvent } from 'react'
import type { BookStatus } from '../../types/service'
import { labels, tone } from './bookStatus'
import classes from '../EditionBooks.module.css'

export function BookGrid({ shown, selected, flash, noun, open }: {
  shown: BookStatus[]
  selected: Set<number>
  flash: number | null
  noun: string
  open: (e: MouseEvent, s: BookStatus) => void
}) {
  const title = (s: BookStatus) =>
    `${noun[0].toUpperCase()}${noun.slice(1)} ${s.book.sequenceStr} · ${labels[s.state] ?? s.state}${s.reason ? ` · ${s.reason}` : ''}`

  return (
        <div className={classes.grid}>
          {shown.map((s) => (
            <button
              key={s.book.id}
              type="button"
              className={classes.cell}
              data-book={s.book.id}
              data-tone={tone(s)}
              data-selected={selected.has(s.book.id)}
              data-flash={flash === s.book.id}
              title={title(s)}
              onClick={(e) => open(e, s)}
            >
              {s.book.sequenceStr}
            </button>
          ))}
        </div>
  )
}
