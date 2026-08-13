import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { statSync } from 'node:fs'
import * as path from 'node:path'

import type { CancellationToken, ExtensionContext } from 'vscode'

import {
  HelperProtocolError,
  MAX_OUTPUT_BYTES,
  PROTOCOL_VERSION,
  type HelperMethod,
  type InitializeParams,
  type JsonRpcProgressNotification,
  type JsonRpcResponse,
  type RepositoryDiscovery,
  type BlameLinesRequest,
  type BlameLinesResponse,
  type HistoryFileRequest,
  type HistoryFileResponse,
  type RevisionContentRequest,
  type RevisionContentResponse,
  type DiffFileRequest,
  type DiffFileResponse,
  type SearchProgress,
  type SearchRequest,
  type SearchResponse,
  parseJsonRpcMessage,
  parseRepositoryDiscovery,
  parseBlameLinesResponse,
  parseHistoryFileResponse,
  parseRevisionContentResponse,
  parseDiffFileResponse,
  parseSearchResponse,
  validateInitializeResult,
} from './protocol.js'

const INITIALIZE_TIMEOUT_MS = 5_000
const MAX_INPUT_BYTES = MAX_OUTPUT_BYTES
const MAX_STDERR_BYTES = 64 * 1024
const MAX_PENDING_REQUESTS = 64

interface PendingRequest {
  resolve(value: unknown): void
  reject(error: Error): void
  disposeCancellation(): void
  onProgress?: (progress: JsonRpcProgressNotification['params']) => void
}

interface ProgressReceiver {
  onProgress?: (progress: JsonRpcProgressNotification['params']) => void
}

export function dispatchHelperProgress(
  pending: ReadonlyMap<number, ProgressReceiver>,
  progress: JsonRpcProgressNotification['params'],
): boolean {
  const receiver = pending.get(progress.requestId)?.onProgress
  if (!receiver) return false
  receiver(progress)
  return true
}

export function logicalSearchProgress(requestId: number, progress: SearchProgress): SearchProgress {
  return { ...progress, requestId }
}

export class HelperClientError extends Error {
  readonly stableCode: string
  readonly rpcCode?: number

  constructor(message: string, stableCode = 'helper_error', rpcCode?: number) {
    super(message)
    this.name = 'HelperClientError'
    this.stableCode = stableCode
    this.rpcCode = rpcCode
  }
}

export interface HelperClientOptions {
  executablePath: string
  extensionVersion: string
  onProgress?: (progress: JsonRpcProgressNotification['params']) => void
}

interface SupportedHelper {
  platform: NodeJS.Platform
  arch: string
  executable: string
}

const SUPPORTED_HELPERS: readonly SupportedHelper[] = [
  { platform: 'darwin', arch: 'arm64', executable: 'gitgit-vscode-helper' },
  { platform: 'darwin', arch: 'x64', executable: 'gitgit-vscode-helper' },
  { platform: 'win32', arch: 'x64', executable: 'gitgit-vscode-helper.exe' },
]

export function assertWorkspaceTrusted(trusted: boolean): void {
  if (!trusted) {
    throw new HelperClientError('Workspace Trust is required before GitGit can start its helper.', 'workspace_untrusted')
  }
}

export function resolveHelperExecutable(
  extensionPath: string,
  platform: NodeJS.Platform = process.platform,
  arch = process.arch,
): string {
  const helper = SUPPORTED_HELPERS.find((candidate) => candidate.platform === platform && candidate.arch === arch)
  if (!helper) {
    throw new HelperClientError(`GitGit v0.1 does not support ${platform}-${arch}.`, 'unsupported_platform')
  }
  return path.join(extensionPath, 'dist', 'bin', helper.executable)
}

export class HelperClient {
  private readonly child: ChildProcessWithoutNullStreams
  private readonly pending = new Map<number, PendingRequest>()
  private nextID = 1
  private stdoutBuffer = Buffer.alloc(0)
  private stderrBytes = 0
  private disposed = false

