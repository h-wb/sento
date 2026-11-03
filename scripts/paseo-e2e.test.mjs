import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

test('Paseo browser task forwards filters and both library directories to the standard task', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'sento-paseo-browser-'))
  try {
    const cache = path.join(dir, 'browser deps')
    for (const file of ['root/usr/lib/x86_64-linux-gnu/libglib-2.0.so.0', 'root/lib/x86_64-linux-gnu/libdbus-1.so.3', 'fonts.conf']) {
      const target = path.join(cache, file)
      mkdirSync(path.dirname(target), { recursive: true })
      writeFileSync(target, '')
    }
    const bin = path.join(dir, 'bin')
    mkdirSync(bin)
    writeFileSync(path.join(bin, 'mise'), '#!/bin/sh\nprintf "%s\\n" "$LD_LIBRARY_PATH" "$FONTCONFIG_FILE" "$@"\n', { mode: 0o755 })
    const result = spawnSync('bash', ['scripts/paseo-e2e.sh', 'activity.spec.ts', '--grep', 'a test with spaces'], {
      encoding: 'utf8',
      env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, SENTO_PASEO_BROWSER_DEPS: cache, LD_LIBRARY_PATH: '/existing-libs' },
    })
    assert.equal(result.status, 0, result.stderr)
    assert.deepEqual(result.stdout.trim().split('\n'), [
      `${cache}/root/usr/lib/x86_64-linux-gnu:${cache}/root/lib/x86_64-linux-gnu:/existing-libs`,
      `${cache}/fonts.conf`, '-C', process.cwd(), 'run', 'e2e', '--', 'activity.spec.ts', '--grep', 'a test with spaces',
    ])
  } finally { rmSync(dir, { recursive: true, force: true }) }
})

test('Paseo browser task explains a missing dependency cache before launching tests', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'sento-paseo-browser-missing-'))
  try {
    const result = spawnSync('bash', ['scripts/paseo-e2e.sh'], {
      encoding: 'utf8',
      env: { ...process.env, SENTO_PASEO_BROWSER_DEPS: dir },
    })
    assert.equal(result.status, 1)
    assert.match(result.stderr, /browser dependency cache is missing/)
    assert.match(result.stderr, /docs\/testing\/paseo.md/)
  } finally { rmSync(dir, { recursive: true, force: true }) }
})
