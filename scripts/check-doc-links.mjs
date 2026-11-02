// Dependency-free check of local file links in the documentation.
import { readdirSync, readFileSync, existsSync } from 'node:fs'
import path from 'node:path'

function markdown(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const file = path.join(dir, entry.name)
    return entry.isDirectory() ? markdown(file) : file.endsWith('.md') ? [file] : []
  })
}

let failures = 0
const files = [
  'README.md', 'CONTRIBUTING.md', 'SECURITY.md', 'AGENTS.md', 'CLAUDE.md',
  'extensions/README.md', 'extensions/NOTICE.md',
  ...markdown('docs'), ...markdown('.vibe'),
]
for (const file of files) {
  for (const match of readFileSync(file, 'utf8').matchAll(/!?\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)/g)) {
    const target = match[1].replace(/^<|>$/g, '')
    if (/^(?:[a-z]+:|#|\/\/)/i.test(target)) continue
    const local = decodeURIComponent(target.split('#')[0])
    if (local && !existsSync(path.resolve(path.dirname(file), local))) {
      console.error(`${file}: missing link target ${target}`)
      failures++
    }
  }
}
if (failures) process.exitCode = 1
else console.log(`Checked local file links in ${files.length} Markdown files.`)
