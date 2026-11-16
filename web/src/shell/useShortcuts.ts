import { spotlight } from '@mantine/spotlight'
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { allLinks } from './navigation'

export function useShortcuts(openHelp: () => void, openAdd: () => void) {
  const navigate = useNavigate()
  // Global keyboard shortcuts: "/" opens search, "?" shows help, and a
  // Gmail-style "g <key>" jumps between pages. Ignored while typing.
  useEffect(() => {
    let gAt = 0
    const typing = (el: EventTarget | null) =>
      el instanceof HTMLElement &&
      (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable)
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target)) return
      if (e.key === '/') {
        e.preventDefault()
        spotlight.open()
      } else if (e.key === '?') {
        e.preventDefault()
        openHelp()
      } else if (e.key === 'g') {
        gAt = e.timeStamp
      } else if (gAt > 0 && e.timeStamp - gAt < 1000) {
        if (e.key === 'n') {
          e.preventDefault()
          openAdd()
          gAt = 0
        } else {
          const link = allLinks.find((l) => l.key === e.key)
          if (link) {
            e.preventDefault()
            navigate(link.to)
            gAt = 0
          }
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, openHelp, openAdd])
}
