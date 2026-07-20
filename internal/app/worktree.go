package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yunsang/gitgit/internal/apperr"
	"github.com/yunsang/gitgit/internal/gitexec"
)

type SparseState struct {
	Enabled     bool     `json:"enabled"`
	Cone        bool     `json:"cone"`
	Directories []string `json:"directories,omitempty"`
}

type WorktreeInfo struct {
	Path              string      `json:"path"`
	Head              string      `json:"head"`
	Branch            string      `json:"branch,omitempty"`
	Detached          bool        `json:"detached"`
	Bare              bool        `json:"bare"`
	Locked            bool        `json:"locked"`
	Prunable          bool        `json:"prunable"`
	Dirty             bool        `json:"dirty"`
	MergedIntoDefault bool        `json:"merged_into_default"`
	Sparse            SparseState `json:"sparse"`
}

type AddWorktreeOptions struct {
	Path      string
	Revision  string
	NewBranch string
	Detach    bool
	Sync      string
}

type WorktreeMutation struct {
	Path     string   `json:"path"`
	Action   string   `json:"action"`
	Warnings []string `json:"warnings,omitempty"`
}

type WorktreeService struct {
	repo *gitexec.Repository
}

func NewWorktreeService(repo *gitexec.Repository) *WorktreeService {
	return &WorktreeService{repo: repo}
}

func (s *WorktreeService) List(ctx context.Context) ([]WorktreeInfo, error) {
	out, err := s.repo.Run(ctx, nil, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, worktreeGitError("list worktrees", err)
	}
	items, err := parseWorktreeList(out)
	if err != nil {
		return nil, apperr.Wrap("worktree_parse_error", "failed to parse worktree list", apperr.ExitFailure, err, nil)
	}
	for index := range items {
		status, statusErr := s.repo.Runner.Run(ctx, items[index].Path, nil, "status", "--porcelain=v2", "-z")
		if statusErr == nil {
			items[index].Dirty = len(status) > 0
		}
		items[index].Sparse = s.sparseState(ctx, items[index].Path)
	}
	return items, nil
}

func parseWorktreeList(data []byte) ([]WorktreeInfo, error) {
	tokens := bytes.Split(data, []byte{0})
	items := make([]WorktreeInfo, 0, 4)
	var current *WorktreeInfo
	for _, raw := range tokens {
		line := string(raw)
		if line == "" {
			if current != nil {
				items = append(items, *current)
				current = nil
			}
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		if key == "worktree" {
			if current != nil {
				items = append(items, *current)
			}
			current = &WorktreeInfo{Path: value}
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("attribute %q appeared before worktree path", line)
		}
		switch key {
		case "HEAD":
			current.Head = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			current.Detached = true
		case "bare":
			current.Bare = true
		case "locked":
			current.Locked = true
		case "prunable":
			current.Prunable = true
		}
	}
	if current != nil {
		items = append(items, *current)
	}
	return items, nil
}

func (s *WorktreeService) Add(ctx context.Context, options AddWorktreeOptions) (WorktreeMutation, error) {
	if options.Path == "" {
		return WorktreeMutation{}, apperr.New("missing_worktree", "worktree path is required", apperr.ExitUsage, nil)
	}
	if options.NewBranch != "" && options.Detach {
		return WorktreeMutation{}, apperr.New("invalid_arguments", "new branch and detached mode cannot be used together", apperr.ExitUsage, nil)
	}
	warnings, err := s.syncBeforeAdd(ctx, options.Sync, options.Revision)
	if err != nil {
		return WorktreeMutation{}, err
	}
	args := []string{"worktree", "add"}
	if options.NewBranch != "" {
		args = append(args, "-b", options.NewBranch)
	}
	if options.Detach {
		args = append(args, "--detach")
	}
	args = append(args, options.Path)
	if options.Revision != "" {
		args = append(args, options.Revision)
	}
	if _, err := s.repo.Run(ctx, nil, args...); err != nil {
		return WorktreeMutation{}, worktreeGitError("add worktree", err)
	}
	path := options.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.repo.Root, path)
	}
	path, _ = filepath.Abs(path)
	return WorktreeMutation{Path: path, Action: "added", Warnings: warnings}, nil
}

