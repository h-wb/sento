import { useState } from 'react'
import type { MediaSettings } from '../../api'

export function useTemplateCaret(
  set: (patch: Partial<MediaSettings>) => void,
  setReaderField: (key: string, template: string) => void,
) {
  // The template last focused: a token picked from the table goes into it,
  // at the caret. Reader fields are keyed "reader:<field>".
  const [active, setActive] = useState<{ field: string; input: HTMLInputElement } | null>(null)
  const focused = (field: string) => (input: HTMLInputElement) => setActive({ field, input })
  const insertToken = (syntax: string) => {
    if (!active?.input.isConnected) return
    const { field, input } = active
    const start = input.selectionStart ?? input.value.length
    const end = input.selectionEnd ?? start
    const value = input.value.slice(0, start) + syntax + input.value.slice(end)
    if (field.startsWith('reader:')) setReaderField(field.slice('reader:'.length), value)
    else set({ [field]: value })
    requestAnimationFrame(() => {
      input.focus()
      input.setSelectionRange(start + syntax.length, start + syntax.length)
    })
  }
  return { focused, insertToken }
}
