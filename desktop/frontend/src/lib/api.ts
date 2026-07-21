import type { CommitDetail, CommitEditStack, CommitFileContent, HistoryBranchesResponse, HistoryRequest, HistoryResponse, ProjectDiscoveryResult, RegisteredProject, RemoteSyncResult, RepositoryState, RepositoryTreeResponse, RewriteCommitsRequest, RewriteCommitsResponse, SearchProgress, SearchRequest, SearchResponse } from './types'

type Backend = {
  Version: () => Promise<string>
  InitialState: () => Promise<RepositoryState>
  Projects: () => Promise<RegisteredProject[]>
  SetProjectFavorite: (root: string, favorite: boolean) => Promise<RegisteredProject[]>
  DiscoverProjects: () => Promise<ProjectDiscoveryResult>
  ChooseRepository: () => Promise<RepositoryState>
  OpenRepository: (path: string) => Promise<RepositoryState>
  Refresh: () => Promise<RepositoryState>
  SyncRemotes: () => Promise<RemoteSyncResult>
  History: (request: HistoryRequest) => Promise<HistoryResponse>
  HistoryBranches: (commits: string[]) => Promise<HistoryBranchesResponse>
  CommitDetail: (commit: string, file: string) => Promise<CommitDetail>
  PrepareCommitEdit: (commit: string) => Promise<CommitEditStack>
  CommitFileContent: (commit: string, file: string) => Promise<CommitFileContent>
  RewriteCommits: (request: RewriteCommitsRequest) => Promise<RewriteCommitsResponse>
  RepositoryTree: (revision: string, directory: string) => Promise<RepositoryTreeResponse>
  Search: (request: SearchRequest) => Promise<SearchResponse>
  CancelSearch: () => Promise<void>
  OpenFile: (path: string) => Promise<void>
  OpenFileInIDE: (path: string, ide: string) => Promise<void>
  RevealFile: (path: string) => Promise<void>
  OpenInTerminal: (path: string, terminal: string) => Promise<void>
  OpenExternalURL: (url: string) => Promise<void>
  OpenWorktree: (path: string) => Promise<void>
  OpenWorktreeInIDE: (path: string, ide: string) => Promise<void>
  RemoveMergedWorktree: (path: string) => Promise<RepositoryState>
  RemoveMergedWorktrees: (paths: string[]) => Promise<RepositoryState>
}

declare global {
  interface Window {
    go?: {
      main?: {
        DesktopApp?: Backend
      }
    }
    runtime?: {
      EventsOnMultiple: (eventName: string, callback: (...data: unknown[]) => void, maxCallbacks: number) => () => void
    }
  }
}

function backend(): Backend {
  const binding = window.go?.main?.DesktopApp
  if (!binding) {
    throw new Error('GitGit desktop bridge is unavailable. Run the UI through Wails.')
  }
  return binding
}

export const api = {
  available: () => Boolean(window.go?.main?.DesktopApp),
  version: () => backend().Version(),
  initialState: () => backend().InitialState(),
  projects: () => backend().Projects(),
  setProjectFavorite: (root: string, favorite: boolean) => backend().SetProjectFavorite(root, favorite),
  discoverProjects: () => backend().DiscoverProjects(),
  chooseRepository: () => backend().ChooseRepository(),
  openRepository: (path: string) => backend().OpenRepository(path),
  refresh: () => backend().Refresh(),
  syncRemotes: () => backend().SyncRemotes(),
  history: (request: HistoryRequest) => backend().History(request),
  historyBranches: (commits: string[]) => backend().HistoryBranches(commits),
  commitDetail: (commit: string, file = '') => backend().CommitDetail(commit, file),
  prepareCommitEdit: (commit: string) => backend().PrepareCommitEdit(commit),
  commitFileContent: (commit: string, file: string) => backend().CommitFileContent(commit, file),
  rewriteCommits: (request: RewriteCommitsRequest) => backend().RewriteCommits(request),
  repositoryTree: (revision: string, directory = '') => backend().RepositoryTree(revision, directory),
  search: (request: SearchRequest) => backend().Search(request),
  onSearchProgress: (callback: (progress: SearchProgress) => void) => {
    if (!window.runtime) return () => undefined
    return window.runtime.EventsOnMultiple('search:progress', (progress) => callback(progress as SearchProgress), -1)
  },
  cancelSearch: () => backend().CancelSearch(),
  openFile: (path: string) => backend().OpenFile(path),
  openFileInIDE: (path: string, ide: string) => backend().OpenFileInIDE(path, ide),
  revealFile: (path: string) => backend().RevealFile(path),
  openInTerminal: (path: string, terminal: string) => backend().OpenInTerminal(path, terminal),
  openExternalURL: (url: string) => backend().OpenExternalURL(url),
  openWorktree: (path: string) => backend().OpenWorktree(path),
  openWorktreeInIDE: (path: string, ide: string) => backend().OpenWorktreeInIDE(path, ide),
  removeMergedWorktree: (path: string) => backend().RemoveMergedWorktree(path),
  removeMergedWorktrees: (paths: string[]) => backend().RemoveMergedWorktrees(paths),
}
