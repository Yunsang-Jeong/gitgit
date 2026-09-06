import type {
  CreateWorktreeRequest,
  MoveWorktreeRequest,
  WorktreeMutationResult, CommitDetail, CommitEditStack, CommitEditTarget, CommitFileContent, HistoryBranchesResponse, HistoryRequest, HistoryResponse, ProjectDiscoveryResult, ProjectPruneResult, RegisteredProject, RemoteBranchesResponse, RemoteSyncResult, RepositoryState, RepositoryTreeResponse, RewriteCommitsRequest, RewriteCommitsResponse, SearchProgress, SearchRequest, SearchResponse } from './types'

type Backend = {
  Version: () => Promise<string>
  InitialState: () => Promise<RepositoryState>
  Projects: () => Promise<RegisteredProject[]>
  SetProjectFavorite: (root: string, favorite: boolean) => Promise<RegisteredProject[]>
  RemoveProject: (root: string) => Promise<RegisteredProject[]>
  RemoveUnavailableProjects: () => Promise<ProjectPruneResult>
  ChooseProjectDiscoveryDirectory: () => Promise<string>
  DiscoverProjects: (directory: string) => Promise<ProjectDiscoveryResult>
  ChooseRepository: () => Promise<RepositoryState>
  OpenRepository: (path: string) => Promise<RepositoryState>
  Refresh: () => Promise<RepositoryState>
  SyncRemotes: () => Promise<RemoteSyncResult>
  PullCurrentBranch: () => Promise<RemoteSyncResult>
  RemoteBranches: (remote: string) => Promise<RemoteBranchesResponse>
  History: (request: HistoryRequest) => Promise<HistoryResponse>
  HistoryBranches: (commits: string[]) => Promise<HistoryBranchesResponse>
  CommitDetail: (commit: string, file: string) => Promise<CommitDetail>
  PrepareCommitEdit: (commit: string, expectedBranch: string) => Promise<CommitEditStack>
  CommitEditTarget: (branch: string) => Promise<CommitEditTarget>
  CommitFileContent: (commit: string, file: string) => Promise<CommitFileContent>
  RewriteCommits: (request: RewriteCommitsRequest) => Promise<RewriteCommitsResponse>
  RepositoryTree: (revision: string, directory: string) => Promise<RepositoryTreeResponse>
  Search: (request: SearchRequest) => Promise<SearchResponse>
  CancelSearch: () => Promise<void>
  OpenFile: (path: string) => Promise<void>
  RevealFile: (path: string) => Promise<void>
  OpenInTerminal: (path: string, terminal: string) => Promise<void>
  OpenExternalURL: (url: string) => Promise<void>
  OpenWorktree: (path: string) => Promise<void>
  OpenWorktreeInTerminal: (path: string, terminal: string) => Promise<void>
  OpenWorktreeInIDE: (path: string, ide: string) => Promise<void>
  RemoveMergedWorktree: (path: string) => Promise<RepositoryState>
  RemoveMergedWorktrees: (paths: string[]) => Promise<RepositoryState>
  CreateWorktree: (request: CreateWorktreeRequest) => Promise<WorktreeMutationResult>
  MoveWorktree: (request: MoveWorktreeRequest) => Promise<WorktreeMutationResult>
  SetWorktreeSparseDirectories: (path: string, directories: string[]) => Promise<WorktreeMutationResult>
  ExpandWorktreeSparseDirectories: (path: string, directories: string[]) => Promise<WorktreeMutationResult>
  ContractWorktreeSparseDirectories: (path: string, directories: string[]) => Promise<WorktreeMutationResult>
  DisableWorktreeSparseCheckout: (path: string) => Promise<WorktreeMutationResult>
  SuggestedWorktreeParentDirectory: () => Promise<string>
  ChooseWorktreeParentDirectory: (current: string) => Promise<string>
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
  removeProject: (root: string) => backend().RemoveProject(root),
  removeUnavailableProjects: () => backend().RemoveUnavailableProjects(),
  chooseProjectDiscoveryDirectory: () => backend().ChooseProjectDiscoveryDirectory(),
  discoverProjects: (directory: string) => backend().DiscoverProjects(directory),
  chooseRepository: () => backend().ChooseRepository(),
  openRepository: (path: string) => backend().OpenRepository(path),
  refresh: () => backend().Refresh(),
  syncRemotes: () => backend().SyncRemotes(),
  pullCurrentBranch: () => backend().PullCurrentBranch(),
  remoteBranches: (remote: string) => backend().RemoteBranches(remote),
  history: (request: HistoryRequest) => backend().History(request),
  historyBranches: (commits: string[]) => backend().HistoryBranches(commits),
  commitDetail: (commit: string, file = '') => backend().CommitDetail(commit, file),
  prepareCommitEdit: (commit: string, expectedBranch: string) => backend().PrepareCommitEdit(commit, expectedBranch),
  commitEditTarget: (branch: string) => backend().CommitEditTarget(branch),
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
  revealFile: (path: string) => backend().RevealFile(path),
  openInTerminal: (path: string, terminal: string) => backend().OpenInTerminal(path, terminal),
  openExternalURL: (url: string) => backend().OpenExternalURL(url),
  openWorktree: (path: string) => backend().OpenWorktree(path),
  openWorktreeInTerminal: (path: string, terminal: string) => backend().OpenWorktreeInTerminal(path, terminal),
  openWorktreeInIDE: (path: string, ide: string) => backend().OpenWorktreeInIDE(path, ide),
  removeMergedWorktree: (path: string) => backend().RemoveMergedWorktree(path),
  removeMergedWorktrees: (paths: string[]) => backend().RemoveMergedWorktrees(paths),
  createWorktree: (request: CreateWorktreeRequest) => backend().CreateWorktree(request),
  moveWorktree: (request: MoveWorktreeRequest) => backend().MoveWorktree(request),
  setWorktreeSparseDirectories: (path: string, directories: string[]) => backend().SetWorktreeSparseDirectories(path, directories),
  expandWorktreeSparseDirectories: (path: string, directories: string[]) => backend().ExpandWorktreeSparseDirectories(path, directories),
  contractWorktreeSparseDirectories: (path: string, directories: string[]) => backend().ContractWorktreeSparseDirectories(path, directories),
  disableWorktreeSparseCheckout: (path: string) => backend().DisableWorktreeSparseCheckout(path),
  suggestedWorktreeParentDirectory: () => backend().SuggestedWorktreeParentDirectory(),
  chooseWorktreeParentDirectory: (current: string) => backend().ChooseWorktreeParentDirectory(current),
}
