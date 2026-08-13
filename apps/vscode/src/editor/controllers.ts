import * as path from 'node:path'

import * as vscode from 'vscode'

import {
  blameableDocumentLineCount,
  normalizedDeletionAnchors,
  normalizedTargetRanges,
  revisionContentMatchesDocument,
  traceFileHistorySelections,
  visibleBlameWindow,
} from './model.js'
import type {
  BlameLinesRequest,
  BlameLinesResponse,
  DiffFileRequest,
  DiffFileResponse,
  HistoryCommit,
  HistoryFileRequest,
  HistoryFileResponse,
  RevisionContentRequest,
  RevisionContentResponse,
} from '../helper/protocol.js'
import { repositoryRelativePath } from '../shared/repository.js'

export interface EditorGitClient {
  blameLines(params: BlameLinesRequest, token?: vscode.CancellationToken): Promise<BlameLinesResponse>
  historyFile(params: HistoryFileRequest, token?: vscode.CancellationToken): Promise<HistoryFileResponse>
  revisionContent(params: RevisionContentRequest, token?: vscode.CancellationToken): Promise<RevisionContentResponse>
  diffFile(params: DiffFileRequest, token?: vscode.CancellationToken): Promise<DiffFileResponse>
}

export interface EditorRepositorySession {
  root: string
  head: string
  helper: EditorGitClient
}

export type EditorRepositorySessionProvider = () => EditorRepositorySession | undefined

export interface CommitFileSelection {
  commit: string
  path: string
  oldPath?: string
}

export class LineBlameController implements vscode.Disposable {
  private readonly decoration: vscode.TextEditorDecorationType
  private readonly subscriptions: vscode.Disposable[] = []
  private cancellation: vscode.CancellationTokenSource | undefined
  private timer: NodeJS.Timeout | undefined
  private generation = 0
  private lastDiagnostic = ''

  constructor(
    private readonly session: EditorRepositorySessionProvider,
    private readonly log: (message: string) => void = () => {},
  ) {
    this.decoration = vscode.window.createTextEditorDecorationType({
      after: {
        color: new vscode.ThemeColor('editorCodeLens.foreground'),
        margin: '0 0 0 2em',
      },
      rangeBehavior: vscode.DecorationRangeBehavior.ClosedClosed,
    })
    this.subscriptions.push(
      this.decoration,
      vscode.window.onDidChangeActiveTextEditor((active) => {
        for (const editor of vscode.window.visibleTextEditors) {
          if (editor !== active) editor.setDecorations(this.decoration, [])
        }
        this.schedule(0)
      }),
      vscode.window.onDidChangeTextEditorSelection((event) => {
        if (event.textEditor === vscode.window.activeTextEditor) this.schedule(40)
      }),
      vscode.window.onDidChangeTextEditorVisibleRanges((event) => {
        if (event.textEditor === vscode.window.activeTextEditor) this.schedule(80)
      }),
      vscode.workspace.onDidChangeTextDocument((event) => {
        if (event.document === vscode.window.activeTextEditor?.document) this.schedule(220)
      }),
      vscode.workspace.onDidChangeConfiguration((event) => {
        if (event.affectsConfiguration('gitgit.blame')
          || event.affectsConfiguration('git.blame.editorDecoration.enabled')) this.schedule(0)
      }),
      vscode.extensions.onDidChange(() => this.schedule(0)),
    )
  }

  start(): void {
    this.schedule(0)
  }

  clear(): void {
    this.generation++
    this.cancellation?.cancel()
    for (const editor of vscode.window.visibleTextEditors) editor.setDecorations(this.decoration, [])
  }

  refresh(): void {
    this.schedule(0)
  }

  dispose(): void {
    this.generation++
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    if (this.timer) clearTimeout(this.timer)
    for (const subscription of this.subscriptions) subscription.dispose()
  }

  private schedule(delay: number): void {
    if (this.timer) clearTimeout(this.timer)
    this.timer = setTimeout(() => {
      this.timer = undefined
      void this.update()
    }, delay)
  }