  private constructor(private readonly options: HelperClientOptions) {
    this.child = spawn(options.executablePath, [], {
      shell: false,
      stdio: ['pipe', 'pipe', 'pipe'],
      windowsHide: true,
    })
    this.child.stdout.on('data', (chunk: Buffer) => this.acceptStdout(chunk))
    this.child.stderr.on('data', (chunk: Buffer) => {
      this.stderrBytes += chunk.length
      if (this.stderrBytes > MAX_STDERR_BYTES) {
        this.stopWithError(new HelperProtocolError('GitGit helper exceeded the stderr limit.', 'stderr_too_large'))
      }
    })
    this.child.once('error', (error) => {
      this.stopWithError(new HelperClientError(`Unable to start GitGit helper: ${error.message}`, 'helper_start_failed'))
    })
    this.child.once('exit', (code, signal) => {
      if (!this.disposed) {
        this.stopWithError(new HelperClientError(`GitGit helper stopped (${signal ?? code ?? 'unknown'}).`, 'helper_stopped'))
      }
    })
  }

  static async start(options: HelperClientOptions): Promise<HelperClient> {
    let stat
    try {
      stat = statSync(options.executablePath)
    } catch {
      throw new HelperClientError('The bundled GitGit helper is missing. Reinstall the platform-specific VSIX.', 'helper_missing')
    }
    if (!stat.isFile()) {
      throw new HelperClientError('The bundled GitGit helper path is not a file.', 'helper_missing')
    }

    const client = new HelperClient(options)
    const timeout = setTimeout(() => {
      client.stopWithError(new HelperClientError('GitGit helper initialization timed out.', 'initialize_timeout'))
    }, INITIALIZE_TIMEOUT_MS)
    try {
      const result = await client.request('initialize', {
        protocolVersion: PROTOCOL_VERSION,
        client: { name: 'gitgit-vscode', version: options.extensionVersion },
      } satisfies InitializeParams)
      validateInitializeResult(result)
      return client
    } catch (error) {
      client.dispose()
      throw error
    } finally {
      clearTimeout(timeout)
    }
  }

  static startForExtension(
    context: ExtensionContext,
    trusted: boolean,
    onProgress?: HelperClientOptions['onProgress'],
  ): Promise<HelperClient> {
    assertWorkspaceTrusted(trusted)
    return HelperClient.start({
      executablePath: resolveHelperExecutable(context.extensionPath),
      extensionVersion: String(context.extension.packageJSON.version),
      onProgress,
    })
  }

  async discover(directory: string, token?: CancellationToken): Promise<RepositoryDiscovery> {
    return parseRepositoryDiscovery(await this.request('repository.discover', { path: directory }, token))
  }

  async blameLines(request: BlameLinesRequest, token?: CancellationToken): Promise<BlameLinesResponse> {
    return parseBlameLinesResponse(await this.request('blame.lines', request, token))
  }

  async historyFile(request: HistoryFileRequest, token?: CancellationToken): Promise<HistoryFileResponse> {
    return parseHistoryFileResponse(await this.request('history.file', request, token))
  }

  async revisionContent(
    request: RevisionContentRequest,
    token?: CancellationToken,
  ): Promise<RevisionContentResponse> {
    return parseRevisionContentResponse(await this.request('revision.content', request, token))
  }

  async diffFile(request: DiffFileRequest, token?: CancellationToken): Promise<DiffFileResponse> {
    return parseDiffFileResponse(await this.request('diff.file', request, token))
  }

  async search(
    searchRequest: SearchRequest,
    token?: CancellationToken,
    onProgress?: (progress: SearchProgress) => void,
  ): Promise<SearchResponse> {
    return parseSearchResponse(await this.request(
      'search.run',
      searchRequest,
      token,
      onProgress
        ? (progress) => onProgress(logicalSearchProgress(searchRequest.requestId, progress))
        : undefined,
    ))
  }

