import { parseCanonicalWebRemote } from '../shared/web-remotes.js'

export const PROTOCOL_VERSION = 1 as const
export const PROTOCOL_TRANSPORT = 'ndjson-jsonrpc-2.0-stdio' as const
export const PROTOCOL_LINE_BASE = 1 as const
export const MAX_OUTPUT_BYTES = 8 * 1024 * 1024

export const REQUIRED_METHODS = [
  'initialize',
  'repository.discover',
  'refs.list',
  'blame.lines',
  'history.file',
  'search.run',
  'revision.content',
  'diff.file',
] as const

export const REQUIRED_NOTIFICATIONS = [
  '$/cancelRequest',
  '$/progress',
] as const

export type HelperMethod = typeof REQUIRED_METHODS[number]

export interface InitializeParams {
  protocolVersion: typeof PROTOCOL_VERSION
  client: {
    name: 'gitgit-vscode'
    version: string
  }
}

export interface HelperCapabilities {
  readOnly: true
  network: false
  lineBase: typeof PROTOCOL_LINE_BASE
  methods: string[]
  notifications: string[]
  maxOutputBytes: number
}

export interface InitializeResult extends HelperCapabilities {
  protocolVersion: typeof PROTOCOL_VERSION
  transport: typeof PROTOCOL_TRANSPORT
  serverName: string
  serverVersion: string
}

export interface RepositoryDiscovery {
  root: string
  commonDir: string
  gitDir: string
  branch: string
  head: string
  webRemotes?: string[]
}

export interface BlameLine {
  line: number
  originalLine?: number
  commit: string
  author: { name: string, email?: string }
  date: string
  message: string
  content?: string
}

export interface BlameLinesRequest {
  repositoryRoot: string
  relativePath: string
  startLine: number
  endLine: number
  revision?: string
  contents?: string
  ignoreWhitespace?: boolean
}

export interface BlameLinesResponse {
  lines: BlameLine[]
  commitMetadata: Record<string, BlameCommitMetadata>
}

export interface BlameCommitMetadata {
  message: string
  parentCount: number
}

export interface HistoryFileChange {
  status: string
  path: string
  oldPath?: string
}

export interface HistoryCommit {
  commit: string
  shortCommit: string
  message: string
  date: string
  author: { name: string, email?: string }
  parents: string[]
  files: HistoryFileChange[]
}

export interface HistoryFileRequest {
  repositoryRoot: string
  relativePath: string
  revision?: string
  limit?: number
}

export interface HistoryFileResponse {
  commits: HistoryCommit[]
}

export interface DiffFileRequest {
  repositoryRoot: string
  commit: string
  path: string
  oldPath?: string
  context?: number
}

export interface RevisionContentRequest {
  repositoryRoot: string
  revision: string
  relativePath: string
}

export interface RevisionContentResponse {
  content: string
}

export interface DiffFileResponse {
  parent: string
  diff: string
  targetRanges: Array<{ startLine: number, endLine: number }>
  deletionAnchors: Array<{ anchorLine: number }>
}

export type SearchPatternSource = 'msg' | 'diff' | 'file'
export type SearchPatternJoin = 'and' | 'or'
export type SearchEngine = 'glob' | 'regex'

export interface SearchWirePattern {
  source: SearchPatternSource
  value: string
  join?: SearchPatternJoin
  openGroups?: number
  closeGroups?: number
}

export interface SearchRequest {
  repositoryRoot: string
  requestId: number
  patterns: SearchWirePattern[]
  engine: SearchEngine
  scope: string
  allRefs: boolean
  author: string
  since: string
  until: string
  followRename: boolean
  limit: number
  context: number
}

export interface SearchProgress {
  requestId: number
  scanned: number
  total: number
}

export interface SearchAuthor {
  name: string
  email?: string
}

export interface SearchFile {
  status: string
  path: string
  oldPath?: string
}

export interface SearchMatchedFile extends SearchFile {
  matchSources: Array<'diff' | 'file'>
}

export interface SearchResult {
  commit: string
  shortCommit: string
  message: string
  author: SearchAuthor
  date: string
  refs: string[]
  matchedFiles: SearchMatchedFile[]
  changedFiles: SearchFile[]
  matchSources: SearchPatternSource[]
}

export interface SearchResponse {
  scope: string
  allRefs: boolean
  scanned: number
  count: number
  hasMore: boolean
  results: SearchResult[]
}

