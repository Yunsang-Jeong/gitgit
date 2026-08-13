export type SearchPatternSource = 'msg' | 'diff' | 'file'
export type SearchPatternJoin = 'and' | 'or'

export interface SearchPattern {
  source: SearchPatternSource
  value: string
  join?: SearchPatternJoin
  open_groups?: number
  close_groups?: number
}

export interface SearchExpressionParseResult {
  patterns: SearchPattern[]
  error: string
}

interface SearchExpressionJoin {
  end: number
  index: number
  join: SearchPatternJoin
}

const sourceExpression = /^(MSG|MESSAGE|DIFF|FILE)\s*:/i

function patternSource(value: string): SearchPatternSource {
  return value.toLocaleLowerCase() === 'message' || value.toLocaleLowerCase() === 'msg'
    ? 'msg'
    : value.toLocaleLowerCase() as SearchPatternSource
}

function skipWhitespace(expression: string, index: number): number {
  while (index < expression.length && /\s/u.test(expression[index] ?? '')) index++
  return index
}

function nextSearchExpressionJoin(expression: string, start: number): SearchExpressionJoin | null {
  let quote = ''
  let escaped = false
  for (let index = start; index < expression.length; index++) {
    const character = expression[index] ?? ''
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
      join: operator[1]?.toLocaleLowerCase() as SearchPatternJoin,
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

  const patterns: SearchPattern[] = []
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
        return { patterns, error: `Add : after ${sourceWithoutColon[1]?.toUpperCase()}.` }
      }
      return { patterns, error: 'Expected MSG:, DIFF:, or FILE: at the start of a condition.' }
    }
    const source = patternSource(sourceMatch[1] ?? '')
    cursor += sourceMatch[0].length

    const nextJoin = nextSearchExpressionJoin(expression, cursor)
    let valueEnd = nextJoin?.index ?? expression.length
    while (valueEnd > cursor && /\s/u.test(expression[valueEnd - 1] ?? '')) valueEnd--

    let closeGroups = 0
    while (closeGroups < balance && expression[valueEnd - 1] === ')') {
      closeGroups++
      valueEnd--
      while (valueEnd > cursor && /\s/u.test(expression[valueEnd - 1] ?? '')) valueEnd--
    }

    const rawValue = expression.slice(cursor, valueEnd).trim()
    if (!rawValue) return { patterns, error: `Enter a pattern after ${source.toUpperCase()}:` }
    const danglingJoin = rawValue.match(/^(.+?)\s+(AND|OR)\s*(?:\(\s*)*$/i)
    if (danglingJoin) {
      return { patterns, error: `Add MSG:, DIFF:, or FILE: after ${danglingJoin[2]?.toUpperCase()}.` }
    }
    const unknownSource = rawValue.match(/\s+(AND|OR)\s+([A-Z][A-Z0-9_-]*)\s*:/i)
    if (unknownSource) {
      return { patterns, error: `${unknownSource[2]?.toUpperCase()}: is not a source. Use MSG:, DIFF:, or FILE:.` }
    }
    const unquoted = unquoteSearchValue(rawValue)
    if (unquoted.error) return { patterns, error: unquoted.error }

    const pattern: SearchPattern = { source, value: unquoted.value }
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
