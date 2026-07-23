package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/yunsang/gitgit/internal/app"
	desktopcore "github.com/yunsang/gitgit/internal/desktop"
)

type DesktopApp struct {
	mu              sync.RWMutex
	context         context.Context
	service         *desktopcore.Service
	projects        *desktopcore.ProjectStore
	projectStoreErr error
	startupOnce     sync.Once
	openProjects    func() (*desktopcore.ProjectStore, error)
	openCache       func() (*desktopcore.PersistentCache, error)
}

type ProjectDiscoveryResult struct {
	Directory string                          `json:"directory"`
	Found     int                             `json:"found"`
	Added     int                             `json:"added"`
	Canceled  bool                            `json:"canceled"`
	Projects  []desktopcore.RegisteredProject `json:"projects"`
}

type ProjectPruneResult struct {
	Removed  []desktopcore.RegisteredProject `json:"removed"`
	Projects []desktopcore.RegisteredProject `json:"projects"`
}

type SearchProgressEvent struct {
	RequestID uint64 `json:"request_id"`
	Scanned   int    `json:"scanned"`
	Total     int    `json:"total"`
}

func NewDesktopApp() *DesktopApp {
	return &DesktopApp{
		openProjects: desktopcore.NewDefaultProjectStore,
		openCache:    desktopcore.OpenDefaultPersistentCache,
	}
}

func (a *DesktopApp) startup(ctx context.Context) {
	a.mu.Lock()
	a.context = ctx
	a.mu.Unlock()

	a.startupOnce.Do(func() {
		projects, projectStoreErr := a.openProjects()
		cache, cacheErr := a.openCache()
		if cacheErr != nil {
			log.Printf("GitGit persistent cache is disabled: %v", cacheErr)
		}
		a.mu.Lock()
		a.service = desktopcore.NewServiceWithCache(nil, cache)
		a.projects = projects
		a.projectStoreErr = projectStoreErr
		a.mu.Unlock()
	})
}

func (a *DesktopApp) shutdown(context.Context) {
	a.mu.RLock()
	service := a.service
	a.mu.RUnlock()
	if service != nil {
		if err := service.Close(); err != nil {
			log.Printf("close GitGit persistent cache: %v", err)
		}
	}
}

func (a *DesktopApp) InitialState() (desktopcore.RepositoryState, error) {
	path, err := repositoryLaunchArgument(os.Args[1:])
	if err != nil {
		return desktopcore.RepositoryState{}, err
	}
	if path != "" {
		return a.openInitialRepository(path)
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return desktopcore.RepositoryState{}, err
	}
	state, workingDirectoryErr := a.openInitialRepository(workingDirectory)
	if workingDirectoryErr == nil {
		return state, nil
	}

	if a.projectStoreErr == nil {
		projects, listErr := a.projects.List()
		if listErr == nil {
			for index := len(projects) - 1; index >= 0; index-- {
				state, openErr := a.openInitialRepository(projects[index].Root)
				if openErr == nil {
					return state, nil
				}
			}
		}
	}
	return desktopcore.RepositoryState{}, workingDirectoryErr
}

func (a *DesktopApp) openInitialRepository(path string) (desktopcore.RepositoryState, error) {
	state, err := a.service.Open(a.appContext(), path)
	if err != nil {
		return desktopcore.RepositoryState{}, err
	}
	if _, err := a.registerProject(state.ProjectRoot); err != nil {
		return desktopcore.RepositoryState{}, err
	}
	return state, nil
}

func repositoryLaunchArgument(arguments []string) (string, error) {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--repository" {
			if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
				return "", errors.New("--repository requires a path")
			}
			return strings.TrimSpace(arguments[index+1]), nil
		}
		if value, found := strings.CutPrefix(argument, "--repository="); found {
			value = strings.TrimSpace(value)
			if value == "" {
				return "", errors.New("--repository requires a path")
			}
			return value, nil
		}
	}
	return "", nil
}

