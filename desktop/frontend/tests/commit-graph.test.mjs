import assert from 'node:assert/strict'
import test from 'node:test'

import { buildCommitGraph, defaultBranchGraphColorIndex, projectVisibleCommits } from '../src/lib/commit-graph.ts'
import { buildCommitGraphDrawing, commitGraphLaneColor, commitGraphRowHeight } from '../src/lib/commit-graph-render.ts'
import { commitGraphLaneLimitForWidth, commitGraphMinimumWidth, commitGraphWidthForLaneCount, maximumVisibleGraphLanes, minimumVisibleGraphLanes } from '../src/lib/commit-graph-sizing.ts'

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
  assert.equal(layout.historicalBranchTips.get('topic-head'), 'aws_s3_buckets')
  assert.equal(layout.historicalBranchTips.has('target'), false)
})

test('GitLab-style merge messages expose the source branch', () => {
  const layout = buildCommitGraph([
    commit('merge', ['main-work', 'topic'], ['main'], "Merge branch 'feature/search' into 'main'\n\nSee merge request platform/GitGit!73"),
    commit('main-work', ['base']),
    commit('topic', ['base']),
    commit('base'),
  ], 'main')

  assert.equal(layout.historicalBranches.get('topic'), 'feature/search')
  assert.equal(layout.historicalBranchTips.get('topic'), 'feature/search')
})

test('each non-first parent of an octopus merge is marked as a historical branch tip', () => {
  const layout = buildCommitGraph([
    commit('merge', ['main-work', 'topic-a', 'topic-b'], ['main'], "Merge branch 'release/candidates' into 'main'"),
    commit('main-work', ['base']),
    commit('topic-a', ['base']),
    commit('topic-b', ['base']),
    commit('base'),
  ], 'main')

  assert.equal(layout.historicalBranchTips.get('topic-a'), 'release/candidates')
  assert.equal(layout.historicalBranchTips.get('topic-b'), 'release/candidates')
  assert.equal(layout.historicalBranchTips.has('base'), false)
})

test('default branch stays left while a completed merge lane collapses into it', () => {
  const layout = buildCommitGraph([
    commit('merge', ['main-work', 'topic'], ['main']),
    commit('main-work', ['base']),
    commit('topic', ['base']),
    commit('base'),
  ], 'main')

  assert.equal(layout.rows.get('merge').lane, 0)
  assert.deepEqual(layout.rows.get('merge').outgoing, [{ color: 0, from: 0, to: 0 }, { color: 1, from: 0, to: 1 }])
  assert.deepEqual(layout.rows.get('topic').outgoing, [{ color: 0, from: 1, to: 0 }])
  assert.equal(layout.rows.get('base').lane, 0)
  assert.equal(layout.laneCount, 2)
})

test('default branch stays continuous when its first parent is already waiting in a side lane', () => {
  const items = [
    commit('main-head', ['next-main', 'topic-tip'], ['main']),
    commit('topic-tip', ['previous-main']),
    commit('next-main', ['previous-main']),
    commit('previous-main'),
  ]
  const layout = buildCommitGraph(items, 'main')
  const row = layout.rows.get('next-main')
  const drawing = buildCommitGraphDrawing(items, layout.rows, true)
  const defaultPath = drawing.paths.find((path) => path.color === '#55a7e8')?.d ?? ''

  assert.deepEqual(row.outgoing, [{ color: 0, from: 0, to: 0 }])
  assert.deepEqual(row.passThrough, [{ color: 1, from: 1, to: 0 }])
  assert.match(defaultPath, /M 12 64 L 12 80 M 12 80 L 12 96/)
})

test('default branch keeps its fixed color through side merges and lane compaction', () => {
  const items = [
    commit('main-head', ['main-next', 'topic-a'], ['main']),
    commit('topic-a', ['main-base']),
    commit('main-next', ['main-base', 'topic-b']),
    commit('topic-b', ['root']),
    commit('main-base', ['root']),
    commit('root'),
  ]
  const layout = buildCommitGraph(items, 'main', maximumVisibleGraphLanes)
  const defaultCommits = ['main-head', 'main-next', 'main-base', 'root']

  for (const oid of defaultCommits) {
    const row = layout.rows.get(oid)
    assert.equal(row.lane, 0)
    assert.equal(row.nodeColor, defaultBranchGraphColorIndex)
  }
  for (const oid of defaultCommits.slice(0, -1)) {
    assert.ok(layout.rows.get(oid).outgoing.some((edge) => (
      edge.from === 0
      && edge.to === 0
      && edge.color === defaultBranchGraphColorIndex
    )))
  }
  assert.notEqual(layout.rows.get('topic-a').nodeColor, defaultBranchGraphColorIndex)
  assert.notEqual(layout.rows.get('topic-b').nodeColor, defaultBranchGraphColorIndex)

  const drawing = buildCommitGraphDrawing(items, layout.rows, true)
  const defaultColor = commitGraphLaneColor(defaultBranchGraphColorIndex)
  assert.ok(drawing.nodes.filter((node) => defaultCommits.includes(node.commit)).every((node) => node.color === defaultColor))
  assert.ok(drawing.paths.some((path) => path.color === defaultColor && !path.gradientID))
  assert.equal(drawing.paths.at(-1).color, defaultColor)
  assert.equal(drawing.paths.at(-1).primary, true)
})

