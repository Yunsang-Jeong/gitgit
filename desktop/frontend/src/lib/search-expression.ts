import type { Pattern } from './types'

export function searchExpressionError(patterns: Pattern[]): string {
  let balance = 0
  for (const pattern of patterns) {
    const open = pattern.open_groups ?? 0
    const close = pattern.close_groups ?? 0
    if (open < 0 || close < 0 || open > 8 || close > 8) return 'Use at most eight parentheses on one condition.'
    balance += open
    balance -= close
    if (balance < 0) return 'A closing parenthesis appears before its opening parenthesis.'
  }
  return balance === 0 ? '' : `${balance} closing ${balance === 1 ? 'parenthesis is' : 'parentheses are'} missing.`
}

export function removeSearchPatternAt(patterns: Pattern[], index: number): Pattern[] {
  if (index < 0 || index >= patterns.length) return patterns
  const removed = patterns[index]
  const remaining = patterns.filter((_, patternIndex) => patternIndex !== index).map((pattern) => ({ ...pattern }))
  if (remaining.length === 0) return []
  if ((removed.open_groups ?? 0) > 0 && index < remaining.length) {
    remaining[index].open_groups = (remaining[index].open_groups ?? 0) + (removed.open_groups ?? 0)
  }
  if ((removed.close_groups ?? 0) > 0 && index > 0) {
    remaining[index - 1].close_groups = (remaining[index - 1].close_groups ?? 0) + (removed.close_groups ?? 0)
  }
  return remaining.map((pattern, patternIndex) => patternIndex === 0
    ? { ...pattern, join: undefined }
    : pattern)
}

export function searchPatternText(pattern: Pattern, index: number): string {
  const join = index > 0 ? `${(pattern.join ?? 'or').toUpperCase()} ` : ''
  const open = '('.repeat(pattern.open_groups ?? 0)
  const close = ')'.repeat(pattern.close_groups ?? 0)
  return `${join}${open}${pattern.source.toUpperCase()}: ${pattern.value}${close}`
}

export function searchExpressionText(patterns: Pattern[]): string {
  return patterns.map(searchPatternText).join(' ')
}

export function groupSearchPatternRange(patterns: Pattern[], start: number, end: number): Pattern[] {
  if (start < 0 || end >= patterns.length || start >= end) return patterns
  return patterns.map((pattern, index) => {
    if (index === start) return { ...pattern, open_groups: (pattern.open_groups ?? 0) + 1 }
    if (index === end) return { ...pattern, close_groups: (pattern.close_groups ?? 0) + 1 }
    return { ...pattern }
  })
}

export function ungroupSearchPatternRange(patterns: Pattern[], start: number, end: number): Pattern[] {
  if (start < 0 || end >= patterns.length || start >= end) return patterns
  if ((patterns[start].open_groups ?? 0) === 0 || (patterns[end].close_groups ?? 0) === 0) return patterns
  return patterns.map((pattern, index) => {
    if (index === start) {
      const open = (pattern.open_groups ?? 0) - 1
      const { open_groups: _, ...withoutOpen } = pattern
      return open > 0 ? { ...withoutOpen, open_groups: open } : withoutOpen
    }
    if (index === end) {
      const close = (pattern.close_groups ?? 0) - 1
      const { close_groups: _, ...withoutClose } = pattern
      return close > 0 ? { ...withoutClose, close_groups: close } : withoutClose
    }
    return { ...pattern }
  })
}

export function searchPatternDepths(patterns: Pattern[]): number[] {
  let depth = 0
  return patterns.map((pattern) => {
    depth += pattern.open_groups ?? 0
    const rowDepth = depth
    depth = Math.max(0, depth - (pattern.close_groups ?? 0))
    return rowDepth
  })
}
