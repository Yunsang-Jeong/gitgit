export const minimumVisibleGraphLanes = 6
export const maximumVisibleGraphLanes = 10
export const commitGraphLaneSpacing = 12
export const commitGraphMinimumWidth = 96

const historyRefCellWidth = 158
const minimumMessageWidth = 260
const graphHorizontalPadding = 24

export function commitGraphLaneLimitForWidth(tableWidth: number): number {
  const availableGraphWidth = Math.max(commitGraphMinimumWidth, Math.floor(tableWidth) - historyRefCellWidth - minimumMessageWidth)
  const responsiveLaneLimit = Math.floor((availableGraphWidth - graphHorizontalPadding) / commitGraphLaneSpacing)
  return Math.min(maximumVisibleGraphLanes, Math.max(minimumVisibleGraphLanes, responsiveLaneLimit))
}

export function commitGraphWidthForLaneCount(laneCount: number): number {
  return Math.max(commitGraphMinimumWidth, graphHorizontalPadding + Math.max(0, laneCount - 1) * commitGraphLaneSpacing)
}
