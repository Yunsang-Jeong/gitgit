import * as vscode from 'vscode'

import {
  REVISION_DOCUMENT_SCHEME,
  parseRevisionDocumentUriParts,
  revisionDocumentUriParts,
  type RevisionDocumentResource,
} from './revision-model.js'

export interface RevisionContentClient {
  revisionContent(
    params: { repositoryRoot: string, revision: string, relativePath: string },
    token?: vscode.CancellationToken,
  ): Promise<{ content: string }>
}

export interface RevisionDocumentSession {
  root: string
  helper: RevisionContentClient
}

export type RevisionDocumentSessionProvider = () => RevisionDocumentSession | undefined

export class RevisionDocumentProvider implements vscode.TextDocumentContentProvider, vscode.Disposable {
  private readonly cachedContent = new Map<string, string>()
  private readonly subscriptions: vscode.Disposable[] = []

  constructor(
    private readonly session: RevisionDocumentSessionProvider,
    private readonly log: (message: string) => void = () => {},
  ) {
    this.subscriptions.push(vscode.workspace.onDidCloseTextDocument((document) => {
      if (document.uri.scheme === REVISION_DOCUMENT_SCHEME) this.cachedContent.delete(document.uri.toString())
    }))
  }

  remember(resource: RevisionDocumentResource, content: string): vscode.Uri {
    const parts = revisionDocumentUriParts(resource)
    const uri = vscode.Uri.from({ scheme: REVISION_DOCUMENT_SCHEME, ...parts })
    this.cachedContent.set(uri.toString(), content)
    return uri
  }

  resource(uri: vscode.Uri): RevisionDocumentResource | undefined {
    if (uri.scheme !== REVISION_DOCUMENT_SCHEME) return undefined
    try {
      return parseRevisionDocumentUriParts(uri)
    } catch (error) {
      this.log(`[revision] rejected invalid URI: ${errorMessage(error)}`)
      return undefined
    }
  }

  async provideTextDocumentContent(uri: vscode.Uri, token: vscode.CancellationToken): Promise<string> {
    const resource = this.resource(uri)
    if (!resource) throw new Error('GitGit revision document URI is invalid.')
    const session = this.session()
    if (!vscode.workspace.isTrusted || vscode.env.remoteName !== undefined || !session || session.root !== resource.root) {
      throw new Error('GitGit revision document is not authorized for the active local repository.')
    }
    const cached = this.cachedContent.get(uri.toString())
    if (cached !== undefined) return cached
    const result = await session.helper.revisionContent({
      repositoryRoot: resource.root,
      revision: resource.contentRevision,
      relativePath: resource.contentPath,
    }, token)
    return result.content
  }

  dispose(): void {
    this.cachedContent.clear()
    for (const subscription of this.subscriptions) subscription.dispose()
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
