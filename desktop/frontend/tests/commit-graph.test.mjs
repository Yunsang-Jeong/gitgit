import assert from 'node:assert/strict'
import test from 'node:test'

import { buildCommitGraph, maxVisibleGraphLanes, projectVisibleCommits } from '../src/lib/commit-graph.ts'
import { buildCommitGraphDrawing, commitGraphRowHeight, commitGraphWidth } from '../src/lib/commit-graph-render.ts'

function commit(commit, parents = [], refs = [], message = commit) {
  return {
    author: { name: 'GitGit Test', email: 'gitgit@example.com' },
    commit,
    short_commit: commit,
    parents,
    message,
    date: '2026-07-22T00:00:00Z',
    refs,
    files: [],
  }
}

test('pull request commits keep their historical branch after the ref is deleted', () => {
  const layout = buildCommitGraph([
    commit('merge', ['main-work', 'topic-head'], ['main'], 'Merge pull request #48965 from hashicorp/aws_s3_buckets'),
    commit('main-work', ['base']),
    commit('topic-head', ['target']),
    commit('target', ['base'], [], 'Added max_buckets validator and test'),
    commit('base'),
  ], 'main')

  assert.equal(layout.historicalBranches.get('topic-head'), 'aws_s3_buckets')
  assert.equal(layout.historicalBranches.get('target'), 'aws_s3_buckets')
  assert.equal(layout.historicalBranches.has('base'), false)
  assert.equal(layout.historicalBranches.has('merge'), false)
})

test('GitLab-style merge messages expose the source branch', () => {
  const layout = buildCommitGraph([
    commit('merge', ['main-work', 'topic'], ['main'], "Merge branch 'feature/search' into 'main'\n\nSee merge request platform/GitGit!73"),
    commit('main-work', ['base']),
    commit('topic', ['base']),
    commit('base'),
  ], 'main')

  assert.equal(layout.historicalBranches.get('topic'), 'feature/search')
})

test('default branch stays left while a completed merge lane collapses into it', () => {
  const layout = buildCommitGraph([
    commit('merge', ['main-work', 'topic'], ['main']),
    commit('main-work', ['base']),
    commit('topic', ['base']),
    commit('base'),
  ], 'main')

  assert.equal(layout.rows.get('merge').lane, 0)
  assert.deepEqual(layout.rows.get('merge').outgoing, [{ from: 0, to: 0 }, { from: 0, to: 1 }])
  assert.deepEqual(layout.rows.get('topic').outgoing, [{ from: 1, to: 0 }])
  assert.equal(layout.rows.get('base').lane, 0)
  assert.equal(layout.laneCount, 2)
})

test('graph exposes at most six real lanes and one overflow marker', () => {
  const parents = Array.from({ length: 8 }, (_, index) => `parent-${index}`)
  const layout = buildCommitGraph([
    commit('octopus', parents, ['main']),
    ...parents.map((parent) => commit(parent)),
  ], 'main')
  const merge = layout.rows.get('octopus')

  assert.equal(merge.overflowLane, maxVisibleGraphLanes)
  assert.ok(merge.outgoing.every((edge) => edge.to <= maxVisibleGraphLanes))
  assert.equal(layout.laneCount, maxVisibleGraphLanes + 1)
  assert.equal(layout.rows.get('parent-6').nodeOverflow, false)
})

test('hidden commits collapse to their nearest visible ancestors', () => {
  const items = [
    commit('head', ['hidden-merge']),
    commit('hidden-merge', ['main-base', 'topic-base']),
    commit('main-base', ['root']),
    commit('topic-base', ['root']),
    commit('root'),
  ]
  const projected = projectVisibleCommits(items, new Set(['head', 'main-base', 'topic-base', 'root']))

  assert.deepEqual(projected.map(({ commit: oid, parents }) => [oid, parents]), [
    ['head', ['main-base', 'topic-base']],
    ['main-base', ['root']],
    ['topic-base', ['root']],
    ['root', []],
  ])
})

test('one continuous drawing owns every row boundary', () => {
  const items = [
    commit('head', ['middle']),
    commit('middle', ['root']),
    commit('root'),
  ]
  const layout = buildCommitGraph(items, '')
  const drawing = buildCommitGraphDrawing(items, layout.rows, false)

  assert.equal(drawing.height, items.length * commitGraphRowHeight)
  assert.equal(commitGraphWidth, 96)
  assert.equal(drawing.paths.length, 1)
  assert.match(drawing.paths[0].d, /M 12 16 L 12 32 M 12 32 L 12 48/)
  assert.deepEqual(drawing.nodes.map(({ x, y }) => [x, y]), [[12, 16], [12, 48], [12, 80]])
})

test('loading older commits does not change an existing graph prefix', () => {
  const initial = [
    commit('head', ['middle'], ['main']),
    commit('middle', ['boundary']),
    commit('boundary', ['older']),
  ]
  const extended = [...initial, commit('older', ['root']), commit('root')]
  const initialLayout = buildCommitGraph(initial, 'main')
  const extendedLayout = buildCommitGraph(extended, 'main')

  for (const item of initial) assert.deepEqual(extendedLayout.rows.get(item.commit), initialLayout.rows.get(item.commit))
})
