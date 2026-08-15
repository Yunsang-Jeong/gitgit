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
import { searchFileSelection } from './search/tree-model.js'
import {
  isCommitObjectID,
  isSafeRepositoryRelativePath,
  repositoryRelativePath,
  revisionRepositoryTarget,
  type RevisionRepositoryTarget,
} from './shared/repository.js'
import { REVISION_DOCUMENT_SCHEME } from './editor/revision-model.js'
import { RevisionDocumentProvider } from './editor/revisions.js'

type RuntimePhase = 'untrusted' | 'unsupported' | 'idle' | 'loading' | 'ready' | 'error'

interface RuntimeState {
  phase: RuntimePhase
  message: string
  repository?: RepositoryDiscovery
}

interface RuntimeRefreshTarget {
  candidates: string[]
  expectedRoot?: string
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

  async refresh(force = false, targetOverride?: RuntimeRefreshTarget | null): Promise<void> {
    const generation = ++this.refreshGeneration
    const blocked = initialState()
    if (blocked.phase === 'untrusted' || blocked.phase === 'unsupported') {
      this.helper?.dispose()
      this.helper = undefined
      this.candidate = undefined
      this.setState(blocked)
      return
    }

    const defaultCandidate = repositoryCandidateDirectory()
    const target = targetOverride === null
      ? undefined
      : targetOverride ?? (defaultCandidate ? { candidates: [defaultCandidate] } : undefined)
    if (!target) {
      this.helper?.dispose()
      this.helper = undefined
      this.candidate = undefined
      this.setState({ phase: 'idle', message: 'Open a local folder or file inside a Git repository.' })
      return
    }
    // Do not cache by repository containment: a directory below the current
    // worktree can itself be a nested Git repository. Only identical discovery
    // candidates are safe to reuse, and an explicit Refresh always rechecks.
    if (!force && this.readyForTarget(target)) return
    while (this.starting) await this.starting
    // Several active-editor events can queue while discovery is running. Only
    // the newest candidate may start the next discovery; otherwise two queued
    // continuations can race and let an older repository win last.
    if (generation !== this.refreshGeneration) return
    if (!force && this.readyForTarget(target)) return
    const operation = this.refreshTrusted(target, generation)
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
      ...(this.stateValue.repository.webRemotes ? { webRemotes: this.stateValue.repository.webRemotes } : {}),
      helper: this.helper,
    }
  }

  private async refreshTrusted(target: RuntimeRefreshTarget, generation: number): Promise<void> {
    this.log(`[repository] discovering from ${target.candidates.join(', ')}`)
    this.setState({ phase: 'loading', message: 'Discovering local Git repository…' })
    try {
      this.helper ??= await HelperClient.startForExtension(this.context, vscode.workspace.isTrusted)
      const helper = this.helper
      let repository: RepositoryDiscovery | undefined
      let selectedCandidate: string | undefined
      let lastError: unknown
      for (const candidate of target.candidates) {
        try {
          const discovered = await helper.discover(candidate)
          if (generation !== this.refreshGeneration) {
            this.log(`[repository] ignored stale discovery from ${candidate}`)
            if (this.stateValue.phase === 'idle'
              || this.stateValue.phase === 'untrusted'
              || this.stateValue.phase === 'unsupported') {
              helper.dispose()
              if (this.helper === helper) this.helper = undefined
            }
            return
          }
          if (target.expectedRoot && discovered.root !== target.expectedRoot) continue
          repository = discovered
          selectedCandidate = candidate
          break
        } catch (error) {
          lastError = error
          if (!target.expectedRoot) throw error
        }
      }
      if (!repository || !selectedCandidate) {
        if (lastError instanceof Error) throw lastError
        throw new Error('Revision repository root does not match the workspace discovery result.')
      }
      this.candidate = selectedCandidate
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

  private readyForTarget(target: RuntimeRefreshTarget): boolean {
    return this.stateValue.phase === 'ready'
      && (!target.expectedRoot || this.stateValue.repository?.root === target.expectedRoot)
      && target.candidates.some((candidate) => (
        this.candidate === candidate || this.stateValue.repository?.root === candidate
      ))
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
  if (selection.status !== undefined
    && (typeof selection.status !== 'string' || !/^[A-Z?][A-Z0-9?]{0,7}$/u.test(selection.status))) return undefined
  return {
    commit: selection.commit,
    path: selection.path,
    ...(typeof selection.oldPath === 'string' ? { oldPath: selection.oldPath } : {}),
    ...(typeof selection.status === 'string' ? { status: selection.status } : {}),
  }
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const output = vscode.window.createOutputChannel('GitGit')
  const log = (message: string): void => output.appendLine(`${new Date().toISOString()} ${message}`)
  const runtime = new GitGitRuntime(context, log)
  const search = new SearchViewProvider(runtime)
  const session = (): EditorRepositorySession | undefined => runtime.editorSession
  const revisions = new RevisionDocumentProvider(session, log)
  const blame = new LineBlameController(session, log)
  const highlights = new CommitHighlightController(session, revisions, log)
  const investigation = new FileHistoryProvider(session, revisions)
  const historyView = vscode.window.createTreeView('gitgit.investigation', {
    treeDataProvider: investigation,
    showCollapseAll: false,
  })
  const searchView = vscode.window.createTreeView('gitgit.search', {
    treeDataProvider: search,
    showCollapseAll: true,
  })
  search.attachView(searchView)
  const updateHistoryScope = (): void => {
    const editor = vscode.window.activeTextEditor
    const repository = runtime.editorSession
    if (!editor || !repository) {
      historyView.description = undefined
      return
    }
    const revision = revisions.resource(editor.document.uri)
    const relativePath = editor.document.uri.scheme === 'file'
      ? repositoryRelativePath(repository.root, editor.document.uri.fsPath)
      : revision?.root === repository.root ? revision.selectedPath : undefined
    historyView.description = relativePath?.split('/').at(-1)
  }
  let refreshGeneration = 0
  const refresh = async (force = false): Promise<void> => {
    const generation = ++refreshGeneration
    investigation.refresh()
    updateHistoryScope()
    const editor = vscode.window.activeTextEditor
    const revision = editor?.document.uri.scheme === REVISION_DOCUMENT_SCHEME
      ? revisions.resource(editor.document.uri)
      : undefined
    const revisionTarget = revision
      ? revisionRefreshTarget(revision.root, runtime.editorSession?.root)
      : undefined
    await runtime.refresh(
      force,
      editor?.document.uri.scheme === REVISION_DOCUMENT_SCHEME ? revisionTarget ?? null : undefined,
    )
    if (generation !== refreshGeneration) return
    investigation.refresh()
    updateHistoryScope()
    if (runtime.editorSession) {
      blame.refresh()
      if (revision && editor && runtime.editorSession.root === revision.root) {
        await highlights.restore(revision, editor.document.uri)
      } else {
        highlights.refresh()
      }
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
    searchView,
    investigation,
    historyView,
    revisions,
    blame,
    highlights,
    vscode.workspace.registerTextDocumentContentProvider(REVISION_DOCUMENT_SCHEME, revisions),
    vscode.commands.registerCommand('gitgit.refresh', () => refresh(true)),
    vscode.commands.registerCommand('gitgit.search.run', () => search.promptAndSearch()),
    vscode.commands.registerCommand('gitgit.search.rerun', () => search.rerun()),
    vscode.commands.registerCommand('gitgit.search.filters', () => search.configureFilters()),
    vscode.commands.registerCommand('gitgit.search.cancel', () => search.cancel()),
    vscode.commands.registerCommand('gitgit.search.clear', () => search.clear()),
    vscode.commands.registerCommand('gitgit.commit.select', async (value: unknown) => {
      const selection = commitFileSelection(value)
      if (!selection) return
      await selectCommitFile(selection)
    }),
    vscode.commands.registerCommand('gitgit.commit.clear', () => (
      highlights.clearByUser(vscode.window.activeTextEditor?.document.uri)
    )),
    vscode.commands.registerCommand('gitgit.search.openWorkingFile', async (value: unknown) => {
      const selection = searchFileSelection(value)
      const repository = runtime.editorSession
      if (!selection || !repository) return
      const target = vscode.Uri.file(path.join(repository.root, ...selection.path.split('/')))
      try {
        const document = await vscode.workspace.openTextDocument(target)
        await vscode.window.showTextDocument(document, { preview: true })
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        void vscode.window.showErrorMessage(`GitGit: ${message}`)
      }
    }),
    vscode.commands.registerCommand('gitgit.search.copyCommitId', async (value: unknown) => {
      const commit = searchCommitObjectID(value)
      if (commit) await vscode.env.clipboard.writeText(commit)
    }),
    vscode.workspace.onDidGrantWorkspaceTrust(() => refresh(true)),
    vscode.window.onDidChangeActiveTextEditor(() => refresh(false)),
  )

  await refresh(true)
  blame.start()
}

export function deactivate(): void {}

function revisionRefreshTarget(root: string, currentRoot?: string): RevisionRepositoryTarget | undefined {
  const workspaceRoots = vscode.workspace.workspaceFolders
    ?.filter((folder) => folder.uri.scheme === 'file')
    .map((folder) => folder.uri.fsPath) ?? []
  return revisionRepositoryTarget(root, currentRoot, workspaceRoots)
}

function searchCommitObjectID(value: unknown): string | undefined {
  if (typeof value !== 'object' || value === null) return undefined
  const record = value as Record<string, unknown>
  const direct = record.commit
  if (typeof direct === 'string' && isCommitObjectID(direct)) return direct
  if (typeof record.result !== 'object' || record.result === null) return undefined
  const commit = (record.result as Record<string, unknown>).commit
  return typeof commit === 'string' && isCommitObjectID(commit) ? commit : undefined
}