  private async update(): Promise<void> {
    const generation = ++this.generation
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.cancellation = new vscode.CancellationTokenSource()

    const editor = vscode.window.activeTextEditor
    const session = this.session()
    if (!editor || !session || editor.document.uri.scheme !== 'file') {
      editor?.setDecorations(this.decoration, [])
      this.diagnose(!editor
        ? '[blame] skipped: no active editor'
        : !session
          ? '[blame] skipped: repository session unavailable'
          : `[blame] skipped: unsupported document scheme ${editor.document.uri.scheme}`)
      return
    }
    const relativePath = repositoryRelativePath(session.root, editor.document.uri.fsPath)
    const configuredMode = vscode.workspace.getConfiguration('gitgit.blame', editor.document.uri)
      .get<'auto' | 'off' | 'activeLine' | 'visibleLines'>('mode', 'auto')
    const nativeBlameEnabled = vscode.workspace.getConfiguration('git', editor.document.uri)
      .get<boolean>('blame.editorDecoration.enabled', false)
    const gitLensActive = vscode.extensions.getExtension('eamodio.gitlens')?.isActive === true
    const mode = configuredMode === 'auto'
      ? (nativeBlameEnabled || gitLensActive ? 'off' : 'activeLine')
      : configuredMode
    const blameableLineCount = blameableDocumentLineCount(
      editor.document.lineCount,
      editor.document.lineAt(editor.document.lineCount - 1).text,
    )
    const window = mode === 'visibleLines'
      ? visibleBlameWindow(editor.visibleRanges.map((range) => ({
        startLine: range.start.line,
        endLine: range.end.line,
      })), blameableLineCount)
      : mode === 'activeLine'
        ? editor.selection.active.line < blameableLineCount
          ? { startLine: editor.selection.active.line + 1, endLine: editor.selection.active.line + 1 }
          : undefined
        : undefined
    if (!relativePath || !window) {
      editor.setDecorations(this.decoration, [])
      this.diagnose(!relativePath
        ? `[blame] skipped: file is outside repository ${session.root}`
        : `[blame] disabled mode=${mode} native=${nativeBlameEnabled} gitlens=${gitLensActive}`)
      return
    }

    try {
      this.diagnose(`[blame] request path=${relativePath} lines=${window.startLine}-${window.endLine} mode=${mode}`)
      const result = await session.helper.blameLines({
        repositoryRoot: session.root,
        relativePath,
        startLine: window.startLine,
        endLine: window.endLine,
        contents: editor.document.isDirty ? editor.document.getText() : undefined,
        ignoreWhitespace: vscode.workspace.getConfiguration('gitgit.blame', editor.document.uri)
          .get<boolean>('ignoreWhitespace', false),
      }, this.cancellation.token)
      if (generation !== this.generation || editor !== vscode.window.activeTextEditor) return
      const decorations: vscode.DecorationOptions[] = []
      for (const line of result.lines) {
        const index = line.line - 1
        if (index < 0 || index >= editor.document.lineCount) continue
        const displayDate = Number.isNaN(Date.parse(line.date))
          ? line.date
          : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(line.date))
        const shortCommit = line.commit.startsWith('00000000') ? 'working' : line.commit.slice(0, 8)
        const hover = new vscode.MarkdownString(undefined, false)
        hover.appendMarkdown(`**${escapeMarkdown(line.message || 'Uncommitted change')}**\n\n`)
        hover.appendText(`${line.author.name} · ${displayDate}\n${line.commit}`)
        decorations.push({
          range: editor.document.lineAt(index).range,
          hoverMessage: hover,
          renderOptions: { after: { contentText: `  ${shortCommit} · ${line.author.name}` } },
        })
      }
      editor.setDecorations(this.decoration, decorations)
      this.diagnose(`[blame] applied path=${relativePath} decorations=${decorations.length}`)
    } catch (error) {
      if (generation !== this.generation || isCancellation(error)) return
      editor.setDecorations(this.decoration, [])
      this.diagnose(`[blame] failed path=${relativePath}: ${errorMessage(error)}`)
    }
  }

  private diagnose(message: string): void {
    if (message === this.lastDiagnostic) return
    this.lastDiagnostic = message
    this.log(message)
  }
}

export class CommitHighlightController implements vscode.Disposable {
  private readonly changedDecoration = vscode.window.createTextEditorDecorationType({
    isWholeLine: true,
    backgroundColor: new vscode.ThemeColor('gitgit.commitChangeBackground'),
    overviewRulerColor: new vscode.ThemeColor('gitgit.commitChangeOverview'),
    overviewRulerLane: vscode.OverviewRulerLane.Left,
  })
  private readonly deletionDecoration = vscode.window.createTextEditorDecorationType({
    isWholeLine: true,
    borderStyle: 'solid',
    borderWidth: '1px 0 0 0',
    borderColor: new vscode.ThemeColor('gitgit.commitDeletionForeground'),
    overviewRulerColor: new vscode.ThemeColor('gitgit.commitDeletionForeground'),
    overviewRulerLane: vscode.OverviewRulerLane.Left,
  })
  private readonly subscriptions: vscode.Disposable[] = []
  private selection: CommitFileSelection | undefined
  private selectionRoot: string | undefined
  private cancellation: vscode.CancellationTokenSource | undefined
  private generation = 0