  request<T = unknown>(
    method: HelperMethod,
    params: unknown,
    token?: CancellationToken,
    onProgress?: PendingRequest['onProgress'],
  ): Promise<T> {
    if (this.disposed) return Promise.reject(new HelperClientError('GitGit helper is closed.', 'helper_closed'))
    if (token?.isCancellationRequested) {
      return Promise.reject(new HelperClientError('GitGit helper request was cancelled.', 'request_cancelled'))
    }
    if (this.pending.size >= MAX_PENDING_REQUESTS) {
      return Promise.reject(new HelperClientError('Too many GitGit helper requests are in flight.', 'too_many_requests'))
    }

    const id = this.nextID++
    const payload = Buffer.from(`${JSON.stringify({ jsonrpc: '2.0', id, method, params })}\n`, 'utf8')
    if (payload.length > MAX_INPUT_BYTES) {
      return Promise.reject(new HelperClientError('GitGit helper request exceeds the input limit.', 'input_too_large'))
    }

    return new Promise<T>((resolve, reject) => {
      let cancellation = { dispose(): void {} }
      const cancel = (): void => {
        const pending = this.pending.get(id)
        if (!pending) return
        this.pending.delete(id)
        pending.disposeCancellation()
        this.sendCancellation(id)
        reject(new HelperClientError('GitGit helper request was cancelled.', 'request_cancelled'))
      }
      cancellation = token?.onCancellationRequested(cancel) ?? cancellation
      this.pending.set(id, {
        resolve: (value) => resolve(value as T),
        reject,
        disposeCancellation: () => cancellation.dispose(),
        ...(onProgress ? { onProgress } : {}),
      })
      this.child.stdin.write(payload, (error) => {
        if (!error) return
        const pending = this.takePending(id)
        pending?.reject(new HelperClientError(`Unable to write to GitGit helper: ${error.message}`, 'helper_write_failed'))
      })
    })
  }

  dispose(): void {
    if (this.disposed) return
    this.disposed = true
    this.rejectAll(new HelperClientError('GitGit helper was closed.', 'helper_closed'))
    this.child.kill()
  }

  private sendCancellation(id: number): void {
    if (this.disposed || !this.child.stdin.writable) return
    const payload = `${JSON.stringify({ jsonrpc: '2.0', method: '$/cancelRequest', params: { id } })}\n`
    this.child.stdin.write(payload)
  }

  private acceptStdout(chunk: Buffer): void {
    this.stdoutBuffer = Buffer.concat([this.stdoutBuffer, chunk])
    for (;;) {
      const newline = this.stdoutBuffer.indexOf(0x0a)
      if (newline < 0) break
      if (newline > MAX_OUTPUT_BYTES) {
        this.stopWithError(new HelperProtocolError('GitGit helper response line exceeds maxOutputBytes.', 'output_too_large'))
        return
      }
      const line = this.stdoutBuffer.subarray(0, newline)
      this.stdoutBuffer = this.stdoutBuffer.subarray(newline + 1)
      if (line.length > 0) this.acceptLine(line.toString('utf8'))
      if (this.disposed) return
    }
    if (this.stdoutBuffer.length > MAX_OUTPUT_BYTES) {
      this.stopWithError(new HelperProtocolError('GitGit helper response line exceeds maxOutputBytes.', 'output_too_large'))
    }
  }

  private acceptLine(line: string): void {
    let raw: unknown
    try {
      raw = JSON.parse(line)
    } catch {
      this.stopWithError(new HelperProtocolError('GitGit helper emitted invalid JSON.', 'invalid_json'))
      return
    }

    let message: JsonRpcResponse | JsonRpcProgressNotification
    try {
      message = parseJsonRpcMessage(raw)
    } catch (error) {
      this.stopWithError(error instanceof Error ? error : new HelperProtocolError(String(error)))
      return
    }
    if ('method' in message) {
      dispatchHelperProgress(this.pending, message.params)
      this.options.onProgress?.(message.params)
      return
    }

    const pending = this.takePending(message.id)
    if (!pending) return
    if (message.error) {
      pending.reject(new HelperClientError(message.error.message, message.error.data.code, message.error.code))
      return
    }
    pending.resolve(message.result)
  }

  private takePending(id: number): PendingRequest | undefined {
    const pending = this.pending.get(id)
    if (!pending) return undefined
    this.pending.delete(id)
    pending.disposeCancellation()
    return pending
  }

  private stopWithError(error: Error): void {
    if (this.disposed) return
    this.disposed = true
    this.rejectAll(error)
    this.child.kill()
  }

  private rejectAll(error: Error): void {
    for (const pending of this.pending.values()) {
      pending.disposeCancellation()
      pending.reject(error)
    }
    this.pending.clear()
  }
}