export interface JsonRpcResponse {
  jsonrpc: '2.0'
  id: number
  result?: unknown
  error?: {
    code: number
    message: string
    data: {
      code: string
    }
  }
}

export interface JsonRpcProgressNotification {
  jsonrpc: '2.0'
  method: '$/progress'
  params: {
    requestId: number
    scanned: number
    total: number
  }
}

export class HelperProtocolError extends Error {
  readonly stableCode: string

  constructor(message: string, stableCode = 'protocol_error') {
    super(message)
    this.name = 'HelperProtocolError'
    this.stableCode = stableCode
  }
}

function record(value: unknown, description: string): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new HelperProtocolError(`GitGit helper returned invalid ${description}.`, 'invalid_response')
  }
  return value as Record<string, unknown>
}

function capability(result: Record<string, unknown>, nested: Record<string, unknown>, key: string): unknown {
  return nested[key] ?? result[key]
}

function exactAllowlist(value: unknown, expected: readonly string[], description: string): string[] {
  if (!Array.isArray(value) || value.some((entry) => typeof entry !== 'string')) {
    throw new HelperProtocolError(`GitGit helper returned an invalid ${description} allowlist.`, 'invalid_capabilities')
  }
  const actual = value as string[]
  const allowed = new Set(expected)
  if (actual.length !== allowed.size || new Set(actual).size !== allowed.size || actual.some((entry) => !allowed.has(entry))) {
    throw new HelperProtocolError(`GitGit helper ${description} allowlist does not match protocol v1.`, 'unsafe_capabilities')
  }
  return [...actual]
}

export function validateInitializeResult(value: unknown): InitializeResult {
  const result = record(value, 'initialize result')
  const nested = result.capabilities === undefined
    ? {}
    : record(result.capabilities, 'initialize capabilities')
  const readOnly = capability(result, nested, 'readOnly')
  const network = capability(result, nested, 'network')
  const lineBase = capability(result, nested, 'lineBase')
  const maxOutputBytes = capability(result, nested, 'maxOutputBytes')
  const methods = exactAllowlist(capability(result, nested, 'methods'), REQUIRED_METHODS, 'method')
  const notifications = exactAllowlist(
    capability(result, nested, 'notifications'),
    REQUIRED_NOTIFICATIONS,
    'notification',
  )

  if (result.protocolVersion !== PROTOCOL_VERSION) {
    throw new HelperProtocolError(`Unsupported GitGit helper protocol ${String(result.protocolVersion)}.`, 'protocol_version_mismatch')
  }
  if (result.transport !== PROTOCOL_TRANSPORT) {
    throw new HelperProtocolError('GitGit helper transport does not match protocol v1.', 'transport_mismatch')
  }
  if (readOnly !== true || network !== false) {
    throw new HelperProtocolError('GitGit helper is not read-only and offline.', 'unsafe_capabilities')
  }
  if (lineBase !== PROTOCOL_LINE_BASE) {
    throw new HelperProtocolError('GitGit helper does not use 1-based line ranges.', 'line_base_mismatch')
  }
  if (maxOutputBytes !== MAX_OUTPUT_BYTES) {
    throw new HelperProtocolError('GitGit helper output bound does not match protocol v1.', 'output_limit_mismatch')
  }

  const server = result.server === undefined ? {} : record(result.server, 'server metadata')
  return {
    protocolVersion: PROTOCOL_VERSION,
    transport: PROTOCOL_TRANSPORT,
    serverName: typeof server.name === 'string' ? server.name : 'gitgit-vscode-helper',
    serverVersion: typeof server.version === 'string' ? server.version : 'unknown',
    readOnly: true,
    network: false,
    lineBase: PROTOCOL_LINE_BASE,
    methods,
    notifications,
    maxOutputBytes: MAX_OUTPUT_BYTES,
  }
}