  constructor(
    private readonly session: EditorRepositorySessionProvider,
    private readonly log: (message: string) => void = () => {},
  ) {
    this.subscriptions.push(
      this.changedDecoration,
      this.deletionDecoration,
      vscode.window.onDidChangeActiveTextEditor(() => { void this.update() }),
      vscode.workspace.onDidChangeTextDocument((event) => {
        if (event.document === vscode.window.activeTextEditor?.document && this.selection) this.clear()
      }),
    )
  }

  async select(selection: CommitFileSelection): Promise<void> {
    const session = this.session()
    if (!session) {
      this.log('[highlight] selection ignored: repository session unavailable')
      this.clear()
      return
    }
    this.selection = selection
    this.selectionRoot = session.root
    this.log(`[highlight] selected commit=${selection.commit.slice(0, 12)} path=${selection.path}`)
    const target = vscode.Uri.file(path.join(session.root, ...selection.path.split('/')))
    try {
      const document = await vscode.workspace.openTextDocument(target)
      await vscode.window.showTextDocument(document, { preview: true, preserveFocus: false })
    } catch (error) {
      // Deleted files can still be inspected through Search; no working-tree editor exists to decorate.
      this.log(`[highlight] working-tree file unavailable path=${selection.path}: ${errorMessage(error)}`)
    }
    await this.update()
  }

  clear(): void {
    this.selection = undefined
    this.selectionRoot = undefined
    this.generation++
    this.cancellation?.cancel()
    this.clearDecorations()
  }

  refresh(): void {
    void this.update()
  }

  dispose(): void {
    this.clear()
    this.cancellation?.dispose()
    for (const subscription of this.subscriptions) subscription.dispose()
  }

  private async update(): Promise<void> {
    const generation = ++this.generation
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.cancellation = new vscode.CancellationTokenSource()

    const editor = vscode.window.activeTextEditor
    const session = this.session()
    const selection = this.selection
    if (!editor || !session || !selection || editor.document.uri.scheme !== 'file') {
      this.clearDecorations()
      if (selection) {
        this.log(!session
          ? '[highlight] skipped: repository session unavailable'
          : !editor
            ? '[highlight] skipped: no active editor'
            : `[highlight] skipped: unsupported document scheme ${editor.document.uri.scheme}`)
      }
      return
    }
    if (this.selectionRoot !== session.root) {
      this.log(`[highlight] cleared after repository switch from ${this.selectionRoot ?? '<unknown>'} to ${session.root}`)
      this.clear()
      return
    }
    const relativePath = repositoryRelativePath(session.root, editor.document.uri.fsPath)
    if (relativePath !== selection.path) {
      this.clearDecorations()
      this.log(`[highlight] skipped: active path=${relativePath ?? '<outside repository>'} selected path=${selection.path}`)
      return
    }

    try {
      const revision = await session.helper.revisionContent({
        repositoryRoot: session.root,
        revision: selection.commit,
        relativePath: selection.path,
      }, this.cancellation.token)
      if (generation !== this.generation || editor !== vscode.window.activeTextEditor) return
      if (!revisionContentMatchesDocument(revision.content, editor.document.getText())) {
        this.clearDecorations()
        this.log(`[highlight] skipped: working-tree content differs from commit=${selection.commit.slice(0, 12)} path=${selection.path}`)
        return
      }
      const result = await session.helper.diffFile({
        repositoryRoot: session.root,
        commit: selection.commit,
        path: selection.path,
        oldPath: selection.oldPath,
        context: 3,
      }, this.cancellation.token)
      if (generation !== this.generation || editor !== vscode.window.activeTextEditor) return
      const changed = normalizedTargetRanges(result.targetRanges, editor.document.lineCount).map((range) => ({
        range: new vscode.Range(
          new vscode.Position(range.startLine - 1, 0),
          editor.document.lineAt(range.endLine - 1).range.end,
        ),
        hoverMessage: `Changed by ${selection.commit.slice(0, 12)}`,
      }))
      const deletions = normalizedDeletionAnchors(result.deletionAnchors, editor.document.lineCount).map((anchor) => ({
        range: editor.document.lineAt(anchor.anchorLine - 1).range,
        hoverMessage: `Deletion in ${selection.commit.slice(0, 12)}`,
      }))
      editor.setDecorations(this.changedDecoration, changed)
      editor.setDecorations(this.deletionDecoration, deletions)
      this.log(`[highlight] applied commit=${selection.commit.slice(0, 12)} path=${selection.path} ranges=${changed.length} deletions=${deletions.length}`)
    } catch (error) {
      if (generation !== this.generation || isCancellation(error)) return
      this.clearDecorations()
      this.log(`[highlight] failed commit=${selection.commit.slice(0, 12)} path=${selection.path}: ${errorMessage(error)}`)
    }
  }

