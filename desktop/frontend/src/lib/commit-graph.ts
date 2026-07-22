import type { CommitSummary } from './types'

export type GraphEdge = {
  from: number
  to: number
}

export type GraphRow = {
  lane: number
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
}

export const maxVisibleGraphLanes = 6

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

function historicalBranchLabels(items: CommitSummary[], commitsByID: Map<string, CommitSummary>): Map<string, string> {
  const labels = new Map<string, string>()
  for (const merge of items) {
    if ((merge.parents?.length ?? 0) < 2) continue
    const branch = historicalBranchFromMerge(merge.message)
    if (!branch) continue
    const firstParentHistory = reachableCommits(merge.parents[0], commitsByID)
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
  return labels
}

function compactLanes(lanes: Array<string | null>, reserveDefaultLane: boolean): Array<string | null> {
  const compacted: Array<string | null> = reserveDefaultLane ? [lanes[0] ?? null] : []
  const start = reserveDefaultLane ? 1 : 0
  for (let index = start; index < lanes.length; index += 1) {
    const oid = lanes[index]
    if (oid !== null && !compacted.includes(oid)) compacted.push(oid)
  }
  return compacted
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

function limitRow(row: GraphRow): GraphRow {
  const overflowLane = maxVisibleGraphLanes
  const referencedLanes = [
    row.lane,
    ...row.passThrough.flatMap((edge) => [edge.from, edge.to]),
    ...row.incoming.flatMap((edge) => [edge.from, edge.to]),
    ...row.outgoing.flatMap((edge) => [edge.from, edge.to]),
  ]
  const truncated = referencedLanes.some((lane) => lane >= maxVisibleGraphLanes)
  if (!truncated) return row

  const visibleLane = (lane: number): number => Math.min(lane, overflowLane)
  return {
    lane: visibleLane(row.lane),
    nodeOverflow: row.lane >= maxVisibleGraphLanes,
    overflowLane,
    passThrough: uniqueEdges(row.passThrough.map((edge) => ({ from: visibleLane(edge.from), to: visibleLane(edge.to) }))),
    incoming: uniqueEdges(row.incoming.map((edge) => ({ from: visibleLane(edge.from), to: visibleLane(edge.to) }))),
    outgoing: uniqueEdges(row.outgoing.map((edge) => ({ from: visibleLane(edge.from), to: visibleLane(edge.to) }))),
  }
}

export function buildCommitGraph(items: CommitSummary[], primaryBranch: string): GraphLayout {
  const commitsByID = new Map(items.map((commit) => [commit.commit, commit]))
  const historicalBranches = historicalBranchLabels(items, commitsByID)
  const defaultChain = new Set<string>()
  const defaultHead = primaryBranch
    ? items.find((commit) => commit.refs?.includes(primaryBranch))
    : undefined

  let defaultCommit = defaultHead
  while (defaultCommit && !defaultChain.has(defaultCommit.commit)) {
    defaultChain.add(defaultCommit.commit)
    defaultCommit = commitsByID.get(defaultCommit.parents?.[0] ?? '')
  }

  const reserveDefaultLane = Boolean(primaryBranch)
  let lanes: Array<string | null> = reserveDefaultLane ? [null] : []
  const rows = new Map<string, GraphRow>()
  let laneCount = lanes.length

  for (const commit of items) {
    const onDefaultChain = defaultChain.has(commit.commit)
    let incomingLanes = lanes
      .map((value, index) => value === commit.commit ? index : -1)
      .filter((index) => index >= 0)

    if (onDefaultChain && !incomingLanes.includes(0)) {
      lanes[0] = commit.commit
      incomingLanes = [0, ...incomingLanes]
    } else if (incomingLanes.length === 0) {
      const start = reserveDefaultLane ? 1 : 0
      const available = lanes.findIndex((value, index) => index >= start && value === null)
      const lane = available >= 0 ? available : lanes.length
      lanes[lane] = commit.commit
      incomingLanes = [lane]
    }

    const lane = onDefaultChain ? 0 : incomingLanes[0]
    const incoming = incomingLanes.map((from) => ({ from, to: lane }))
    const continuing = lanes
      .map((oid, from) => oid !== null && !incomingLanes.includes(from) ? { oid, from } : null)
      .filter((value): value is { oid: string; from: number } => value !== null)

    for (const index of incomingLanes) lanes[index] = null

    const parentSources: Array<{ parent: string; from: number }> = []
    for (const [parentIndex, parent] of (commit.parents ?? []).entries()) {
      let target = lanes.indexOf(parent)
      if (target < 0) {
        if (parentIndex === 0 && lanes[lane] === null) {
          target = lane
        } else {
          const start = reserveDefaultLane ? 1 : 0
          const available = lanes.findIndex((value, index) => index >= start && value === null)
          target = available >= 0 ? available : lanes.length
        }
        lanes[target] = parent
      }
      parentSources.push({ parent, from: lane })
    }

    lanes = compactLanes(lanes, reserveDefaultLane)
    const bottomLaneByOID = new Map<string, number>()
    lanes.forEach((oid, index) => {
      if (oid !== null) bottomLaneByOID.set(oid, index)
    })

    const passThrough = continuing
      .map(({ oid, from }) => ({ from, to: bottomLaneByOID.get(oid) ?? from }))
    const outgoing = parentSources
      .map(({ parent, from }) => ({ from, to: bottomLaneByOID.get(parent) ?? from }))
    const limitedRow = limitRow({
      lane,
      nodeOverflow: false,
      overflowLane: null,
      passThrough: uniqueEdges(passThrough),
      incoming: uniqueEdges(incoming),
      outgoing: uniqueEdges(outgoing),
    })

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

  return { rows, laneCount: Math.max(1, laneCount), historicalBranches }
}
