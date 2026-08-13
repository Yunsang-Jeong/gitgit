import * as path from 'node:path'

import * as vscode from 'vscode'

import { HelperClient } from './helper/client.js'
import {
  CommitHighlightController,
  FileHistoryProvider,
  LineBlameController,
  type CommitFileSelection,
  type EditorRepositorySession,
} from './editor/controllers.js'
import type {
  RepositoryDiscovery,
  SearchProgress,
  SearchRequest,
  SearchResponse,
} from './helper/protocol.js'
import { SearchViewProvider } from './search/view.js'
import { isCommitObjectID, isSafeRepositoryRelativePath } from './shared/repository.js'

type RuntimePhase = 'untrusted' | 'unsupported' | 'idle' | 'loading' | 'ready' | 'error'

interface RuntimeState {
  phase: RuntimePhase
  message: string
  repository?: RepositoryDiscovery
}

class GitGitRuntime implements vscode.Disposable {
  private helper: HelperClient | undefined
  private stateValue: RuntimeState
  private starting: Promise<void> | undefined
  private candidate: string | undefined
  private refreshGeneration = 0
  private readonly changed = new vscode.EventEmitter<void>()
  readonly onDidChange = this.changed.event

  constructor(
    private readonly context: vscode.ExtensionContext,
    private readonly log: (message: string) => void,
  ) {
    this.stateValue = initialState()
  }

  get state(): RuntimeState {
    return this.stateValue
  }

  async refresh(force = false): Promise<void> {
    const generation = ++this.refreshGeneration
    const blocked = initialState()
    if (blocked.phase === 'untrusted' || blocked.phase === 'unsupported') {
      this.helper?.dispose()
      this.helper = undefined
      this.candidate = undefined
      this.setState(blocked)
      return
    }

    const candidate = repositoryCandidateDirectory()
    if (!candidate) {
      this.helper?.dispose()
      this.helper = undefined
      this.candidate = undefined
      this.setState({ phase: 'idle', message: 'Open a local folder or file inside a Git repository.' })
      return
    }
    // Do not cache by repository containment: a directory below the current
    // worktree can itself be a nested Git repository. Only identical discovery
    // candidates are safe to reuse, and an explicit Refresh always rechecks.
    if (!force && this.stateValue.phase === 'ready' && this.candidate === candidate) return
    while (this.starting) await this.starting
    // Several active-editor events can queue while discovery is running. Only
    // the newest candidate may start the next discovery; otherwise two queued
    // continuations can race and let an older repository win last.
    if (generation !== this.refreshGeneration) return
    if (!force && this.stateValue.phase === 'ready' && this.candidate === candidate) return
    const operation = this.refreshTrusted(candidate, generation)
    this.starting = operation
    try {
      await operation
    } finally {
      if (this.starting === operation) this.starting = undefined
    }
  }

  dispose(): void {
    this.helper?.dispose()
    this.changed.dispose()
  }

  async runSearch(
    request: SearchRequest,
    onProgress: (progress: SearchProgress) => void,
    token: vscode.CancellationToken,
  ): Promise<SearchResponse> {
    if (!vscode.workspace.isTrusted) throw new Error('Workspace Trust is required to Search.')
    if (vscode.env.remoteName !== undefined) throw new Error('Remote Search is not supported in GitGit v0.1.')
    if (!this.helper || this.stateValue.phase !== 'ready' || this.stateValue.repository?.root !== request.repositoryRoot) {
      throw new Error('Open and refresh a local Git repository before Search.')
    }
    return this.helper.search(request, token, onProgress)
  }

  get editorSession(): EditorRepositorySession | undefined {
    if (!this.helper || this.stateValue.phase !== 'ready' || !this.stateValue.repository) return undefined
    return {
      root: this.stateValue.repository.root,
      head: this.stateValue.repository.head,
      helper: this.helper,
    }
  }

  private async refreshTrusted(candidate: string, generation: number): Promise<void> {
    this.log(`[repository] discovering from ${candidate}`)
    this.setState({ phase: 'loading', message: 'Discovering local Git repository…' })
    try {
      this.helper ??= await HelperClient.startForExtension(this.context, vscode.workspace.isTrusted)
      const repository = await this.helper.discover(candidate)
      if (generation !== this.refreshGeneration) {
        this.log(`[repository] ignored stale discovery from ${candidate}`)
        if (this.stateValue.phase === 'idle'
          || this.stateValue.phase === 'untrusted'
          || this.stateValue.phase === 'unsupported') {
          this.helper.dispose()
          this.helper = undefined
        }
        return
      }
      this.candidate = candidate
      this.log(`[repository] ready root=${repository.root} head=${repository.head.slice(0, 12)}`)
      this.setState({ phase: 'ready', message: 'Repository ready.', repository })
    } catch (error) {
      if (generation !== this.refreshGeneration) return
      this.helper?.dispose()
      this.helper = undefined
      this.candidate = undefined
      const message = error instanceof Error ? error.message : String(error)
      this.log(`[repository] failed: ${message}`)
      this.setState({ phase: 'error', message })
    }
  }

