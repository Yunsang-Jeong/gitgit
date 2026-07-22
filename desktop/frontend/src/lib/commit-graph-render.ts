import type { CommitSummary } from './types'
import type { GraphRow } from './commit-graph'

export const commitGraphRowHeight = 32
export const commitGraphWidth = 96
export const commitGraphLaneSpacing = 12

const laneColors = ['#55a7e8', '#d568a7', '#62b97a', '#d69a2d', '#9a78d1']

export type CommitGraphPath = {
  color: string
  d: string
}

export type CommitGraphNode = {
  color: string
  commit: string
  primary: boolean
  radius: number
  x: number
  y: number
}

export type CommitGraphMarker = {
  x: number
  y: number
}

export type CommitGraphDrawing = {
  height: number
  markers: CommitGraphMarker[]
  nodes: CommitGraphNode[]
  paths: CommitGraphPath[]
}

function laneX(lane: number): number {
  return 12 + lane * commitGraphLaneSpacing
}

function laneColor(lane: number): string {
  return laneColors[lane % laneColors.length]
}

function addSegment(segments: Map<string, string[]>, color: string, d: string): void {
  const path = segments.get(color) ?? []
  path.push(d)
  segments.set(color, path)
}

export function buildCommitGraphDrawing(commits: CommitSummary[], rows: Map<string, GraphRow>, showPrimaryLane: boolean): CommitGraphDrawing {
  const segments = new Map<string, string[]>()
  const nodes: CommitGraphNode[] = []
  const markers: CommitGraphMarker[] = []

  commits.forEach((commit, rowIndex) => {
    const row = rows.get(commit.commit)
    if (!row) return
    const top = rowIndex * commitGraphRowHeight
    const middle = top + commitGraphRowHeight / 2
    const bottom = top + commitGraphRowHeight

    for (const edge of row.passThrough) {
      const fromX = laneX(edge.from)
      const toX = laneX(edge.to)
      const d = edge.from === edge.to
        ? `M ${fromX} ${top} L ${toX} ${bottom}`
        : `M ${fromX} ${top} C ${fromX} ${top + 12}, ${toX} ${bottom - 12}, ${toX} ${bottom}`
      addSegment(segments, laneColor(edge.to), d)
    }

    for (const edge of row.incoming) {
      const fromX = laneX(edge.from)
      const toX = laneX(edge.to)
      const d = edge.from === edge.to
        ? `M ${fromX} ${top} L ${toX} ${middle}`
        : `M ${fromX} ${top} C ${fromX} ${top + 8}, ${toX} ${top + 8}, ${toX} ${middle}`
      addSegment(segments, laneColor(edge.from), d)
    }

    for (const edge of row.outgoing) {
      const fromX = laneX(edge.from)
      const toX = laneX(edge.to)
      const d = edge.from === edge.to
        ? `M ${fromX} ${middle} L ${toX} ${bottom}`
        : `M ${fromX} ${middle} C ${fromX} ${top + 24}, ${toX} ${top + 24}, ${toX} ${bottom}`
      addSegment(segments, laneColor(edge.to), d)
    }

    if (row.overflowLane !== null) markers.push({ x: laneX(row.overflowLane), y: middle + 4 })
    if (!row.nodeOverflow) {
      const primary = row.lane === 0 && showPrimaryLane
      nodes.push({
        color: laneColor(row.lane),
        commit: commit.commit,
        primary,
        radius: primary ? 4 : 3.5,
        x: laneX(row.lane),
        y: middle,
      })
    }
  })

  return {
    height: commits.length * commitGraphRowHeight,
    markers,
    nodes,
    paths: [...segments.entries()].map(([color, path]) => ({ color, d: path.join(' ') })),
  }
}
