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
  parseBlameLinesResponse,
  parseRevisionContentResponse,
  parseSearchResponse,
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
  methodContracts: Record<string, ProtocolMethodContract>
}

interface ProtocolMethodContract {
  requiredParams: string[]
  optionalParams?: string[]
  requiredResult: string[]
  optionalResult?: string[]
  requiredResultItem?: string[]
  requiredMatchedFile?: string[]
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
  assert.deepEqual(fixture.methodContracts['search.run'], {
    requiredParams: ['repositoryRoot', 'patterns'],
    optionalParams: ['engine', 'scope', 'allRefs', 'author', 'since', 'until', 'followRename', 'limit', 'context'],
    requiredResult: ['scope', 'allRefs', 'scanned', 'count', 'hasMore', 'results'],
    requiredResultItem: ['author', 'commit', 'shortCommit', 'message', 'date', 'matchedFiles', 'changedFiles', 'matchSources'],
    requiredMatchedFile: ['status', 'path', 'matchSources'],
  })
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

  assert.deepEqual(parseRepositoryDiscovery({
    root: '/repo',
    commonDir: '/repo/.git',
    gitDir: '/repo/.git',
    branch: 'main',
    head: 'abc123',
    webRemotes: ['https://github.com/acme/repo'],
  }).webRemotes, ['https://github.com/acme/repo'])
  assert.throws(() => parseRepositoryDiscovery({
    root: '/repo', commonDir: '/repo/.git', gitDir: '/repo/.git', branch: 'main', head: 'abc123',
    webRemotes: ['https://github.com/acme/repo', 1],
  }), /webRemotes/)
  assert.throws(() => parseRepositoryDiscovery({
    root: '/repo', commonDir: '/repo/.git', gitDir: '/repo/.git', branch: 'main', head: 'abc123',
    webRemotes: ['https://evil).example/acme/repo'],
  }), /webRemotes/)
  assert.throws(() => parseRepositoryDiscovery({
    root: '/repo', commonDir: '/repo/.git', gitDir: '/repo/.git', branch: 'main', head: 'abc123',
    webRemotes: ['https://example.test/acme/repo)'],
  }), /webRemotes/)
  assert.throws(() => parseRepositoryDiscovery({
    root: '/repo', commonDir: '/repo/.git', gitDir: '/repo/.git', branch: 'main', head: 'abc123',
    webRemotes: ['https://github.com/acme/repo?token=secret'],
  }), /webRemotes/)
})

test('blame response keeps commit metadata deduplicated by object id', () => {
  const commit = 'a'.repeat(40)
  assert.deepEqual(parseBlameLinesResponse({
    lines: [{
      line: 1,
      originalLine: 1,
      commit,
      author: { name: 'Test' },
      date: '2026-08-15T00:00:00Z',
      message: 'Merge branch feature',
      content: 'line',
    }],
    commitMetadata: {
      [commit]: { message: 'Merge branch feature\n\nSee merge request acme/repo!12\n', parentCount: 2 },
    },
  }), {
    lines: [{
      line: 1,
      originalLine: 1,
      commit,
      author: { name: 'Test' },
      date: '2026-08-15T00:00:00Z',
      message: 'Merge branch feature',
      content: 'line',
    }],
    commitMetadata: {
      [commit]: { message: 'Merge branch feature\n\nSee merge request acme/repo!12\n', parentCount: 2 },
    },
  })
  assert.throws(() => parseBlameLinesResponse({ lines: [], commitMetadata: { [commit]: 12 } }), /metadata/)
  assert.throws(() => parseBlameLinesResponse({
    lines: [], commitMetadata: { [commit]: { message: 'unrelated', parentCount: 1 } },
  }), /metadata/)

  const unusualMerge = parseBlameLinesResponse({
    lines: [{ line: 1, commit, author: { name: 'Ada' }, date: '2026-08-15', message: 'subject' }],
    commitMetadata: { [commit]: { message: 'octopus merge', parentCount: 65 } },
  })
  assert.deepEqual(unusualMerge.lines.map((line) => line.commit), [commit])
  assert.deepEqual(unusualMerge.commitMetadata, {})
})

test('revision content requires an explicit text payload', () => {
  assert.deepEqual(parseRevisionContentResponse({ content: 'first\nsecond\n' }), {
    content: 'first\nsecond\n',
  })
  assert.throws(() => parseRevisionContentResponse({}), /omitted content/)
  assert.throws(() => parseRevisionContentResponse({ content: new Uint8Array() }), /omitted content/)
})

test('Search response keeps commit-first matched and changed files separate', () => {
  const commit = 'a'.repeat(40)
  assert.deepEqual(parseSearchResponse({
    scope: 'HEAD',
    allRefs: false,
    scanned: 12,
    count: 1,
    hasMore: true,
    results: [{
      commit,
      shortCommit: 'aaaaaaa',
      message: 'feat: compact Search',
      author: { name: 'Ada' },
      date: '2026-08-15T00:00:00Z',
      refs: ['main'],
      matchedFiles: [{ status: 'M', path: 'search.ts', matchSources: ['diff', 'file'] }],
      changedFiles: [
        { status: 'M', path: 'search.ts' },
        { status: 'A', path: 'search.test.ts' },
      ],
      matchSources: ['msg', 'diff', 'file'],
    }],
  }), {
    scope: 'HEAD',
    allRefs: false,
    scanned: 12,
    count: 1,
    hasMore: true,
    results: [{
      commit,
      shortCommit: 'aaaaaaa',
      message: 'feat: compact Search',
      author: { name: 'Ada' },
      date: '2026-08-15T00:00:00Z',
      refs: ['main'],
      matchedFiles: [{ status: 'M', path: 'search.ts', matchSources: ['diff', 'file'] }],
      changedFiles: [
        { status: 'M', path: 'search.ts' },
        { status: 'A', path: 'search.test.ts' },
      ],
      matchSources: ['msg', 'diff', 'file'],
    }],
  })
})

test('Search response rejects ambiguous file matches and legacy row payloads', () => {
  const base = {
    scope: 'HEAD', allRefs: false, scanned: 1, count: 1, hasMore: false,
    results: [{
      commit: 'a'.repeat(40), shortCommit: 'aaaaaaa', message: 'subject',
      author: { name: 'Ada' }, date: '2026-08-15T00:00:00Z', refs: [],
      matchedFiles: [{ status: 'M', path: 'other.ts', matchSources: ['diff'] }],
      changedFiles: [{ status: 'M', path: 'search.ts' }], matchSources: ['diff'],
    }],
  }
  assert.throws(() => parseSearchResponse(base), /outside changedFiles/)
  assert.throws(() => parseSearchResponse({
    ...base,
    results: [{ ...base.results[0], matchedFiles: [{ status: 'M', path: 'search.ts', matchSources: ['msg'] }] }],
  }), /matched file sources/)
  assert.throws(() => parseSearchResponse({ ...base, hasMore: undefined }), /scope or results/)
  assert.throws(() => parseSearchResponse({
    ...base,
    results: [{ ...base.results[0], matchedFiles: undefined, changedFiles: undefined, file: {}, files: [] }],
  }), /result arrays/)
  const validResult = {
    ...base.results[0],
    matchedFiles: [{ status: 'M', path: 'search.ts', matchSources: ['diff'] }],
  }
  assert.throws(() => parseSearchResponse({
    ...base,
    count: 2,
    results: [validResult, { ...validResult }],
  }), /duplicate commits/)
})