  private setState(state: RuntimeState): void {
    this.stateValue = state
    this.changed.fire()
  }
}

function initialState(): RuntimeState {
  if (!vscode.workspace.isTrusted) {
    return {
      phase: 'untrusted',
      message: 'Trust this workspace before GitGit starts its bundled helper.',
    }
  }
  if (vscode.env.remoteName !== undefined) {
    return {
      phase: 'unsupported',
      message: `Remote environment ${vscode.env.remoteName} is not supported in GitGit v0.1.`,
    }
  }
  const folders = vscode.workspace.workspaceFolders
  if (folders?.some((folder) => folder.uri.scheme !== 'file')) {
    return {
      phase: 'unsupported',
      message: 'Virtual workspaces are not supported in GitGit v0.1.',
    }
  }
  return { phase: 'idle', message: 'Open a local Git repository.' }
}

function repositoryCandidateDirectory(): string | undefined {
  const document = vscode.window.activeTextEditor?.document
  if (document?.uri.scheme === 'file') return path.dirname(document.uri.fsPath)
  return vscode.workspace.workspaceFolders?.find((folder) => folder.uri.scheme === 'file')?.uri.fsPath
}

function commitFileSelection(value: unknown): CommitFileSelection | undefined {
  if (typeof value !== 'object' || value === null) return undefined
  const selection = value as Record<string, unknown>
  if (typeof selection.commit !== 'string' || !isCommitObjectID(selection.commit)) return undefined
  if (typeof selection.path !== 'string' || !isSafeRepositoryRelativePath(selection.path)) return undefined
  if (selection.oldPath !== undefined
    && (typeof selection.oldPath !== 'string' || !isSafeRepositoryRelativePath(selection.oldPath))) return undefined
  return {
    commit: selection.commit,
    path: selection.path,
    ...(typeof selection.oldPath === 'string' ? { oldPath: selection.oldPath } : {}),
  }
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const output = vscode.window.createOutputChannel('GitGit')
  const log = (message: string): void => output.appendLine(`${new Date().toISOString()} ${message}`)
  const runtime = new GitGitRuntime(context, log)
  const search = new SearchViewProvider(runtime)
  const session = (): EditorRepositorySession | undefined => runtime.editorSession
  const blame = new LineBlameController(session, log)
  const highlights = new CommitHighlightController(session, log)
  const investigation = new FileHistoryProvider(session)
  const refresh = async (force = false): Promise<void> => {
    investigation.refresh()
    await runtime.refresh(force)
    investigation.refresh()
    if (runtime.editorSession) {
      blame.refresh()
      highlights.refresh()
    }
    else {
      blame.clear()
      highlights.clear()
    }
  }
  const selectCommitFile = async (selection: CommitFileSelection): Promise<void> => {
    await highlights.select(selection)
  }

  context.subscriptions.push(
    output,
    runtime,
    search,
    investigation,
    blame,
    highlights,
    vscode.window.registerWebviewViewProvider('gitgit.search', search),
    vscode.window.registerTreeDataProvider('gitgit.investigation', investigation),
    vscode.commands.registerCommand('gitgit.refresh', () => refresh(true)),
    vscode.commands.registerCommand('gitgit.commit.select', async (value: unknown) => {
      const selection = commitFileSelection(value)
      if (!selection) return
      await selectCommitFile(selection)
    }),
    vscode.commands.registerCommand('gitgit.commit.clear', () => highlights.clear()),
    vscode.workspace.onDidGrantWorkspaceTrust(() => refresh(true)),
    vscode.window.onDidChangeActiveTextEditor(async () => {
      investigation.refresh()
      await runtime.refresh()
      investigation.refresh()
      blame.refresh()
      highlights.refresh()
    }),
    search.onDidSelectResult(async (selection) => {
      const commitSelection: CommitFileSelection = {
        commit: selection.commit,
        path: selection.path,
        ...(selection.oldPath ? { oldPath: selection.oldPath } : {}),
      }
      if (selection.action === 'selectCommitFile') {
        await selectCommitFile(commitSelection)
        return
      }
      const repository = runtime.editorSession
      if (!repository) return
      const target = vscode.Uri.file(path.join(repository.root, ...selection.path.split('/')))
      try {
        const document = await vscode.workspace.openTextDocument(target)
        await vscode.window.showTextDocument(document, { preview: true })
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        void vscode.window.showErrorMessage(`GitGit: ${message}`)
      }
    }),
  )

  await refresh(true)
  blame.start()
}

export function deactivate(): void {}