func (a *DesktopApp) ChooseRepository() (desktopcore.RepositoryState, error) {
	defaultDirectory := ""
	if state, err := a.service.Current(a.appContext()); err == nil {
		defaultDirectory = state.Root
	}
	path, err := runtime.OpenDirectoryDialog(a.appContext(), runtime.OpenDialogOptions{
		Title:            "Open Git repository",
		DefaultDirectory: defaultDirectory,
	})
	if err != nil {
		return desktopcore.RepositoryState{}, err
	}
	if path == "" {
		return a.service.Current(a.appContext())
	}
	state, err := a.service.Open(a.appContext(), path)
	if err != nil {
		return desktopcore.RepositoryState{}, err
	}
	if _, err := a.registerProject(state.ProjectRoot); err != nil {
		return desktopcore.RepositoryState{}, err
	}
	return state, nil
}

func (a *DesktopApp) Projects() ([]desktopcore.RegisteredProject, error) {
	if a.projectStoreErr != nil {
		return nil, a.projectStoreErr
	}
	return a.projects.List()
}

func (a *DesktopApp) SetProjectFavorite(root string, favorite bool) ([]desktopcore.RegisteredProject, error) {
	if a.projectStoreErr != nil {
		return nil, a.projectStoreErr
	}
	projects, err := a.projects.SetFavorite(root, favorite)
	if err != nil {
		return nil, fmt.Errorf("update project favorite: %w", err)
	}
	return projects, nil
}

func (a *DesktopApp) RemoveProject(root string) ([]desktopcore.RegisteredProject, error) {
	if a.projectStoreErr != nil {
		return nil, a.projectStoreErr
	}
	projects, err := a.projects.Remove(root)
	if err != nil {
		return nil, fmt.Errorf("unregister project: %w", err)
	}
	return projects, nil
}

func (a *DesktopApp) RemoveUnavailableProjects() (ProjectPruneResult, error) {
	if a.projectStoreErr != nil {
		return ProjectPruneResult{}, a.projectStoreErr
	}
	projects, removed, err := a.projects.PruneUnavailable(a.appContext())
	if err != nil {
		return ProjectPruneResult{}, fmt.Errorf("remove unavailable projects: %w", err)
	}
	return ProjectPruneResult{Removed: removed, Projects: projects}, nil
}

func (a *DesktopApp) ChooseProjectDiscoveryDirectory() (string, error) {
	defaultDirectory := ""
	a.mu.RLock()
	service := a.service
	a.mu.RUnlock()
	if service != nil {
		if state, err := service.Current(a.appContext()); err == nil {
			defaultDirectory = state.ProjectRoot
		}
	}
	return runtime.OpenDirectoryDialog(a.appContext(), runtime.OpenDialogOptions{
		Title:            "Choose directory to discover Git projects",
		DefaultDirectory: defaultDirectory,
	})
}

func (a *DesktopApp) DiscoverProjects(directory string) (ProjectDiscoveryResult, error) {
	if a.projectStoreErr != nil {
		return ProjectDiscoveryResult{}, a.projectStoreErr
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return ProjectDiscoveryResult{}, errors.New("project discovery directory is required")
	}
	path, err := filepath.Abs(directory)
	if err != nil {
		return ProjectDiscoveryResult{}, fmt.Errorf("resolve project discovery directory: %w", err)
	}
	roots, err := desktopcore.DiscoverGitProjects(a.appContext(), path)
	if err != nil {
		return ProjectDiscoveryResult{}, err
	}
	projects, added, err := a.projects.AddMany(roots)
	if err != nil {
		return ProjectDiscoveryResult{}, fmt.Errorf("register discovered projects: %w", err)
	}
	return ProjectDiscoveryResult{Directory: path, Found: len(roots), Added: added, Projects: projects}, nil
}

func (a *DesktopApp) OpenRepository(path string) (desktopcore.RepositoryState, error) {
	return a.service.Open(a.appContext(), path)
}

func (a *DesktopApp) Refresh() (desktopcore.RepositoryState, error) {
	return a.service.Current(a.appContext())
}

func (a *DesktopApp) SyncRemotes() (desktopcore.RemoteSyncResult, error) {
	return a.service.SyncRemotes(a.appContext())
}

func (a *DesktopApp) PullCurrentBranch() (desktopcore.RemoteSyncResult, error) {
	return a.service.PullCurrentBranch(a.appContext())
}

