package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yunsang/gitgit/internal/gitexec"
)

var ignoredProjectDiscoveryDirectories = map[string]bool{
	".git":         true,
	".cache":       true,
	".Trash":       true,
	"node_modules": true,
}

func DiscoverGitProjects(ctx context.Context, root string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("project discovery directory is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve project discovery directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect project discovery directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("project discovery path must be a directory")
	}

	runner := gitexec.NewRunner()
	seen := map[string]bool{}
	projects := make([]string, 0)
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrPermission) && entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path != absolute && ignoredProjectDiscoveryDirectories[entry.Name()] {
			return filepath.SkipDir
		}
		if _, statErr := os.Lstat(filepath.Join(path, ".git")); statErr != nil {
			return nil
		}
		out, gitErr := runner.Run(ctx, path, nil, "worktree", "list", "--porcelain")
		if gitErr != nil {
			return nil
		}
		firstLine, _, _ := strings.Cut(string(out), "\n")
		mainWorktree := strings.TrimSpace(strings.TrimPrefix(firstLine, "worktree "))
		if mainWorktree == "" || mainWorktree == firstLine {
			return nil
		}
		mainWorktree, gitErr = canonicalProjectRoot(mainWorktree)
		if gitErr == nil && !seen[mainWorktree] {
			seen[mainWorktree] = true
			projects = append(projects, mainWorktree)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover Git projects: %w", err)
	}
	sort.Strings(projects)
	return projects, nil
}
