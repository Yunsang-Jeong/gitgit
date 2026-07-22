import type { Pattern, PatternSource, SearchPatternJoin } from './types'

export interface SearchExpressionParseResult {
  patterns: Pattern[]
  error: string
}

interface SearchExpressionJoin {
  end: number
  index: number
  join: SearchPatternJoin
}

const sourceExpression = /^(MSG|MESSAGE|DIFF|FILE)\s*:/i

function patternSource(value: string): PatternSource {
  return value.toLocaleLowerCase() === 'message' || value.toLocaleLowerCase() === 'msg'
    ? 'msg'
    : value.toLocaleLowerCase() as PatternSource
}

function skipWhitespace(expression: string, index: number): number {
  while (index < expression.length && /\s/u.test(expression[index])) index++
  return index
}

function nextSearchExpressionJoin(expression: string, start: number): SearchExpressionJoin | null {
  let quote = ''
  let escaped = false
  for (let index = start; index < expression.length; index++) {
    const character = expression[index]
    if (quote) {
      if (escaped) escaped = false
      else if (character === '\\') escaped = true
      else if (character === quote) quote = ''
      continue
    }
    if (character === '"' || character === "'") {
      quote = character
      continue
    }
    if (!/\s/u.test(character)) continue
    const operator = expression.slice(index).match(/^\s+(AND|OR)\b/i)
    if (!operator) continue
    let next = skipWhitespace(expression, index + operator[0].length)
    while (expression[next] === '(') next = skipWhitespace(expression, next + 1)
    if (!sourceExpression.test(expression.slice(next))) continue
    return {
      index,
      end: index + operator[0].length,
      join: operator[1].toLocaleLowerCase() as SearchPatternJoin,
    }
  }
  return null
}

function unquoteSearchValue(value: string): { value: string, error: string } {
  const quote = value[0]
  if (quote !== '"' && quote !== "'") return { value, error: '' }
  if (value.length < 2 || value[value.length - 1] !== quote) {
    return { value, error: `Close the ${quote} quoted pattern.` }
  }
  const inner = value.slice(1, -1)
  return {
    value: inner.replace(new RegExp(`\\\\([\\\\${quote}])`, 'gu'), '$1'),
    error: '',
  }
}

export function parseSearchExpression(expression: string): SearchExpressionParseResult {
  if (!expression.trim()) return { patterns: [], error: '' }

  const patterns: Pattern[] = []
  let cursor = 0
  let balance = 0
  let join: SearchPatternJoin | undefined

  while (cursor < expression.length) {
    cursor = skipWhitespace(expression, cursor)
    let openGroups = 0
    while (expression[cursor] === '(') {
      openGroups++
      cursor = skipWhitespace(expression, cursor + 1)
    }
    if (openGroups > 8) return { patterns, error: 'Use at most eight parentheses before one condition.' }
    balance += openGroups

    const sourceMatch = expression.slice(cursor).match(sourceExpression)
    if (!sourceMatch) {
      const sourceWithoutColon = expression.slice(cursor).match(/^(MSG|MESSAGE|DIFF|FILE)\b/i)
      if (sourceWithoutColon) {
        return { patterns, error: `Add : after ${sourceWithoutColon[1].toUpperCase()}.` }
      }
      return { patterns, error: 'Expected MSG:, DIFF:, or FILE: at the start of a condition.' }
    }
    const source = patternSource(sourceMatch[1])
    cursor += sourceMatch[0].length

    const nextJoin = nextSearchExpressionJoin(expression, cursor)
    let valueEnd = nextJoin?.index ?? expression.length
    while (valueEnd > cursor && /\s/u.test(expression[valueEnd - 1])) valueEnd--

    let closeGroups = 0
    while (closeGroups < balance && expression[valueEnd - 1] === ')') {
      closeGroups++
      valueEnd--
      while (valueEnd > cursor && /\s/u.test(expression[valueEnd - 1])) valueEnd--
    }

    const rawValue = expression.slice(cursor, valueEnd).trim()
    if (!rawValue) return { patterns, error: `Enter a pattern after ${source.toUpperCase()}:` }
    const danglingJoin = rawValue.match(/^(.+?)\s+(AND|OR)\s*(?:\(\s*)*$/i)
    if (danglingJoin) {
      return { patterns, error: `Add MSG:, DIFF:, or FILE: after ${danglingJoin[2].toUpperCase()}.` }
    }
    const unknownSource = rawValue.match(/\s+(AND|OR)\s+([A-Z][A-Z0-9_-]*)\s*:/i)
    if (unknownSource) {
      return { patterns, error: `${unknownSource[2].toUpperCase()}: is not a source. Use MSG:, DIFF:, or FILE:.` }
    }
    const unquoted = unquoteSearchValue(rawValue)
    if (unquoted.error) return { patterns, error: unquoted.error }

    const pattern: Pattern = { source, value: unquoted.value }
    if (patterns.length > 0) pattern.join = join ?? 'or'
    if (openGroups > 0) pattern.open_groups = openGroups
    if (closeGroups > 0) pattern.close_groups = closeGroups
    patterns.push(pattern)
    balance -= closeGroups

    if (!nextJoin) break
    join = nextJoin.join
    cursor = nextJoin.end
  }

  if (balance > 0) {
    return {
      patterns,
      error: `${balance} closing ${balance === 1 ? 'parenthesis is' : 'parentheses are'} missing.`,
    }
  }
  return { patterns, error: '' }
}

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