test('default path is painted after side paths at merge intersections', () => {
  const items = [
    commit('merge', ['main-next', 'topic'], ['main']),
    commit('topic', ['base']),
    commit('main-next', ['base']),
    commit('base'),
  ]
  const layout = buildCommitGraph(items, 'main', maximumVisibleGraphLanes)
  const drawing = buildCommitGraphDrawing(items, layout.rows, true)
  const defaultColor = commitGraphLaneColor(defaultBranchGraphColorIndex)
  const sidePathIndex = drawing.paths.findIndex((path) => path.color !== defaultColor)
  const defaultPathIndex = drawing.paths.findIndex((path) => path.primary)

  assert.ok(sidePathIndex >= 0)
  assert.ok(defaultPathIndex > sidePathIndex)
  assert.equal(defaultPathIndex, drawing.paths.length - 1)
  assert.match(drawing.paths[sidePathIndex].d, /M 18 16 C 18 24, 24 24, 24 32/)
})

test('All branches keeps the remote default blue when its symbolic HEAD decoration is filtered', () => {
  const items = [
    commit('remote-head', ['remote-next', 'topic'], ['origin/main']),
    commit('topic', ['remote-base']),
    commit('remote-next', ['remote-base']),
    commit('remote-base', ['local-head']),
    commit('local-head', ['root'], ['main']),
    commit('root'),
  ]
  const layout = buildCommitGraph(items, 'main', maximumVisibleGraphLanes, true)
  const defaultCommits = ['remote-head', 'remote-next', 'remote-base', 'local-head', 'root']

  for (const oid of defaultCommits) {
    assert.equal(layout.rows.get(oid).lane, 0)
    assert.equal(layout.rows.get(oid).nodeColor, defaultBranchGraphColorIndex)
  }
  assert.notEqual(layout.rows.get('topic').nodeColor, defaultBranchGraphColorIndex)
})

test('local branch scope still prefers its exact head over a divergent remote default', () => {
  const items = [
    commit('remote-head', ['base'], ['origin/main']),
    commit('local-head', ['base'], ['main']),
    commit('base'),
  ]
  const layout = buildCommitGraph(items, 'main', maximumVisibleGraphLanes)

  assert.notEqual(layout.rows.get('remote-head').nodeColor, defaultBranchGraphColorIndex)
  assert.equal(layout.rows.get('local-head').nodeColor, defaultBranchGraphColorIndex)
})

test('graph starts with six real lanes and one collapsed overflow lane', () => {
  const parents = Array.from({ length: 8 }, (_, index) => `parent-${index}`)
  const layout = buildCommitGraph([
    commit('octopus', parents, ['main']),
    ...parents.map((parent) => commit(parent)),
  ], 'main')
  const merge = layout.rows.get('octopus')

  assert.equal(merge.overflowLane, minimumVisibleGraphLanes)
  assert.ok(merge.outgoing.every((edge) => edge.to <= minimumVisibleGraphLanes))
  assert.equal(layout.laneCount, minimumVisibleGraphLanes + 1)
  assert.equal(layout.rows.get('parent-6').nodeOverflow, false)
})

test('wider tables expose more graph lanes up to the practical maximum', () => {
  const parents = Array.from({ length: 8 }, (_, index) => `parent-${index}`)
  const laneLimit = commitGraphLaneLimitForWidth(640)
  const layout = buildCommitGraph([
    commit('octopus', parents, ['main']),
    ...parents.map((parent) => commit(parent)),
  ], 'main', laneLimit)

  assert.ok(laneLimit > minimumVisibleGraphLanes)
  assert.equal(laneLimit, maximumVisibleGraphLanes)
  assert.equal(commitGraphLaneLimitForWidth(540), 8)
  assert.equal(layout.rows.get('octopus').overflowLane, null)
  assert.equal(commitGraphWidthForLaneCount(layout.laneCount), 108)
})

