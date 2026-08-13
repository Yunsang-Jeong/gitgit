import type { CommitSummary } from './types'

export const historyDateSeparatorHeight = 22

export type HistoryDateSeparator = {
  key: string
  label: string
}

export type HistoryDateRow = {
  commit: CommitSummary
  separator: HistoryDateSeparator | null
}

export type HistoryRowGeometry = {
  height: number
  tops: Map<string, number>
}

function calendarStart(value: Date): Date {
  return new Date(value.getFullYear(), value.getMonth(), value.getDate())
}

function dateSeparator(value: string, now: Date): HistoryDateSeparator | null {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null

  const today = calendarStart(now)
  const recentStart = new Date(today)
  recentStart.setDate(recentStart.getDate() - 6)
  const commitDay = calendarStart(date)
  const year = date.getFullYear()
  const month = date.getMonth() + 1

  if (commitDay >= recentStart) {
    return { key: `day:${year}-${month}-${date.getDate()}`, label: `${year}. ${month}. ${date.getDate()}.` }
  }
  if (year === now.getFullYear()) {
    return { key: `month:${year}-${month}`, label: `${year}. ${month}. 1.` }
  }
  return { key: `year:${year}`, label: `${year}. 1. 1.` }
}

export function buildHistoryDateRows(commits: CommitSummary[], now = new Date()): HistoryDateRow[] {
  let previousSeparatorKey = ''
  return commits.map((commit) => {
    const candidate = dateSeparator(commit.date, now)
    const separator = candidate && candidate.key !== previousSeparatorKey ? candidate : null
    if (candidate) previousSeparatorKey = candidate.key
    return { commit, separator }
  })
}

export function buildHistoryRowGeometry(rows: HistoryDateRow[], commitRowHeight: number): HistoryRowGeometry {
  const tops = new Map<string, number>()
  let height = 0
  for (const row of rows) {
    if (row.separator) height += historyDateSeparatorHeight
    tops.set(row.commit.commit, height)
    height += commitRowHeight
  }
  return { height, tops }
}