func (s *WorktreeService) syncBeforeAdd(ctx context.Context, mode, explicitRevision string) ([]string, error) {
	if mode == "" {
		mode = "ff-only"
	}
	switch mode {
	case "ff-only", "fetch", "none":
	default:
		return nil, apperr.New("invalid_sync", fmt.Sprintf("unsupported sync mode %q", mode), apperr.ExitUsage, map[string]any{"allowed": []string{"ff-only", "fetch", "none"}})
	}
	if mode == "none" {
		return nil, nil
	}
	remoteOutput, err := s.repo.Run(ctx, nil, "remote")
	if err != nil {
		return nil, worktreeGitError("list remotes", err)
	}
	if len(strings.Fields(string(remoteOutput))) == 0 {
		return []string{"no remotes configured; worktree created from local history"}, nil
	}
	if _, err := s.repo.Run(ctx, nil, "fetch", "--all"); err != nil {
		return nil, apperr.Wrap("remote_sync_failed", "failed to fetch all remotes; worktree was not created", apperr.ExitPrecondition, err, nil)
	}
	if mode == "fetch" || explicitRevision != "" {
		return nil, nil
	}
	if _, err := s.repo.Run(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD"); err != nil {
		return []string{"current HEAD is detached; fetched remotes but did not fast-forward"}, nil
	}
	upstream, err := s.repo.Run(ctx, nil, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return []string{"current branch has no upstream; fetched remotes but did not fast-forward"}, nil
	}
	upstreamName := strings.TrimSpace(string(upstream))
	counts, err := s.repo.Run(ctx, nil, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return nil, worktreeGitError("compare current branch with upstream", err)
	}
	fields := strings.Fields(string(counts))
	if len(fields) != 2 {
		return nil, apperr.New("remote_sync_parse_error", "Git returned an invalid ahead/behind count", apperr.ExitFailure, map[string]any{"output": strings.TrimSpace(string(counts))})
	}
	localAhead, _ := strconv.Atoi(fields[0])
	remoteAhead, _ := strconv.Atoi(fields[1])
	if localAhead > 0 && remoteAhead > 0 {
		return nil, apperr.New("branch_diverged", "current branch has diverged from its upstream; worktree was not created", apperr.ExitPrecondition, map[string]any{"upstream": upstreamName, "local_ahead": localAhead, "remote_ahead": remoteAhead})
	}
	warnings := []string{}
	if remoteAhead > 0 {
		status, statusErr := s.repo.Run(ctx, nil, "status", "--porcelain=v2", "-z")
		if statusErr != nil {
			return nil, worktreeGitError("check current worktree status", statusErr)
		}
		if len(status) > 0 {
			return nil, apperr.New("dirty_worktree", "current worktree is dirty and requires a fast-forward; worktree was not created", apperr.ExitPrecondition, map[string]any{"upstream": upstreamName})
		}
		if _, err := s.repo.Run(ctx, nil, "merge", "--ff-only", "@{upstream}"); err != nil {
			return nil, apperr.Wrap("fast_forward_failed", "failed to fast-forward current branch; worktree was not created", apperr.ExitPrecondition, err, map[string]any{"upstream": upstreamName})
		}
	}
	if localAhead > 0 {
		warnings = append(warnings, fmt.Sprintf("current branch is %d commit(s) ahead of %s", localAhead, upstreamName))
	}
	return warnings, nil
}

func (s *WorktreeService) Move(ctx context.Context, source, destination string) (WorktreeMutation, error) {
	if source == "" || destination == "" {
		return WorktreeMutation{}, apperr.New("missing_worktree_path", "source and destination paths are required", apperr.ExitUsage, nil)
	}
	if _, err := s.repo.Run(ctx, nil, "worktree", "move", source, destination); err != nil {
		return WorktreeMutation{}, worktreeGitError("move worktree", err)
	}
	return WorktreeMutation{Path: destination, Action: "moved"}, nil
}

func (s *WorktreeService) Remove(ctx context.Context, path string, confirmed bool) (WorktreeMutation, error) {
	if path == "" {
		return WorktreeMutation{}, apperr.New("missing_worktree", "worktree path is required", apperr.ExitUsage, nil)
	}
	if !confirmed {
		return WorktreeMutation{}, apperr.New("confirmation_required", "worktree removal requires confirmation", apperr.ExitConfirmation, map[string]any{"path": path})
	}
	if _, err := s.repo.Run(ctx, nil, "worktree", "remove", path); err != nil {
		return WorktreeMutation{}, worktreeGitError("remove worktree", err)
	}
	return WorktreeMutation{Path: path, Action: "removed"}, nil
}

