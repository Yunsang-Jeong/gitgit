export type DateBoundary = 'since' | 'until'

type ParsedDatePattern = {
  start: Date
  end: Date
}

export function formatDate(value: string, includeTime = false): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const calendar = `${date.getFullYear()}. ${date.getMonth() + 1}. ${date.getDate()}.`
  if (!includeTime) return calendar
  return `${calendar} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

export function matchesDatePattern(value: string, pattern: string, now = new Date()): boolean | null {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return false
  const relative = /^last:(\d+)d$/i.exec(pattern.trim())
  if (relative) {
    const days = Number(relative[1])
    if (!Number.isFinite(days) || days < 0) return false
    const threshold = new Date(now)
    threshold.setDate(threshold.getDate() - days)
    return date >= threshold && date <= now
  }
  const absolute = parseDisplayDate(pattern)
  if (!absolute) return null
  return date >= absolute.start && date < absolute.end
}

export function normalizeSearchBoundary(value: string, boundary: DateBoundary, now = new Date()): string {
  const normalized = value.trim()
  if (!normalized) return ''
  const relative = /^last:(\d+)d$/i.exec(normalized)
  if (relative) {
    const days = Number(relative[1])
    if (!Number.isFinite(days) || days < 0) return normalized
    const date = new Date(now)
    date.setDate(date.getDate() - days)
    if (Number.isNaN(date.getTime())) return normalized
    return date.toISOString()
  }
  const absolute = parseDisplayDate(normalized)
  if (!absolute) return normalized
  return (boundary === 'since' ? absolute.start : new Date(absolute.end.getTime() - 1)).toISOString()
}

function parseDisplayDate(value: string): ParsedDatePattern | null {
  const match = /^(\d{4})\.\s*(\d{1,2})\.\s*(\d{1,2})\.(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?)?$/.exec(value.trim())
  if (!match) return null
  const [, yearValue, monthValue, dayValue, hourValue, minuteValue, secondValue] = match
  const year = Number(yearValue)
  const month = Number(monthValue)
  const day = Number(dayValue)
  const hour = hourValue === undefined ? 0 : Number(hourValue)
  const minute = minuteValue === undefined ? 0 : Number(minuteValue)
  const second = secondValue === undefined ? 0 : Number(secondValue)
  const start = new Date(year, month - 1, day, hour, minute, second, 0)
  if (
    start.getFullYear() !== year
    || start.getMonth() !== month - 1
    || start.getDate() !== day
    || start.getHours() !== hour
    || start.getMinutes() !== minute
    || start.getSeconds() !== second
  ) return null
  const end = new Date(start)
  if (hourValue === undefined) end.setDate(end.getDate() + 1)
  else if (secondValue === undefined) end.setMinutes(end.getMinutes() + 1)
  else end.setSeconds(end.getSeconds() + 1)
  return { start, end }
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}
