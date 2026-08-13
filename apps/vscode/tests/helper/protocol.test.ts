import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import * as path from 'node:path'
import test from 'node:test'

import {
  MAX_OUTPUT_BYTES,
  PROTOCOL_LINE_BASE,
  PROTOCOL_TRANSPORT,
  PROTOCOL_VERSION,
  REQUIRED_METHODS,
  REQUIRED_NOTIFICATIONS,
  parseRepositoryDiscovery,
  parseRevisionContentResponse,
  validateInitializeResult,
} from '../../src/helper/protocol.js'

interface ProtocolManifest {
  protocolVersion: number
  transport: string
  lineBase: number
  readOnly: boolean
  network: boolean
  maxOutputBytes: number
  methods: string[]
  notifications: string[]
}

async function readFixture<T>(relativePath: string): Promise<T> {
  const repositoryRoot = path.resolve(__dirname, '../../../../..')
  return JSON.parse(await readFile(path.join(repositoryRoot, relativePath), 'utf8')) as T
}

test('protocol constants match the canonical manifest', async () => {
  const fixture = await readFixture<ProtocolManifest>('testdata/vscode-contract/protocol-v1.json')
  assert.equal(PROTOCOL_VERSION, fixture.protocolVersion)
  assert.equal(PROTOCOL_TRANSPORT, fixture.transport)
  assert.equal(PROTOCOL_LINE_BASE, fixture.lineBase)
  assert.equal(MAX_OUTPUT_BYTES, fixture.maxOutputBytes)
  assert.deepEqual([...REQUIRED_METHODS], fixture.methods)
  assert.deepEqual([...REQUIRED_NOTIFICATIONS], fixture.notifications)
})

test('initialize validates flat and nested read-only capabilities', async () => {
  const fixture = await readFixture<ProtocolManifest>('testdata/vscode-contract/protocol-v1.json')
  const common = {
    protocolVersion: fixture.protocolVersion,
    transport: fixture.transport,
    server: { name: 'gitgit-vscode-helper', version: '0.1.0' },
  }
  const flat = validateInitializeResult({
    ...common,
    readOnly: fixture.readOnly,
    network: fixture.network,
    lineBase: fixture.lineBase,
    maxOutputBytes: fixture.maxOutputBytes,
    methods: fixture.methods,
    notifications: fixture.notifications,
  })
  const nested = validateInitializeResult({
    ...common,
    capabilities: {
      readOnly: fixture.readOnly,
      network: fixture.network,
      lineBase: fixture.lineBase,
      maxOutputBytes: fixture.maxOutputBytes,
      methods: fixture.methods,
      notifications: fixture.notifications,
    },
  })

  assert.equal(flat.serverVersion, '0.1.0')
  assert.deepEqual(nested, flat)
})

test('initialize rejects expanded helper capabilities', async () => {
  const fixture = await readFixture<ProtocolManifest>('testdata/vscode-contract/protocol-v1.json')
  assert.throws(() => validateInitializeResult({
    protocolVersion: fixture.protocolVersion,
    readOnly: true,
    network: false,
    lineBase: fixture.lineBase,
    maxOutputBytes: fixture.maxOutputBytes,
    methods: [...fixture.methods, 'git.raw'],
    notifications: fixture.notifications,
  }), /allowlist/)
  assert.throws(() => validateInitializeResult({
    protocolVersion: fixture.protocolVersion,
    transport: fixture.transport,
    readOnly: true,
    network: false,
    lineBase: fixture.lineBase,
    maxOutputBytes: fixture.maxOutputBytes,
    methods: fixture.methods,
  }), /notification allowlist/)
})

test('repository discovery requires every protocol field', () => {
  assert.deepEqual(parseRepositoryDiscovery({
    root: '/repo',
    commonDir: '/repo/.git',
    gitDir: '/repo/.git',
    branch: 'main',
    head: 'abc123',
  }), {
    root: '/repo',
    commonDir: '/repo/.git',
    gitDir: '/repo/.git',
    branch: 'main',
    head: 'abc123',
  })
  assert.throws(() => parseRepositoryDiscovery({ root: '/repo' }), /commonDir/)
})

test('revision content requires an explicit text payload', () => {
  assert.deepEqual(parseRevisionContentResponse({ content: 'first\nsecond\n' }), {
    content: 'first\nsecond\n',
  })
  assert.throws(() => parseRevisionContentResponse({}), /omitted content/)
  assert.throws(() => parseRevisionContentResponse({ content: new Uint8Array() }), /omitted content/)
})
