import type { CommitSummary } from './types'
import { maximumVisibleGraphLanes, minimumVisibleGraphLanes } from './commit-graph-sizing.ts'

export const defaultBranchGraphColorIndex = 0

export type GraphEdge = {
  color: number
  from: number
  to: number
}

export type GraphRow = {
  lane: number
  nodeColor: number
  nodeOverflow: boolean
  overflowLane: number | null
  passThrough: GraphEdge[]
  incoming: GraphEdge[]
  outgoing: GraphEdge[]
}

export type GraphLayout = {
  rows: Map<string, GraphRow>
  laneCount: number
  historicalBranches: Map<string, string>
  historicalBranchTips: Map<string, string>
}

export function projectVisibleCommits(items: CommitSummary[], visibleCommits: Set<string>): CommitSummary[] {
  if (visibleCommits.size === items.length) return items

  const commitsByID = new Map(items.map((commit) => [commit.commit, commit]))
  const resolvedParents = new Map<string, string[]>()

  function nearestVisibleParents(oid: string, visiting = new Set<string>()): string[] {
    if (!oid) return []
    if (visibleCommits.has(oid)) return [oid]
    const cached = resolvedParents.get(oid)
    if (cached) return cached
    if (visiting.has(oid)) return []

    const commit = commitsByID.get(oid)
    if (!commit) return [oid]

    const nextVisiting = new Set(visiting).add(oid)
    const parents = [...new Set((commit.parents ?? []).flatMap((parent) => nearestVisibleParents(parent, nextVisiting)))]
    resolvedParents.set(oid, parents)
    return parents
  }

  return items
    .filter((commit) => visibleCommits.has(commit.commit))
    .map((commit) => ({
      ...commit,
      parents: [...new Set((commit.parents ?? []).flatMap((parent) => nearestVisibleParents(parent)))],
    }))
}

