import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

// Fake only external git/GitHub commands: exercise the actual publication
// gate without a token, network requests or changing the checkout's tags.
function check(tag, { onMain = true, passed = true } = {}) {
  const dir = mkdtempSync(path.join(tmpdir(), 'sento-release-check-'))
  try {
    writeFileSync(path.join(dir, 'git'), `#!/bin/sh
case "$1" in
  rev-parse) echo abc123 ;;
  fetch) exit 0 ;;
  merge-base) exit ${onMain ? 0 : 1} ;;
  *) exit 99 ;;
esac
`, { mode: 0o755 })
    writeFileSync(path.join(dir, 'gh'), `#!/bin/sh\necho ${passed}\n`, { mode: 0o755 })
    return spawnSync('bash', ['scripts/check-release.sh'], {
      encoding: 'utf8',
      env: { ...process.env, PATH: `${dir}:${process.env.PATH}`,
        SENTO_RELEASE_TAG: tag, GITHUB_REPOSITORY: 'example/sento' },
    })
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

test('release gate rejects malformed and prerelease tags', () => {
  for (const tag of ['v1', '1.2.3', 'v1.2.3-rc.1', 'v01.2.3', 'v1.2.3;echo bad']) {
    const result = check(tag)
    assert.notEqual(result.status, 0, tag)
    assert.match(result.stderr, /Expected a stable/)
  }
})

test('release gate rejects commits outside main or without passing CI', () => {
  const branch = check('v0.1.0', { onMain: false })
  assert.notEqual(branch.status, 0)
  assert.match(branch.stderr, /on main/)
  const ci = check('v0.1.0', { passed: false })
  assert.notEqual(ci.status, 0)
  assert.match(ci.stderr, /No successful main CI/)
})

test('release gate accepts a passing main commit and stable tag', () => {
  const result = check('v0.1.0')
  assert.equal(result.status, 0, result.stderr)
})