  private clearDecorations(): void {
    for (const editor of vscode.window.visibleTextEditors) {
      editor.setDecorations(this.changedDecoration, [])
      editor.setDecorations(this.deletionDecoration, [])
    }
  }
}

class HistoryItem extends vscode.TreeItem {
  constructor(commit: HistoryCommit, selection: CommitFileSelection) {
    super(commit.message.split('\n')[0] || commit.shortCommit, vscode.TreeItemCollapsibleState.None)
    this.description = `${commit.shortCommit} · ${commit.author.name}`
    const tooltip = new vscode.MarkdownString(undefined, false)
    tooltip.appendMarkdown(`**${escapeMarkdown(commit.message)}**\n\n`)
    tooltip.appendText(`${commit.author.name} <${commit.author.email ?? ''}>\n${commit.date}\n${commit.commit}`)
    this.tooltip = tooltip
    this.iconPath = new vscode.ThemeIcon('git-commit')
    this.command = {
      command: 'gitgit.commit.select',
      title: 'Highlight Commit Changes',
      arguments: [selection],
    }
    this.contextValue = 'gitgit.historyCommit'
  }
}

class StatusItem extends vscode.TreeItem {
  constructor(label: string, description?: string, icon = 'info') {
    super(label, vscode.TreeItemCollapsibleState.None)
    this.description = description
    this.iconPath = new vscode.ThemeIcon(icon)
  }
}

export class FileHistoryProvider implements vscode.TreeDataProvider<vscode.TreeItem>, vscode.Disposable {
  private readonly changed = new vscode.EventEmitter<vscode.TreeItem | undefined>()
  private cancellation: vscode.CancellationTokenSource | undefined
  private generation = 0
  readonly onDidChangeTreeData = this.changed.event

  constructor(private readonly session: EditorRepositorySessionProvider) {}

  getTreeItem(element: vscode.TreeItem): vscode.TreeItem {
    return element
  }

  async getChildren(element?: vscode.TreeItem): Promise<vscode.TreeItem[]> {
    if (element) return []
    const generation = ++this.generation
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.cancellation = new vscode.CancellationTokenSource()

    const editor = vscode.window.activeTextEditor
    const session = this.session()
    if (!session) return [new StatusItem('No Git repository discovered')]
    if (!editor || editor.document.uri.scheme !== 'file') return [new StatusItem('Open a local file to inspect its history')]
    const relativePath = repositoryRelativePath(session.root, editor.document.uri.fsPath)
    if (!relativePath) return [new StatusItem('Active file is outside this repository', undefined, 'warning')]

    try {
      const result = await session.helper.historyFile({
        repositoryRoot: session.root,
        relativePath,
        limit: 100,
      }, this.cancellation.token)
      if (generation !== this.generation) return []
      if (result.commits.length === 0) return [new StatusItem('No commits found for this file')]
      const selections = traceFileHistorySelections(result.commits, relativePath)
      return result.commits.map((commit, index) => new HistoryItem(commit, selections[index] ?? {
        commit: commit.commit,
        path: relativePath,
      }))
    } catch (error) {
      if (generation !== this.generation || isCancellation(error)) return []
      return [new StatusItem('Unable to read file history', errorMessage(error), 'error')]
    }
  }

  refresh(): void {
    this.generation++
    this.cancellation?.cancel()
    this.changed.fire(undefined)
  }

  dispose(): void {
    this.generation++
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.changed.dispose()
  }
}

function escapeMarkdown(value: string): string {
  return value.replace(/[\\`*_{}\[\]()#+\-.!]/gu, '\\$&')
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function isCancellation(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  const stableCode = 'stableCode' in error ? String(error.stableCode) : ''
  return stableCode === 'request_cancelled' || /cancel/iu.test(error.message)
}