export function parseRepositoryDiscovery(value: unknown): RepositoryDiscovery {
  const result = record(value, 'repository discovery')
  for (const key of ['root', 'commonDir', 'gitDir', 'branch', 'head'] as const) {
    if (typeof result[key] !== 'string') {
      throw new HelperProtocolError(`repository.discover omitted ${key}.`, 'invalid_repository')
    }
  }
  const webRemotes = result.webRemotes
  if (webRemotes !== undefined
    && (!Array.isArray(webRemotes)
      || webRemotes.some((remote) => !parseCanonicalWebRemote(remote))
      || new Set(webRemotes).size !== webRemotes.length)) {
    throw new HelperProtocolError('repository.discover returned invalid webRemotes.', 'invalid_repository')
  }
  return {
    root: result.root as string,
    commonDir: result.commonDir as string,
    gitDir: result.gitDir as string,
    branch: result.branch as string,
    head: result.head as string,
    ...(Array.isArray(webRemotes) && webRemotes.length > 0 ? { webRemotes: [...webRemotes] as string[] } : {}),
  }
}

function optionalString(value: unknown, description: string): string | undefined {
  if (value === undefined || value === '') return undefined
  if (typeof value !== 'string') {
    throw new HelperProtocolError(`GitGit helper returned invalid ${description}.`, 'invalid_response')
  }
  return value
}

function parseAuthor(value: unknown, description: string): { name: string, email?: string } {
  const author = record(value, description)
  if (typeof author.name !== 'string') {
    throw new HelperProtocolError(`GitGit helper returned invalid ${description} name.`, 'invalid_response')
  }
  const email = optionalString(author.email, `${description} email`)
  return { name: author.name, ...(email ? { email } : {}) }
}

export function parseBlameLinesResponse(value: unknown): BlameLinesResponse {
  const response = record(value, 'blame.lines result')
  if (!Array.isArray(response.lines)) {
    throw new HelperProtocolError('blame.lines omitted lines.', 'invalid_blame_response')
  }
  const lines = response.lines.map((entry) => {
      const line = record(entry, 'blame line')
      const message = line.message ?? line.summary
      if (!Number.isSafeInteger(line.line) || (line.line as number) < 1
        || typeof line.commit !== 'string' || !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u.test(line.commit)
        || typeof line.date !== 'string'
        || typeof message !== 'string') {
        throw new HelperProtocolError('blame.lines returned invalid line metadata.', 'invalid_blame_response')
      }
      const originalLine = line.originalLine
      if (originalLine !== undefined && (!Number.isSafeInteger(originalLine) || (originalLine as number) < 1)) {
        throw new HelperProtocolError('blame.lines returned an invalid originalLine.', 'invalid_blame_response')
      }
      const content = optionalString(line.content, 'blame line content')
      return {
        line: line.line as number,
        ...(originalLine === undefined ? {} : { originalLine: originalLine as number }),
        commit: line.commit,
        author: parseAuthor(line.author, 'blame author'),
        date: line.date,
        message,
        ...(content ? { content } : {}),
      }
    })
  const lineCommits = new Set(lines.map((line) => line.commit))
  const commitMetadataValue = response.commitMetadata ?? {}
  const commitMetadataRecord = record(commitMetadataValue, 'blame commit metadata')
  const commitMetadata: Record<string, BlameCommitMetadata> = {}
  for (const [commit, value] of Object.entries(commitMetadataRecord)) {
    const metadata = record(value, 'blame commit metadata entry')
    if (!lineCommits.has(commit)
      || typeof metadata.message !== 'string'
      || !Number.isSafeInteger(metadata.parentCount)
      || (metadata.parentCount as number) < 0) {
      throw new HelperProtocolError('blame.lines returned invalid commit metadata.', 'invalid_blame_response')
    }
    if ((metadata.parentCount as number) > 64) continue
    commitMetadata[commit] = {
      message: metadata.message,
      parentCount: metadata.parentCount as number,
    }
  }
  return {
    commitMetadata,
    lines,
  }
}

function parseHistoryFileChange(value: unknown): HistoryFileChange {
  const file = record(value, 'history file change')
  const oldPath = file.oldPath ?? file.old_path
  if (typeof file.status !== 'string' || typeof file.path !== 'string') {
    throw new HelperProtocolError('history.file returned invalid file metadata.', 'invalid_history_response')
  }
  const parsedOldPath = optionalString(oldPath, 'history oldPath')
  return { status: file.status, path: file.path, ...(parsedOldPath ? { oldPath: parsedOldPath } : {}) }
}

