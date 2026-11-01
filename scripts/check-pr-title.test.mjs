import { test } from 'node:test'
import assert from 'node:assert/strict'
import { validTitle } from './check-pr-title.mjs'

test('accepts supported types, optional scopes and breaking changes', () => {
  for (const title of ['fix: handle missing files', 'feat(api)!: replace queue response',
    'docs: explain setup', 'chore(deps): update Go', 'revert: undo import change']) {
    assert.equal(validTitle(title), true, title)
  }
})

test('rejects unclassified, empty and multiline titles', () => {
  for (const title of ['', 'Update README', 'feature: add profiles', 'fix:',
    'fix: ', 'fix:  ', 'fix(): broken', 'fix(api):missing space', 'fix: ok\nrun this']) {
    assert.equal(validTitle(title), false, JSON.stringify(title))
  }
})