func (a *DesktopApp) Search(request desktopcore.SearchRequest) (desktopcore.SearchResponse, error) {
	ctx := a.appContext()
	return a.service.SearchWithProgress(ctx, request, func(progress app.SearchProgress) {
		runtime.EventsEmit(ctx, "search:progress", SearchProgressEvent{
			RequestID: request.RequestID,
			Scanned:   progress.Scanned,
			Total:     progress.Total,
		})
	})
}

func (a *DesktopApp) History(request desktopcore.HistoryRequest) (desktopcore.HistoryResponse, error) {
	return a.service.History(a.appContext(), request)
}

func (a *DesktopApp) HistoryBranches(commits []string) (desktopcore.HistoryBranchesResponse, error) {
	return a.service.HistoryBranches(a.appContext(), commits)
}

func (a *DesktopApp) CommitDetail(commit, file string) (desktopcore.CommitDetail, error) {
	return a.service.CommitDetail(a.appContext(), commit, file)
}

func (a *DesktopApp) PrepareCommitEdit(commit string) (desktopcore.CommitEditStack, error) {
	return a.service.PrepareCommitEdit(a.appContext(), commit)
}

func (a *DesktopApp) CommitFileContent(commit, file string) (desktopcore.CommitFileContent, error) {
	return a.service.CommitFileContent(a.appContext(), commit, file)
}

func (a *DesktopApp) RewriteCommits(request desktopcore.RewriteCommitsRequest) (desktopcore.RewriteCommitsResponse, error) {
	return a.service.RewriteCommits(a.appContext(), request)
}

func (a *DesktopApp) RepositoryTree(revision, directory string) (desktopcore.RepositoryTreeResponse, error) {
	return a.service.RepositoryTree(a.appContext(), revision, directory)
}

func (a *DesktopApp) CancelSearch() {
	a.service.CancelSearch()
}

func (a *DesktopApp) OpenFile(path string) error {
	resolved, err := a.repositoryFile(path)
	if err != nil {
		return err
	}
	return openPath(a.appContext(), resolved, false)
}

func (a *DesktopApp) RevealFile(path string) error {
	resolved, err := a.repositoryFile(path)
	if err != nil {
		return err
	}
	return openPath(a.appContext(), resolved, true)
}

func (a *DesktopApp) OpenInTerminal(path, terminal string) error {
	resolved, err := a.repositoryFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("inspect terminal path: %w", err)
	}
	if !info.IsDir() {
		resolved = filepath.Dir(resolved)
	}
	return openTerminal(a.appContext(), resolved, terminal)
}

func (a *DesktopApp) OpenExternalURL(value string) error {
	resolved, err := validateExternalURL(value)
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.appContext(), resolved)
	return nil
}

func validateExternalURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("external URL must be an absolute HTTP or HTTPS URL")
	}
	return parsed.String(), nil
}

func (a *DesktopApp) OpenWorktree(path string) error {
	path, err := a.registeredWorktreePath(path)
	if err != nil {
		return err
	}
	return openPath(a.appContext(), path, false)
}

func (a *DesktopApp) OpenWorktreeInTerminal(path, terminal string) error {
	path, err := a.registeredWorktreePath(path)
	if err != nil {
		return err
	}
	return openTerminal(a.appContext(), path, terminal)
}

func (a *DesktopApp) OpenWorktreeInIDE(path, ide string) error {
	path, err := a.registeredWorktreePath(path)
	if err != nil {
		return err
	}
	return openInIDE(a.appContext(), path, "", ide)
}

func (a *DesktopApp) registeredWorktreePath(path string) (string, error) {
	state, err := a.service.Current(a.appContext())
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	for _, worktree := range state.Worktrees {
		if filepath.Clean(worktree.Path) == path {
			return worktree.Path, nil
		}
	}
	return "", errors.New("path is not a registered worktree")
}

func (a *DesktopApp) RemoveMergedWorktree(path string) (desktopcore.RepositoryState, error) {
	return a.service.RemoveMergedWorktree(a.appContext(), path)
}

func (a *DesktopApp) RemoveMergedWorktrees(paths []string) (desktopcore.RepositoryState, error) {
	return a.service.RemoveMergedWorktrees(a.appContext(), paths)
}

