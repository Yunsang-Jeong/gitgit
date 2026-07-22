import type { CommitSummary } from './types'
import type { GraphEdge, GraphRow } from './commit-graph'
import { commitGraphLaneSpacing } from './commit-graph-sizing.ts'

export const commitGraphRowHeight = 32

const laneColors = [
  '#55a7e8',
  '#d568a7',
  '#62b97a',
  '#d69a2d',
  '#9a78d1',
  '#47b8b2',
  '#e07a5f',
  '#8fbc5a',
  '#7b8fe4',
  '#c989d9',
]
const overflowLaneColor = '#65767d'

export type CommitGraphPath = {
  color: string
  d: string
  gradientID: string | null
  overflow: boolean
}

export type CommitGraphGradient = {
  endColor: string
  id: string
  startColor: string
  x1: number
  x2: number
  y1: number
  y2: number
}

export type CommitGraphNode = {
  color: string
  commit: string
  primary: boolean
  radius: number
  x: number
  y: number
}

export type CommitGraphDrawing = {
  gradients: CommitGraphGradient[]
  height: number
  nodes: CommitGraphNode[]
  paths: CommitGraphPath[]
}

function laneX(lane: number): number {
  return 12 + lane * commitGraphLaneSpacing
}

export function commitGraphLaneColor(lane: number): string {
  return laneColors[lane % laneColors.length]
}

function addSegment(segments: Map<string, string[]>, color: string, d: string, overflow = false): void {
  const key = `${color}:${overflow ? 'overflow' : 'lane'}`
  const path = segments.get(key) ?? []
  path.push(d)
  segments.set(key, path)
}

function addOverflowTransition(
  gradients: CommitGraphGradient[],
  paths: CommitGraphPath[],
  id: string,
  d: string,
  edge: GraphEdge,
  overflowLane: number,
  x1: number,
  y1: number,
  x2: number,
  y2: number,
): void {
  const fromOverflow = edge.from === overflowLane
  const visibleColor = commitGraphLaneColor(edge.color)
  gradients.push({
    endColor: fromOverflow ? visibleColor : overflowLaneColor,
    id,
    startColor: fromOverflow ? overflowLaneColor : visibleColor,
    x1,
    x2,
    y1,
    y2,
  })
  paths.push({ color: visibleColor, d, gradientID: id, overflow: false })
}

export function buildCommitGraphDrawing(
  commits: CommitSummary[],
  rows: Map<string, GraphRow>,
  showPrimaryLane: boolean,
  rowTops?: Map<string, number>,
  drawingHeight?: number,
): CommitGraphDrawing {
  const segments = new Map<string, string[]>()
  const gradients: CommitGraphGradient[] = []
  const nodes: CommitGraphNode[] = []
  const transitionPaths: CommitGraphPath[] = []

  commits.forEach((commit, rowIndex) => {
    const row = rows.get(commit.commit)
    if (!row) return
    const top = rowTops?.get(commit.commit) ?? rowIndex * commitGraphRowHeight
    const middle = top + commitGraphRowHeight / 2
    const bottom = top + commitGraphRowHeight
    const nextCommit = commits[rowIndex + 1]
    const nextTop = nextCommit ? rowTops?.get(nextCommit.commit) ?? bottom : bottom

    for (const [edgeIndex, edge] of row.passThrough.entries()) {
      const fromX = laneX(edge.from)
      const toX = laneX(edge.to)
      const fromOverflow = edge.from === row.overflowLane
      const toOverflow = edge.to === row.overflowLane
      const d = edge.from === edge.to
        ? `M ${fromX} ${top} L ${toX} ${bottom}`
        : `M ${fromX} ${top} C ${fromX} ${top + 12}, ${toX} ${bottom - 12}, ${toX} ${bottom}`
      if (row.overflowLane !== null && fromOverflow !== toOverflow) {
        addOverflowTransition(gradients, transitionPaths, `commit-overflow-${commit.commit}-pass-${edgeIndex}`, d, edge, row.overflowLane, fromX, top, toX, bottom)
      } else {
        addSegment(segments, fromOverflow ? overflowLaneColor : commitGraphLaneColor(edge.color), d, fromOverflow)
      }
    }

    for (const [edgeIndex, edge] of row.incoming.entries()) {
      const fromX = laneX(edge.from)
      const toX = laneX(edge.to)
      const fromOverflow = edge.from === row.overflowLane
      const toOverflow = edge.to === row.overflowLane
      const d = edge.from === edge.to
        ? `M ${fromX} ${top} L ${toX} ${middle}`
        : `M ${fromX} ${top} C ${fromX} ${top + 8}, ${toX} ${top + 8}, ${toX} ${middle}`
      if (row.overflowLane !== null && fromOverflow !== toOverflow) {
        addOverflowTransition(gradients, transitionPaths, `commit-overflow-${commit.commit}-in-${edgeIndex}`, d, edge, row.overflowLane, fromX, top, toX, middle)
      } else {
        addSegment(segments, fromOverflow ? overflowLaneColor : commitGraphLaneColor(edge.color), d, fromOverflow)
      }
    }

    for (const [edgeIndex, edge] of row.outgoing.entries()) {
      const fromX = laneX(edge.from)
      const toX = laneX(edge.to)
      const fromOverflow = edge.from === row.overflowLane
      const toOverflow = edge.to === row.overflowLane
      const d = edge.from === edge.to
        ? `M ${fromX} ${middle} L ${toX} ${bottom}`
        : `M ${fromX} ${middle} C ${fromX} ${top + 24}, ${toX} ${top + 24}, ${toX} ${bottom}`
      if (row.overflowLane !== null && fromOverflow !== toOverflow) {
        addOverflowTransition(gradients, transitionPaths, `commit-overflow-${commit.commit}-out-${edgeIndex}`, d, edge, row.overflowLane, fromX, middle, toX, bottom)
      } else {
        addSegment(segments, toOverflow ? overflowLaneColor : commitGraphLaneColor(edge.color), d, toOverflow)
      }
    }

    if (nextTop > bottom) {
      const bridges = new Map<number, { color: string; overflow: boolean }>()
      for (const edge of [...row.passThrough, ...row.outgoing]) {
        const overflow = edge.to === row.overflowLane
        bridges.set(edge.to, { color: overflow ? overflowLaneColor : commitGraphLaneColor(edge.color), overflow })
      }
      for (const [lane, bridge] of bridges) {
        const x = laneX(lane)
        addSegment(segments, bridge.color, `M ${x} ${bottom} L ${x} ${nextTop}`, bridge.overflow)
      }
    }

    if (!row.nodeOverflow) {
      const primary = row.lane === 0 && showPrimaryLane
      nodes.push({
        color: commitGraphLaneColor(row.nodeColor),
        commit: commit.commit,
        primary,
        radius: primary ? 4 : 3.5,
        x: laneX(row.lane),
        y: middle,
      })
    }
  })

  const paths: CommitGraphPath[] = [...segments.entries()].map(([key, path]) => {
    const [color, kind] = key.split(':')
    return { color, d: path.join(' '), gradientID: null, overflow: kind === 'overflow' }
  })

  return {
    gradients,
    height: drawingHeight ?? commits.length * commitGraphRowHeight,
    nodes,
    paths: [...paths, ...transitionPaths],
  }
}
