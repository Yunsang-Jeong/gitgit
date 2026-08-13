import assert from 'node:assert/strict'
import * as path from 'node:path'
import test from 'node:test'

import {
  assertWorkspaceTrusted,
  dispatchHelperProgress,
  logicalSearchProgress,
  resolveHelperExecutable,
} from '../../src/helper/client.js'

test('Trust is rejected before helper startup', () => {
  assert.throws(() => assertWorkspaceTrusted(false), /Workspace Trust/)
  assert.doesNotThrow(() => assertWorkspaceTrusted(true))
})

test('helper resolution permits exactly the three local v0.1 targets', () => {
  assert.equal(
    resolveHelperExecutable('/extension', 'darwin', 'arm64'),
    path.join('/extension', 'dist', 'bin', 'gitgit-vscode-helper'),
  )
  assert.equal(
    resolveHelperExecutable('/extension', 'darwin', 'x64'),
    path.join('/extension', 'dist', 'bin', 'gitgit-vscode-helper'),
  )
  assert.equal(
    resolveHelperExecutable('/extension', 'win32', 'x64'),
    path.join('/extension', 'dist', 'bin', 'gitgit-vscode-helper.exe'),
  )
  assert.throws(() => resolveHelperExecutable('/extension', 'linux', 'x64'), /does not support/)
  assert.throws(() => resolveHelperExecutable('/extension', 'win32', 'arm64'), /does not support/)
})

test('helper progress routes by JSON-RPC id and translates to the Search session id', () => {
  const received: Array<{ requestId: number, scanned: number, total: number }> = []
  const pending = new Map([
    [12, { onProgress: (progress: { requestId: number, scanned: number, total: number }) => received.push(progress) }],
  ])
  assert.equal(dispatchHelperProgress(pending, { requestId: 99, scanned: 1, total: 10 }), false)
  assert.equal(dispatchHelperProgress(pending, { requestId: 12, scanned: 2, total: 10 }), true)
  assert.deepEqual(received, [{ requestId: 12, scanned: 2, total: 10 }])
  assert.deepEqual(logicalSearchProgress(7, received[0]!), { requestId: 7, scanned: 2, total: 10 })
})