export function parseHistoryFileResponse(value: unknown): HistoryFileResponse {
  const response = record(value, 'history.file result')
  if (!Array.isArray(response.commits)) {
    throw new HelperProtocolError('history.file omitted commits.', 'invalid_history_response')
  }
  return {
    commits: response.commits.map((entry) => {
      const commit = record(entry, 'history commit')
      const shortCommit = commit.shortCommit ?? commit.short_commit
      if (typeof commit.commit !== 'string' || typeof shortCommit !== 'string'
        || typeof commit.message !== 'string' || typeof commit.date !== 'string'
        || !Array.isArray(commit.parents) || commit.parents.some((parent) => typeof parent !== 'string')
        || !Array.isArray(commit.files)) {
        throw new HelperProtocolError('history.file returned invalid commit metadata.', 'invalid_history_response')
      }
      return {
        commit: commit.commit,
        shortCommit,
        message: commit.message,
        date: commit.date,
        author: parseAuthor(commit.author, 'history author'),
        parents: [...commit.parents] as string[],
        files: commit.files.map(parseHistoryFileChange),
      }
    }),
  }
}

export function parseRevisionContentResponse(value: unknown): RevisionContentResponse {
  const response = record(value, 'revision.content result')
  if (typeof response.content !== 'string') {
    throw new HelperProtocolError('revision.content omitted content.', 'invalid_revision_content_response')
  }
  return { content: response.content }
}

export function parseDiffFileResponse(value: unknown): DiffFileResponse {
  const response = record(value, 'diff.file result')
  if (typeof response.parent !== 'string' || typeof response.diff !== 'string'
    || !Array.isArray(response.targetRanges) || !Array.isArray(response.deletionAnchors)) {
    throw new HelperProtocolError('diff.file returned invalid result metadata.', 'invalid_diff_response')
  }
  const targetRanges = response.targetRanges.map((entry) => {
    const range = record(entry, 'diff target range')
    if (!Number.isSafeInteger(range.startLine) || !Number.isSafeInteger(range.endLine)
      || (range.startLine as number) < 1 || (range.endLine as number) < (range.startLine as number)) {
      throw new HelperProtocolError('diff.file returned an invalid target range.', 'invalid_diff_response')
    }
    return { startLine: range.startLine as number, endLine: range.endLine as number }
  })
  const deletionAnchors = response.deletionAnchors.map((entry) => {
    const anchor = record(entry, 'diff deletion anchor')
    if (!Number.isSafeInteger(anchor.anchorLine) || (anchor.anchorLine as number) < 1) {
      throw new HelperProtocolError('diff.file returned an invalid deletion anchor.', 'invalid_diff_response')
    }
    return { anchorLine: anchor.anchorLine as number }
  })
  return { parent: response.parent, diff: response.diff, targetRanges, deletionAnchors }
}

function stringField(result: Record<string, unknown>, key: string, alias?: string): string {
  const value = result[key] ?? (alias ? result[alias] : undefined)
  if (typeof value !== 'string') {
    throw new HelperProtocolError(`search.run returned invalid ${key}.`, 'invalid_search_response')
  }
  return value
}

function integerField(result: Record<string, unknown>, key: string): number {
  const value = result[key]
  if (!Number.isSafeInteger(value) || (value as number) < 0) {
    throw new HelperProtocolError(`search.run returned invalid ${key}.`, 'invalid_search_response')
  }
  return value as number
}

function parseSearchFile(value: unknown): SearchFile {
  const file = record(value, 'Search file')
  const oldPath = file.oldPath ?? file.old_path
  if (oldPath !== undefined && typeof oldPath !== 'string') {
    throw new HelperProtocolError('search.run returned invalid oldPath.', 'invalid_search_response')
  }
  return {
    status: stringField(file, 'status'),
    path: stringField(file, 'path'),
    ...(oldPath ? { oldPath } : {}),
  }
}

function parseSearchMatchedFile(value: unknown): SearchMatchedFile {
  const file = record(value, 'matched Search file')
  const parsed = parseSearchFile(file)
  const sources = file.matchSources ?? file.match_sources
  if (!Array.isArray(sources) || sources.length === 0
    || sources.some((source) => source !== 'diff' && source !== 'file')
    || new Set(sources).size !== sources.length) {
    throw new HelperProtocolError('search.run returned invalid matched file sources.', 'invalid_search_response')
  }
  return { ...parsed, matchSources: [...sources] as Array<'diff' | 'file'> }
}