func (a *DesktopApp) repositoryFile(path string) (string, error) {
	_, resolved, err := a.repositoryFileContext(path)
	return resolved, err
}

func (a *DesktopApp) repositoryFileContext(path string) (string, string, error) {
	state, err := a.service.Current(a.appContext())
	if err != nil {
		return "", "", err
	}
	if filepath.IsAbs(path) {
		return "", "", errors.New("file path must be repository-relative")
	}
	resolved := filepath.Join(state.Root, filepath.Clean(path))
	relative, err := filepath.Rel(state.Root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("file path escapes the repository")
	}
	if _, err := os.Stat(resolved); err != nil {
		return "", "", fmt.Errorf("file is not present in the current worktree: %w", err)
	}
	return state.Root, resolved, nil
}

func (a *DesktopApp) appContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.context == nil {
		return context.Background()
	}
	return a.context
}

func (a *DesktopApp) registerProject(root string) ([]desktopcore.RegisteredProject, error) {
	if a.projectStoreErr != nil {
		return nil, a.projectStoreErr
	}
	projects, err := a.projects.Add(root)
	if err != nil {
		return nil, fmt.Errorf("register repository: %w", err)
	}
	return projects, nil
}

func openPath(ctx context.Context, path string, reveal bool) error {
	var command *exec.Cmd
	switch stdruntime.GOOS {
	case "darwin":
		if reveal {
			command = exec.CommandContext(ctx, "open", "-R", path)
		} else {
			command = exec.CommandContext(ctx, "open", path)
		}
	case "windows":
		command = exec.CommandContext(ctx, "explorer", path)
	default:
		if reveal {
			path = filepath.Dir(path)
		}
		command = exec.CommandContext(ctx, "xdg-open", path)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open path: %w", err)
	}
	return nil
}

func openInIDE(ctx context.Context, projectRoot, targetPath, ide string) error {
	application, commandName, err := ideCommand(ide)
	if err != nil {
		return err
	}
	var command *exec.Cmd
	paths := ideOpenPaths(projectRoot, targetPath)
	if stdruntime.GOOS == "darwin" {
		arguments := append([]string{"-a", application}, paths...)
		command = exec.CommandContext(ctx, "open", arguments...)
	} else {
		command = exec.CommandContext(ctx, commandName, paths...)
	}
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return fmt.Errorf("open path in %s: %w: %s", application, runErr, strings.TrimSpace(string(output)))
	}
	return nil
}

func openTerminal(ctx context.Context, path, terminal string) error {
	var command *exec.Cmd
	switch stdruntime.GOOS {
	case "darwin":
		application, err := terminalApplication(terminal)
		if err != nil {
			return err
		}
		command = exec.CommandContext(ctx, "open", "-a", application, path)
	case "windows":
		command = exec.CommandContext(ctx, "cmd.exe", "/K", "cd", "/D", path)
	default:
		command = exec.CommandContext(ctx, "x-terminal-emulator", "--working-directory", path)
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("open terminal: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ideOpenPaths(projectRoot, targetPath string) []string {
	projectRoot = filepath.Clean(projectRoot)
	targetPath = filepath.Clean(targetPath)
	paths := []string{projectRoot}
	if targetPath != "." && targetPath != projectRoot {
		paths = append(paths, targetPath)
	}
	return paths
}

func terminalApplication(terminal string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(terminal)) {
	case "terminal":
		return "Terminal", nil
	case "iterm2":
		return "iTerm", nil
	case "warp":
		return "Warp", nil
	case "ghostty":
		return "Ghostty", nil
	case "wezterm":
		return "WezTerm", nil
	default:
		return "", fmt.Errorf("unsupported Terminal application: %s", terminal)
	}
}

func ideCommand(ide string) (application, command string, err error) {
	switch strings.ToLower(strings.TrimSpace(ide)) {
	case "vscode":
		return "Visual Studio Code", "code", nil
	case "cursor":
		return "Cursor", "cursor", nil
	case "zed":
		return "Zed", "zed", nil
	case "idea":
		return "IntelliJ IDEA", "idea", nil
	case "xcode":
		return "Xcode", "xed", nil
	default:
		return "", "", fmt.Errorf("unsupported IDE: %s", ide)
	}
}