func (s *WorktreeService) SparseList(ctx context.Context, target string) (SparseState, error) {
	target = s.targetPath(target)
	state := s.sparseState(ctx, target)
	return state, nil
}

func (s *WorktreeService) SparseSet(ctx context.Context, target string, directories []string) (SparseState, error) {
	target = s.targetPath(target)
	directories, err := normalizeSparseDirectories(directories)
	if err != nil {
		return SparseState{}, err
	}
	current := s.sparseState(ctx, target)
	if current.Enabled && !current.Cone {
		return SparseState{}, apperr.New("non_cone_unsupported", "GitGit only manages cone-mode sparse checkouts; disable the existing non-cone checkout first", apperr.ExitPrecondition, nil)
	}
	if current.Enabled {
		if err := s.protectDirtyPaths(ctx, target, directories); err != nil {
			return SparseState{}, err
		}
	}
	args := []string{"sparse-checkout", "set", "--cone"}
	if !current.Enabled {
		args = append(args, "--no-sparse-index")
	}
	args = append(args, "--stdin")
	if _, err := s.repo.Runner.Run(ctx, target, strings.NewReader(strings.Join(directories, "\n")+"\n"), args...); err != nil {
		return SparseState{}, worktreeGitError("set sparse directories", err)
	}
	return s.sparseState(ctx, target), nil
}

func (s *WorktreeService) SparseExpand(ctx context.Context, target string, directories []string) (SparseState, error) {
	target = s.targetPath(target)
	directories, err := normalizeSparseDirectories(directories)
	if err != nil {
		return SparseState{}, err
	}
	if len(directories) == 0 {
		return SparseState{}, apperr.New("missing_directories", "at least one sparse directory is required", apperr.ExitUsage, nil)
	}
	current := s.sparseState(ctx, target)
	if current.Enabled && !current.Cone {
		return SparseState{}, apperr.New("non_cone_unsupported", "GitGit only manages cone-mode sparse checkouts", apperr.ExitPrecondition, nil)
	}
	if !current.Enabled {
		return s.SparseSet(ctx, target, directories)
	}
	if _, err := s.repo.Runner.Run(ctx, target, strings.NewReader(strings.Join(directories, "\n")+"\n"), "sparse-checkout", "add", "--stdin"); err != nil {
		return SparseState{}, worktreeGitError("expand sparse directories", err)
	}
	return s.sparseState(ctx, target), nil
}

func (s *WorktreeService) SparseContract(ctx context.Context, target string, directories []string) (SparseState, error) {
	target = s.targetPath(target)
	directories, err := normalizeSparseDirectories(directories)
	if err != nil {
		return SparseState{}, err
	}
	if len(directories) == 0 {
		return SparseState{}, apperr.New("missing_directories", "at least one sparse directory is required", apperr.ExitUsage, nil)
	}
	current := s.sparseState(ctx, target)
	if !current.Enabled {
		return SparseState{}, apperr.New("sparse_disabled", "sparse-checkout is not enabled", apperr.ExitPrecondition, nil)
	}
	if !current.Cone {
		return SparseState{}, apperr.New("non_cone_unsupported", "GitGit only manages cone-mode sparse checkouts", apperr.ExitPrecondition, nil)
	}
	remove := map[string]bool{}
	for _, directory := range directories {
		remove[directory] = true
	}
	next := make([]string, 0, len(current.Directories))
	for _, directory := range current.Directories {
		if !remove[directory] {
			next = append(next, directory)
		}
	}
	if len(next) == len(current.Directories) {
		return SparseState{}, apperr.New("sparse_directory_missing", "none of the requested directories are in the sparse specification", apperr.ExitPrecondition, map[string]any{"directories": directories})
	}
	return s.SparseSet(ctx, target, next)
}

func (s *WorktreeService) SparseDisable(ctx context.Context, target string) (SparseState, error) {
	target = s.targetPath(target)
	if _, err := s.repo.Runner.Run(ctx, target, nil, "sparse-checkout", "disable"); err != nil {
		return SparseState{}, worktreeGitError("disable sparse-checkout", err)
	}
	return SparseState{Enabled: false}, nil
}