function parseSearchResult(value: unknown): SearchResult {
  const result = record(value, 'Search result')
  const author = record(result.author, 'Search author')
  const matchedFilesValue = result.matchedFiles ?? result.matched_files
  const changedFilesValue = result.changedFiles ?? result.changed_files
  const sourcesValue = result.matchSources ?? result.match_sources
  const refsValue = result.refs ?? []
  if (!Array.isArray(matchedFilesValue) || !Array.isArray(changedFilesValue)
    || !Array.isArray(sourcesValue) || !Array.isArray(refsValue)) {
    throw new HelperProtocolError('search.run returned invalid result arrays.', 'invalid_search_response')
  }
  if (sourcesValue.some((source) => source !== 'msg' && source !== 'diff' && source !== 'file')) {
    throw new HelperProtocolError('search.run returned an invalid match source.', 'invalid_search_response')
  }
  if (refsValue.some((ref) => typeof ref !== 'string')) {
    throw new HelperProtocolError('search.run returned invalid refs.', 'invalid_search_response')
  }
  const matchedFiles = matchedFilesValue.map(parseSearchMatchedFile)
  const changedFiles = changedFilesValue.map(parseSearchFile)
  const changedKeys = new Set(changedFiles.map(searchFileKey))
  if (matchedFiles.some((file) => !changedKeys.has(searchFileKey(file)))) {
    throw new HelperProtocolError('search.run returned a matched file outside changedFiles.', 'invalid_search_response')
  }
  return {
    commit: stringField(result, 'commit'),
    shortCommit: stringField(result, 'shortCommit', 'short_commit'),
    message: stringField(result, 'message'),
    author: {
      name: stringField(author, 'name'),
      ...(typeof author.email === 'string' && author.email ? { email: author.email } : {}),
    },
    date: stringField(result, 'date'),
    refs: [...refsValue] as string[],
    matchedFiles,
    changedFiles,
    matchSources: [...sourcesValue] as SearchPatternSource[],
  }
}

function searchFileKey(file: SearchFile): string {
  return `${file.status}\u0000${file.oldPath ?? ''}\u0000${file.path}`
}

export function parseSearchResponse(value: unknown): SearchResponse {
  const result = record(value, 'Search response')
  const allRefs = result.allRefs ?? result.all_refs
  const hasMore = result.hasMore ?? result.has_more
  if (typeof allRefs !== 'boolean' || typeof hasMore !== 'boolean' || !Array.isArray(result.results)) {
    throw new HelperProtocolError('search.run returned invalid scope or results.', 'invalid_search_response')
  }
  const results = result.results.map(parseSearchResult)
  const count = integerField(result, 'count')
  if (count !== results.length) {
    throw new HelperProtocolError('search.run count does not match results.', 'invalid_search_response')
  }
  if (new Set(results.map((entry) => entry.commit)).size !== results.length) {
    throw new HelperProtocolError('search.run returned duplicate commits.', 'invalid_search_response')
  }
  return {
    scope: stringField(result, 'scope'),
    allRefs,
    scanned: integerField(result, 'scanned'),
    count,
    hasMore,
    results,
  }
}

export function parseJsonRpcMessage(value: unknown): JsonRpcResponse | JsonRpcProgressNotification {
  const message = record(value, 'JSON-RPC message')
  if (message.jsonrpc !== '2.0') {
    throw new HelperProtocolError('GitGit helper returned an invalid JSON-RPC version.', 'invalid_jsonrpc')
  }
  if (message.method !== undefined) {
    if (message.method !== '$/progress') {
      throw new HelperProtocolError('GitGit helper returned an unknown notification.', 'unknown_notification')
    }
    const params = record(message.params, 'progress notification')
    if (![params.requestId, params.scanned, params.total].every(Number.isSafeInteger)) {
      throw new HelperProtocolError('GitGit helper returned invalid progress values.', 'invalid_progress')
    }
    return message as unknown as JsonRpcProgressNotification
  }
  if (!Number.isSafeInteger(message.id)) {
    throw new HelperProtocolError('GitGit helper returned an invalid request id.', 'invalid_request_id')
  }
  if (message.error !== undefined) {
    const error = record(message.error, 'JSON-RPC error')
    const data = record(error.data, 'JSON-RPC error data')
    if (!Number.isSafeInteger(error.code) || typeof error.message !== 'string' || typeof data.code !== 'string' || !data.code) {
      throw new HelperProtocolError('GitGit helper returned an invalid stable error.', 'invalid_error')
    }
  }
  return message as unknown as JsonRpcResponse
}
