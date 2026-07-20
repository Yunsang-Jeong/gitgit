import assert from 'node:assert/strict'
import test from 'node:test'
import { addedAndDeletedLines, buildChangedFileTree, changedDirectoryPaths, changedFilesSignature, changedFileStatusMap } from '../src/lib/changed-files.ts'

test('changed files form a directory-first tree without losing file metadata', () => {
  const tree = buildChangedFileTree([
    { status: 'M', path: 'README.md' },
    { status: 'A', path: 'internal/git/history.go' },
    { status: 'R', old_path: 'internal/git/old.go', path: 'internal/git/new.go' },
    { status: 'M', path: 'desktop/app.go' },
  ])

  assert.deepEqual(tree.map((node) => [node.kind, node.name]), [
    ['directory', 'desktop'],
    ['directory', 'internal'],
    ['file', 'README.md'],
  ])
  const internal = tree[1]
  assert.equal(internal.children[0].path, 'internal/git')
  assert.deepEqual(internal.children[0].children.map((node) => node.name), ['history.go', 'new.go'])
  assert.equal(internal.children[0].children[1].file.old_path, 'internal/git/old.go')
})

test('diff preview keeps only real added and deleted content lines', () => {
  const lines = addedAndDeletedLines([
    'diff --git a/app.go b/app.go',
    'index 123..456 100644',
    '--- a/app.go',
    '+++ b/app.go',
    '@@ -1,3 +1,3 @@',
    ' context',
    '-old value',
    '+new value',
    '',
  ].join('\n'))

  assert.deepEqual(lines, [
    { kind: 'deletion', text: '-old value' },
    { kind: 'addition', text: '+new value' },
  ])
})

test('empty and metadata-only diffs do not produce preview lines', () => {
  assert.deepEqual(addedAndDeletedLines(''), [])
  assert.deepEqual(addedAndDeletedLines('diff --git a/a b/a\n--- a/a\n+++ b/a'), [])
})

test('repository tree metadata highlights changed paths and expands ancestors only', () => {
  const files = [
    { status: 'M', path: 'desktop/frontend/src/App.svelte' },
    { status: 'R', old_path: 'internal/git/old.go', path: 'internal/git/new.go' },
    { status: 'A', path: 'README.md' },
  ]

  assert.deepEqual(changedFileStatusMap(files), {
    'desktop/frontend/src/App.svelte': 'M',
    'internal/git/new.go': 'R',
    'internal/git/old.go': 'R',
    'README.md': 'A',
  })
  assert.deepEqual([...changedDirectoryPaths(files)].sort(), [
    'desktop',
    'desktop/frontend',
    'desktop/frontend/src',
    'internal',
    'internal/git',
  ])
  assert.equal(changedFilesSignature(files), 'A:README.md|M:desktop/frontend/src/App.svelte|R:internal/git/new.go|R:internal/git/old.go')
})