function historicalBranchFromMerge(message: string): string {
  const pullRequest = message.match(/^Merge pull request #\d+ from [^/\s]+\/(\S+)/im)
  if (pullRequest) return pullRequest[1]
  const mergeBranch = message.match(/^Merge branch ['"]([^'"]+)['"](?:\s+of\s+\S+)?\s+into\b/im)
  return mergeBranch?.[1] ?? ''
}

function reachableCommits(start: string, commitsByID: Map<string, CommitSummary>): Set<string> {
  const reachable = new Set<string>()
  const pending = start ? [start] : []
  while (pending.length > 0) {
    const oid = pending.pop() ?? ''
    if (!oid || reachable.has(oid)) continue
    reachable.add(oid)
    for (const parent of commitsByID.get(oid)?.parents ?? []) pending.push(parent)
  }
  return reachable
}

function historicalBranchContext(items: CommitSummary[], commitsByID: Map<string, CommitSummary>): {
  labels: Map<string, string>
  tips: Map<string, string>
} {
  const labels = new Map<string, string>()
  const tips = new Map<string, string>()
  for (const merge of items) {
    if ((merge.parents?.length ?? 0) < 2) continue
    const branch = historicalBranchFromMerge(merge.message)
    if (!branch) continue
    const firstParentHistory = reachableCommits(merge.parents[0], commitsByID)
    for (const parent of merge.parents.slice(1)) {
      if (parent && commitsByID.has(parent) && !firstParentHistory.has(parent)) tips.set(parent, branch)
    }
    const pending = merge.parents.slice(1)
    const visited = new Set<string>()
    while (pending.length > 0) {
      const oid = pending.pop() ?? ''
      if (!oid || visited.has(oid) || firstParentHistory.has(oid)) continue
      visited.add(oid)
      if (!commitsByID.has(oid)) continue
      labels.set(oid, branch)
      for (const parent of commitsByID.get(oid)?.parents ?? []) pending.push(parent)
    }
  }
  return { labels, tips }
}

function primaryBranchHead(items: CommitSummary[], primaryBranch: string, preferRemoteDefault: boolean): CommitSummary | undefined {
  if (!primaryBranch) return undefined

  const remoteHead = items.find((commit) => (commit.refs ?? []).some((ref) => {
    if (!ref.endsWith('/HEAD')) return false
    const remote = ref.slice(0, -'/HEAD'.length)
    return commit.refs?.includes(`${remote}/${primaryBranch}`)
  }))
  const exactHead = items.find((commit) => commit.refs?.includes(primaryBranch))
  const originHead = primaryBranch.includes('/')
    ? undefined
    : items.find((commit) => commit.refs?.includes(`origin/${primaryBranch}`))
  const remoteBranchHead = items.find((commit) => commit.refs?.some((ref) => ref.endsWith(`/${primaryBranch}`)))

  return preferRemoteDefault
    ? remoteHead ?? originHead ?? remoteBranchHead ?? exactHead
    : exactHead ?? remoteHead ?? originHead ?? remoteBranchHead
}

export function hasPrimaryBranchHead(items: CommitSummary[], primaryBranch: string, preferRemoteDefault = false): boolean {
  return primaryBranchHead(items, primaryBranch, preferRemoteDefault) !== undefined
}

export function isLocalPrimaryBranchHead(items: CommitSummary[], primaryBranch: string, preferRemoteDefault = false): boolean {
  const head = primaryBranchHead(items, primaryBranch, preferRemoteDefault)
  return Boolean(primaryBranch) && Boolean(head?.refs?.includes(primaryBranch))
}

type ActiveLane = {
  color: number | null
  oid: string | null
}

function emptyLane(): ActiveLane {
  return { color: null, oid: null }
}

function compactLanes(lanes: ActiveLane[], reserveDefaultLane: boolean): ActiveLane[] {
  const compacted: ActiveLane[] = reserveDefaultLane ? [{ ...(lanes[0] ?? emptyLane()) }] : []
  const seen = new Set(compacted.flatMap((lane) => lane.oid ? [lane.oid] : []))
  const start = reserveDefaultLane ? 1 : 0
  for (let index = start; index < lanes.length; index += 1) {
    const lane = lanes[index]
    if (lane.oid !== null && !seen.has(lane.oid)) {
      compacted.push({ ...lane })
      seen.add(lane.oid)
    }
  }
  return compacted
}

function availableLaneColor(lanes: ActiveLane[], reserveDefaultLane: boolean): number {
  const used = new Set(lanes.flatMap((lane) => lane.oid !== null && lane.color !== null ? [lane.color] : []))
  const start = reserveDefaultLane ? defaultBranchGraphColorIndex + 1 : 0
  for (let color = start; color < maximumVisibleGraphLanes; color += 1) {
    if (!used.has(color)) return color
  }
  let overflowColor = maximumVisibleGraphLanes
  while (used.has(overflowColor)) overflowColor += 1
  return overflowColor
}

function uniqueEdges(edges: GraphEdge[]): GraphEdge[] {
  const seen = new Set<string>()
  return edges.filter((edge) => {
    const key = `${edge.from}:${edge.to}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function limitRow(row: GraphRow, visibleLaneLimit: number): GraphRow {
  const overflowLane = visibleLaneLimit
  const referencedLanes = [
    row.lane,
    ...row.passThrough.flatMap((edge) => [edge.from, edge.to]),
    ...row.incoming.flatMap((edge) => [edge.from, edge.to]),
    ...row.outgoing.flatMap((edge) => [edge.from, edge.to]),
  ]
  const truncated = referencedLanes.some((lane) => lane >= visibleLaneLimit)
  if (!truncated) return row

  const visibleLane = (lane: number): number => Math.min(lane, overflowLane)
  return {
    lane: visibleLane(row.lane),
    nodeColor: row.nodeColor,
    nodeOverflow: row.lane >= visibleLaneLimit,
    overflowLane,
    passThrough: uniqueEdges(row.passThrough.map((edge) => ({ ...edge, from: visibleLane(edge.from), to: visibleLane(edge.to) }))),
    incoming: uniqueEdges(row.incoming.map((edge) => ({ ...edge, from: visibleLane(edge.from), to: visibleLane(edge.to) }))),
    outgoing: uniqueEdges(row.outgoing.map((edge) => ({ ...edge, from: visibleLane(edge.from), to: visibleLane(edge.to) }))),
  }
}

export function buildCommitGraph(
  items: CommitSummary[],
  primaryBranch: string,
  visibleLaneLimit = minimumVisibleGraphLanes,
  preferRemoteDefault = false,
): GraphLayout {
  const commitsByID = new Map(items.map((commit) => [commit.commit, commit]))
  const historicalBranchContextValue = historicalBranchContext(items, commitsByID)
  const historicalBranches = historicalBranchContextValue.labels
  const historicalBranchTips = historicalBranchContextValue.tips
  const defaultChain = new Set<string>()
  const defaultHead = primaryBranchHead(items, primaryBranch, preferRemoteDefault)

  let defaultCommit = defaultHead
  while (defaultCommit && !defaultChain.has(defaultCommit.commit)) {
    defaultChain.add(defaultCommit.commit)
    defaultCommit = commitsByID.get(defaultCommit.parents?.[0] ?? '')
  }

  const reserveDefaultLane = Boolean(primaryBranch)
  let lanes: ActiveLane[] = reserveDefaultLane ? [emptyLane()] : []
  const rows = new Map<string, GraphRow>()
  let laneCount = lanes.length

  for (const commit of items) {
    const onDefaultChain = defaultChain.has(commit.commit)
    let incomingLanes = lanes
      .map((value, index) => value.oid === commit.commit ? index : -1)
      .filter((index) => index >= 0)

    if (onDefaultChain && !incomingLanes.includes(0)) {
      lanes[0] = { color: defaultBranchGraphColorIndex, oid: commit.commit }
      incomingLanes = [0, ...incomingLanes]
    } else if (incomingLanes.length === 0) {
      const start = reserveDefaultLane ? 1 : 0
      const available = lanes.findIndex((value, index) => index >= start && value.oid === null)
      const lane = available >= 0 ? available : lanes.length
      lanes[lane] = { color: availableLaneColor(lanes, reserveDefaultLane), oid: commit.commit }
      incomingLanes = [lane]
    }

    const lane = onDefaultChain ? 0 : incomingLanes[0]
    const nodeColor = onDefaultChain
      ? defaultBranchGraphColorIndex
      : lanes[lane]?.color ?? availableLaneColor(lanes, reserveDefaultLane)
    const incoming = incomingLanes.map((from) => ({ color: lanes[from]?.color ?? nodeColor, from, to: lane }))
    const continuing = lanes
      .map((activeLane, from) => activeLane.oid !== null && activeLane.color !== null && !incomingLanes.includes(from)
        ? { color: activeLane.color, oid: activeLane.oid, from }
        : null)
      .filter((value): value is { color: number; oid: string; from: number } => value !== null)

    for (const index of incomingLanes) lanes[index] = emptyLane()

    const parentSources: Array<{ color: number; parent: string; from: number }> = []
    for (const [parentIndex, parent] of (commit.parents ?? []).entries()) {
      let target = onDefaultChain && parentIndex === 0 ? 0 : lanes.findIndex((activeLane) => activeLane.oid === parent)
      if (target < 0) {
        if (parentIndex === 0 && lanes[lane].oid === null) {
          target = lane
        } else {
          const start = reserveDefaultLane ? 1 : 0
          const available = lanes.findIndex((value, index) => index >= start && value.oid === null)
          target = available >= 0 ? available : lanes.length
        }
      }
      const existingTarget = lanes[target]
      const color = existingTarget && existingTarget.oid !== null && existingTarget.color !== null
        ? existingTarget.color
        : parentIndex === 0
          ? nodeColor
          : availableLaneColor(lanes, reserveDefaultLane)
      lanes[target] = { color, oid: parent }
      parentSources.push({ color, parent, from: lane })
    }

    lanes = compactLanes(lanes, reserveDefaultLane)
    const bottomLaneByOID = new Map<string, number>()
    lanes.forEach((activeLane, index) => {
      if (activeLane.oid !== null) bottomLaneByOID.set(activeLane.oid, index)
    })

    const passThrough = continuing
      .map(({ color, oid, from }) => ({ color, from, to: bottomLaneByOID.get(oid) ?? from }))
    const outgoing = parentSources
      .map(({ color, parent, from }) => ({ color, from, to: bottomLaneByOID.get(parent) ?? from }))
    const limitedRow = limitRow({
      lane,
      nodeColor,
      nodeOverflow: false,
      overflowLane: null,
      passThrough: uniqueEdges(passThrough),
      incoming: uniqueEdges(incoming),
      outgoing: uniqueEdges(outgoing),
    }, visibleLaneLimit)

    laneCount = Math.max(
      laneCount,
      limitedRow.lane + 1,
      limitedRow.overflowLane === null ? 0 : limitedRow.overflowLane + 1,
      ...limitedRow.passThrough.flatMap((edge) => [edge.from + 1, edge.to + 1]),
      ...limitedRow.incoming.flatMap((edge) => [edge.from + 1, edge.to + 1]),
      ...limitedRow.outgoing.flatMap((edge) => [edge.from + 1, edge.to + 1]),
    )
    rows.set(commit.commit, limitedRow)
  }

  return { rows, laneCount: Math.max(1, laneCount), historicalBranches, historicalBranchTips }
}
