import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync, chmodSync, statSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { tmpdir } from 'node:os'
import path from 'node:path'

const script = path.resolve('scripts/worktree-setup.sh')

function fixture(t, ignored = true) {
  const root = mkdtempSync(path.join(tmpdir(), 'sento-worktree-setup-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const source = path.join(root, 'source')
  const target = path.join(root, 'target')
  const bin = path.join(root, 'bin')
  for (const dir of [source, target, bin]) mkdirSync(dir)
  const git = spawnSync('git', ['init', '-q', target], { encoding: 'utf8' })
  assert.equal(git.status, 0, git.stderr)
  if (ignored) writeFileSync(path.join(target, '.gitignore'), 'mise.local.toml\nfnox.toml\n')
  writeFileSync(path.join(source, 'mise.local.toml'), '[env]\nTEST_CLIENT="local"\n')
  writeFileSync(path.join(source, 'fnox.toml'), '# fixture reference map\n')
  chmodSync(path.join(source, 'fnox.toml'), 0o600)
  const log = path.join(root, 'calls')
  writeFileSync(path.join(bin, 'mise'), '#!/bin/sh\nprintf "%s\\n" "$*" >> "$SETUP_LOG"\n', { mode: 0o755 })
  const run = (withSource = true) => spawnSync('bash', [script], {
    cwd: target, encoding: 'utf8',
    env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, SETUP_LOG: log,
      PASEO_SOURCE_CHECKOUT_PATH: withSource ? source : '' },
  })
  return { source, target, log, run }
}

test('fresh worktrees inherit ignored private references with their permissions', t => {
  const { source, target, log, run } = fixture(t)
  const result = run()
  assert.equal(result.status, 0, result.stderr)
  for (const file of ['mise.local.toml', 'fnox.toml']) {
    assert.equal(readFileSync(path.join(target, file), 'utf8'), readFileSync(path.join(source, file), 'utf8'))
  }
  assert.equal(statSync(path.join(target, 'fnox.toml')).mode & 0o777, 0o600)
  assert.equal(readFileSync(log, 'utf8'), 'install\nrun web:install\n')
})

test('existing worktree overrides are preserved', t => {
  const { target, run } = fixture(t)
  writeFileSync(path.join(target, 'mise.local.toml'), '# existing local configuration\n')
  const result = run()
  assert.equal(result.status, 0, result.stderr)
  assert.equal(readFileSync(path.join(target, 'mise.local.toml'), 'utf8'), '# existing local configuration\n')
})

test('public setup works without a source and never copies files that are not ignored', t => {
  const { target, log, run } = fixture(t, false)
  for (const source of [true, false]) {
    const result = run(source)
    assert.equal(result.status, 0, result.stderr)
    assert.equal(existsSync(path.join(target, 'mise.local.toml')), false)
    assert.equal(existsSync(path.join(target, 'fnox.toml')), false)
  }
  assert.equal(readFileSync(log, 'utf8'), 'install\nrun web:install\ninstall\nrun web:install\n')
})
