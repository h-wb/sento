import { pathToFileURL } from 'node:url'

export function validTitle(title) {
  return /^(feat|fix|perf|refactor|docs|test|build|ci|chore|revert)(\([a-zA-Z0-9_./-]+\))?!?: \S[^\r\n]*$/.test(title)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  if (!validTitle(process.env.PR_TITLE ?? '')) {
    console.error('Use a Conventional Commit PR title, e.g. fix(import): preserve original files. See CONTRIBUTING.md.')
    process.exitCode = 1
  }
}
