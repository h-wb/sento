import type { Condition } from '../../api'

// condText renders a condition compactly: "title ~ \bLQ\b", "NOT flag:raw".
export function condText(c: Condition): string {
  const body =
    c.type === 'flag' || c.type === 'protocol' ? `${c.type}:${c.value}` : `${c.type} ~ ${c.value}`
  return `${c.negate ? 'NOT ' : ''}${body}${c.required ? ' (required)' : ''}`
}

export function downloadJSON(name: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.json`
  a.click()
  URL.revokeObjectURL(url)
}
