import assert from 'node:assert/strict'
import test from 'node:test'

import { openDocumentIfCurrent } from '../../src/editor/revision-navigation.js'

test('stale revision document does not steal focus after a delayed open', async () => {
  let resolveOpen: ((value: string) => void) | undefined
  let current = true
  let showCalls = 0
  const pending = openDocumentIfCurrent(
    () => new Promise<string>((resolve) => { resolveOpen = resolve }),
    async () => { showCalls++ },
    () => current,
    () => current,
  )

  current = false
  resolveOpen?.('revision-a')
  assert.equal(await pending, undefined)
  assert.equal(showCalls, 0)
})

test('current revision document opens and must remain current after focus', async () => {
  const order: string[] = []
  let active = false
  const result = await openDocumentIfCurrent(
    async () => {
      order.push('open')
      return 'revision-b'
    },
    async () => {
      order.push('show')
      active = true
    },
    () => true,
    () => active,
  )

  assert.equal(result, 'revision-b')
  assert.deepEqual(order, ['open', 'show'])
})