test('graph width stops growing after ten visible lanes and colors do not repeat', () => {
  const parents = Array.from({ length: 14 }, (_, index) => `parent-${index}`)
  const laneLimit = commitGraphLaneLimitForWidth(4_000)
  const layout = buildCommitGraph([
    commit('octopus', parents, ['main']),
    ...parents.map((parent) => commit(parent)),
  ], 'main', laneLimit)
  const drawing = buildCommitGraphDrawing([
    commit('octopus', parents, ['main']),
    ...parents.map((parent) => commit(parent)),
  ], layout.rows, true)

  assert.equal(laneLimit, maximumVisibleGraphLanes)
  assert.equal(layout.laneCount, maximumVisibleGraphLanes + 1)
  assert.equal(commitGraphWidthForLaneCount(layout.laneCount), 144)
  assert.equal(new Set(Array.from({ length: maximumVisibleGraphLanes }, (_, lane) => commitGraphLaneColor(lane))).size, maximumVisibleGraphLanes)
  assert.ok(drawing.paths.some((path) => path.overflow && path.color === '#65767d'))
  assert.ok(drawing.gradients.some((gradient) => gradient.startColor === '#55a7e8' && gradient.endColor === '#65767d'))
  assert.ok(drawing.paths.some((path) => path.gradientID && !path.overflow && path.color === '#55a7e8'))
})

test('overflow transitions fade toward the visible lane in either direction', () => {
  const item = commit('transition')
  const drawing = buildCommitGraphDrawing([item], new Map([[
    item.commit,
    {
      lane: 2,
      nodeColor: 2,
      nodeOverflow: false,
      overflowLane: maximumVisibleGraphLanes,
      passThrough: [{ color: 2, from: maximumVisibleGraphLanes, to: 2 }],
      incoming: [],
      outgoing: [],
    },
  ]]), false)

  assert.deepEqual(drawing.gradients.map(({ startColor, endColor }) => [startColor, endColor]), [
    ['#65767d', commitGraphLaneColor(2)],
  ])
  assert.equal(drawing.paths.filter((path) => path.overflow).length, 0)
})

test('a lineage keeps its color while its lane compacts left', () => {
  const items = [
    commit('head', ['main-next', 'side-head', 'other-head'], ['main']),
    commit('main-next', ['base']),
    commit('side-head', ['side-next']),
    commit('other-head', ['other-next']),
    commit('side-next', ['base']),
    commit('other-next', ['base']),
    commit('base'),
  ]
  const layout = buildCommitGraph(items, 'main', maximumVisibleGraphLanes)
  const compactingRow = layout.rows.get('side-next')
  const movedLineage = layout.rows.get('other-next')

  assert.ok(compactingRow.passThrough.some((edge) => edge.color === 2 && edge.from === 2 && edge.to === 1))
  assert.equal(movedLineage.lane, 1)
  assert.equal(movedLineage.nodeColor, 2)

  const drawing = buildCommitGraphDrawing(items, layout.rows, true)
  const lineagePath = drawing.paths.find((path) => path.color === commitGraphLaneColor(2) && !path.gradientID)?.d ?? ''
  assert.match(lineagePath, /M 36 128 C 36 140, 24 148, 24 160/)
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
  assert.equal(drawing.gradients.length, 0)
  assert.equal(commitGraphWidthForLaneCount(layout.laneCount), commitGraphMinimumWidth)
  assert.equal(drawing.paths.length, 1)
  assert.match(drawing.paths[0].d, /M 12 16 L 12 32 M 12 32 L 12 48/)
  assert.deepEqual(drawing.nodes.map(({ x, y }) => [x, y]), [[12, 16], [12, 48], [12, 80]])
})

test('branch badges can reuse the exact graph lane palette', () => {
  const items = [
    commit('merge', ['main-parent', 'topic-parent'], ['main']),
    commit('main-parent'),
    commit('topic-parent', [], ['topic']),
  ]
  const layout = buildCommitGraph(items, 'main')
  const drawing = buildCommitGraphDrawing(items, layout.rows, true)

  for (const node of drawing.nodes) {
    assert.equal(node.color, commitGraphLaneColor(layout.rows.get(node.commit).nodeColor))
  }
})

test('continuous drawing bridges date separator gaps', () => {
  const items = [commit('head', ['root'], ['main']), commit('root')]
  const layout = buildCommitGraph(items, 'main')
  const rowTops = new Map([['head', 22], ['root', 76]])
  const drawing = buildCommitGraphDrawing(items, layout.rows, true, rowTops, 108)
  const defaultPath = drawing.paths.find((path) => path.color === '#55a7e8')?.d ?? ''

  assert.equal(drawing.height, 108)
  assert.match(defaultPath, /M 12 54 L 12 76/)
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
