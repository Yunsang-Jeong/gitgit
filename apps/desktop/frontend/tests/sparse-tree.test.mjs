import assert from 'node:assert/strict'
import test from 'node:test'

import { findSparseNode, flattenSparseNodes, selectedSparsePaths, setSparseNodeChildren, toSparseNodes, toggleSparseExpansion, toggleSparseNode } from '../src/lib/sparse-tree.ts'

function entry(name, path, objectType = 'tree') {
  return { name, path, object_type: objectType, oid: 'a'.repeat(40) }
}

test('only directories become nodes', () => {
  const nodes = toSparseNodes([
    entry('internal', 'internal'),
    entry('README.md', 'README.md', 'blob'),
    entry('vendor', 'vendor', 'commit'),
    entry('docs', 'docs'),
  ])
  assert.deepEqual(nodes.map((node) => node.path), ['internal', 'docs'])
  assert.equal(nodes[0].children, null)
  assert.equal(nodes[0].expanded, false)
})

// Cone mode includes everything under a selected directory, so a child of a
// checked ancestor is already included even though it was never clicked.
test('a directory under a checked ancestor loads as checked', () => {
  const nodes = toSparseNodes([entry('guide', 'docs/guide'), entry('api', 'other/api')], ['docs'])
  assert.equal(nodes[0].checked, true)
  assert.equal(nodes[1].checked, false)
})

test('flattening yields only expanded levels, with depth', () => {
  let nodes = toSparseNodes([entry('docs', 'docs')])
  nodes = setSparseNodeChildren(nodes, 'docs', toSparseNodes([entry('guide', 'docs/guide')]))
  assert.deepEqual(flattenSparseNodes(nodes).map((visible) => visible.node.path), ['docs'])

  nodes = toggleSparseExpansion(nodes, 'docs', true)
  assert.deepEqual(
    flattenSparseNodes(nodes).map((visible) => [visible.node.path, visible.depth]),
    [['docs', 0], ['docs/guide', 1]],
  )
})

test('checking a directory cascades into the levels already loaded', () => {
  let nodes = toSparseNodes([entry('docs', 'docs'), entry('internal', 'internal')])
  nodes = setSparseNodeChildren(nodes, 'docs', toSparseNodes([entry('guide', 'docs/guide')]))
  nodes = toggleSparseNode(nodes, 'docs', true)

  assert.equal(findSparseNode(nodes, 'docs').checked, true)
  assert.equal(findSparseNode(nodes, 'docs/guide').checked, true)
  assert.equal(findSparseNode(nodes, 'internal').checked, false)

  nodes = toggleSparseNode(nodes, 'docs', false)
  assert.equal(findSparseNode(nodes, 'docs/guide').checked, false)
})

// A shared prefix is not a parent: toggling "docs" must not touch "docs-old".
test('cascading matches path segments, not string prefixes', () => {
  let nodes = toSparseNodes([entry('docs', 'docs'), entry('docs-old', 'docs-old')])
  nodes = toggleSparseNode(nodes, 'docs', true)
  assert.equal(findSparseNode(nodes, 'docs').checked, true)
  assert.equal(findSparseNode(nodes, 'docs-old').checked, false)
})

test('a level loaded under a checked directory inherits its state', () => {
  let nodes = toSparseNodes([entry('docs', 'docs')])
  nodes = toggleSparseNode(nodes, 'docs', true)
  nodes = setSparseNodeChildren(nodes, 'docs', toSparseNodes([entry('guide', 'docs/guide')]))
  assert.equal(findSparseNode(nodes, 'docs/guide').checked, true)
})

test('only the topmost checked directory is reported', () => {
  let nodes = toSparseNodes([entry('docs', 'docs'), entry('internal', 'internal')])
  nodes = setSparseNodeChildren(nodes, 'docs', toSparseNodes([entry('guide', 'docs/guide')]))
  nodes = toggleSparseNode(nodes, 'docs', true)
  assert.deepEqual(selectedSparsePaths(nodes), ['docs'])

  nodes = toggleSparseNode(nodes, 'docs', false)
  nodes = toggleSparseNode(nodes, 'docs/guide', true)
  assert.deepEqual(selectedSparsePaths(nodes), ['docs/guide'])

  assert.deepEqual(selectedSparsePaths(toSparseNodes([entry('a', 'a')])), [])
})

test('an unexplored node is left alone by findSparseNode', () => {
  const nodes = toSparseNodes([entry('docs', 'docs')])
  assert.equal(findSparseNode(nodes, 'docs/guide'), null)
  assert.equal(findSparseNode(nodes, 'docs').path, 'docs')
})
