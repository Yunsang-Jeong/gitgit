import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import * as path from 'node:path'
import test from 'node:test'

import { parseSearchExpression, type SearchPattern } from '../../src/search/expression.js'

interface ExpressionCase {
  name: string
  expression: string
  patterns?: SearchPattern[]
  error?: string
  errorIncludes?: string
}

test('Search parser matches every shared Desktop expression case', async () => {
  const repositoryRoot = path.resolve(__dirname, '../../../../..')
  const fixturePath = path.join(repositoryRoot, 'testdata/search-contract/expression-cases.json')
  const cases = JSON.parse(await readFile(fixturePath, 'utf8')) as ExpressionCase[]

  assert.equal(cases.length, 10)
  for (const entry of cases) {
    const result = parseSearchExpression(entry.expression)
    if (entry.patterns) assert.deepEqual(result.patterns, entry.patterns, entry.name)
    if (entry.error !== undefined) assert.equal(result.error, entry.error, entry.name)
    if (entry.errorIncludes) assert.match(result.error, new RegExp(entry.errorIncludes.replace(/[.*+?^${}()|[\]\\]/gu, '\\$&')), entry.name)
  }
})