func (s *WorktreeService) sparseState(ctx context.Context, target string) SparseState {
	out, err := s.repo.Runner.Run(ctx, target, nil, "config", "--bool", "core.sparseCheckout")
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return SparseState{Enabled: false}
	}
	coneOut, coneErr := s.repo.Runner.Run(ctx, target, nil, "config", "--bool", "core.sparseCheckoutCone")
	cone := coneErr == nil && strings.TrimSpace(string(coneOut)) == "true"
	list, listErr := s.repo.Runner.Run(ctx, target, nil, "sparse-checkout", "list")
	if listErr != nil {
		return SparseState{Enabled: true, Cone: cone}
	}
	trimmed := strings.TrimRight(string(list), "\n")
	directories := []string{}
	if trimmed != "" {
		directories = strings.Split(trimmed, "\n")
	}
	sort.Strings(directories)
	return SparseState{Enabled: true, Cone: cone, Directories: directories}
}

func (s *WorktreeService) protectDirtyPaths(ctx context.Context, target string, directories []string) error {
	out, err := s.repo.Runner.Run(ctx, target, nil, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return worktreeGitError("check sparse worktree status", err)
	}
	dirty := parsePorcelainPaths(out)
	if len(dirty) == 0 {
		return nil
	}
	rules, err := os.CreateTemp("", "gitgit-sparse-rules-*.txt")
	if err != nil {
		return apperr.Wrap("sparse_rules_error", "failed to create temporary sparse rules", apperr.ExitFailure, err, nil)
	}
	rulesPath := rules.Name()
	defer os.Remove(rulesPath)
	if _, err := rules.WriteString(strings.Join(directories, "\n") + "\n"); err != nil {
		rules.Close()
		return err
	}
	if err := rules.Close(); err != nil {
		return err
	}
	input := strings.Join(dirty, "\x00") + "\x00"
	matchedOut, err := s.repo.Runner.Run(ctx, target, strings.NewReader(input), "sparse-checkout", "check-rules", "--rules-file", rulesPath, "-z")
	if err != nil {
		return apperr.Wrap("sparse_rules_unsupported", "Git cannot validate the proposed sparse rules", apperr.ExitPrecondition, err, nil)
	}
	matched := map[string]bool{}
	for _, raw := range bytes.Split(matchedOut, []byte{0}) {
		if len(raw) > 0 {
			matched[string(raw)] = true
		}
	}
	outside := make([]string, 0)
	for _, path := range dirty {
		if !matched[path] {
			outside = append(outside, path)
		}
	}
	if len(outside) > 0 {
		return apperr.New("dirty_paths_outside_sparse", "modified or untracked paths would fall outside the new sparse specification", apperr.ExitPrecondition, map[string]any{"paths": outside})
	}
	return nil
}

func parsePorcelainPaths(data []byte) []string {
	tokens := bytes.Split(data, []byte{0})
	paths := make([]string, 0, len(tokens))
	for index := 0; index < len(tokens); index++ {
		record := string(tokens[index])
		if len(record) < 4 {
			continue
		}
		paths = append(paths, record[3:])
		if record[0] == 'R' || record[1] == 'R' || record[0] == 'C' || record[1] == 'C' {
			if index+1 < len(tokens) && len(tokens[index+1]) > 0 {
				index++
				paths = append(paths, string(tokens[index]))
			}
		}
	}
	return paths
}

func normalizeSparseDirectories(directories []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(directories))
	for _, directory := range directories {
		if strings.ContainsAny(directory, "\r\n\x00") {
			return nil, apperr.New("invalid_sparse_directory", "sparse directory cannot contain a newline or NUL byte", apperr.ExitUsage, nil)
		}
		directory = filepath.ToSlash(filepath.Clean(strings.TrimSpace(directory)))
		if directory == "" || directory == "." {
			continue
		}
		if filepath.IsAbs(directory) || directory == ".." || strings.HasPrefix(directory, "../") {
			return nil, apperr.New("invalid_sparse_directory", fmt.Sprintf("sparse directory must be repository-relative: %s", directory), apperr.ExitUsage, nil)
		}
		if !seen[directory] {
			seen[directory] = true
			result = append(result, directory)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (s *WorktreeService) targetPath(target string) string {
	if target == "" {
		return s.repo.Root
	}
	return target
}

func worktreeGitError(action string, err error) error {
	return apperr.Wrap("git_worktree_error", fmt.Sprintf("failed to %s", action), apperr.ExitFailure, err, nil)
}
