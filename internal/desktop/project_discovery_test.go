package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverGitProjectsFindsNestedRepositoriesAndDeduplicatesWorktrees(t *testing.T) {
	root := t.TempDir()
	mainRepository := filepath.Join(root, "main-project")
	nestedRepository := filepath.Join(mainRepository, "examples", "nested-project")
	ignoredRepository := filepath.Join(root, "node_modules", "ignored-project")
	for _, repository := range []string{mainRepository, nestedRepository, ignoredRepository} {
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatal(err)
		}
		runGit(t, repository, nil, "init", "-q", "-b", "main")
		runGit(t, repository, nil, "config", "user.name", "GitGit Test")
		runGit(t, repository, nil, "config", "user.email", "gitgit@example.com")
		runGit(t, repository, nil, "commit", "-q", "--allow-empty", "-m", "initial")
	}
	linked := filepath.Join(root, "linked-main")
	runGit(t, mainRepository, nil, "worktree", "add", "-q", "--detach", linked)

	projects, err := DiscoverGitProjects(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("discovered projects = %#v, want main and nested", projects)
	}
	seen := map[string]bool{}
	for _, project := range projects {
		seen[filepath.Clean(project)] = true
	}
	wantMain, mainErr := canonicalProjectRoot(mainRepository)
	wantNested, nestedErr := canonicalProjectRoot(nestedRepository)
	if mainErr != nil || nestedErr != nil {
		t.Fatalf("canonicalize expected projects: %v, %v", mainErr, nestedErr)
	}
	if !seen[wantMain] || !seen[wantNested] {
		t.Fatalf("discovered projects = %#v", projects)
	}
}
