package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yunsang/gitgit/internal/gitexec"
)

const projectStoreVersion = 1

type RegisteredProject struct {
	Root     string `json:"root"`
	Name     string `json:"name"`
	Favorite bool   `json:"favorite"`
}

type projectStoreFile struct {
	Version  int                 `json:"version"`
	Projects []RegisteredProject `json:"projects"`
}

type ProjectStore struct {
	path string
	mu   sync.Mutex
}

func NewDefaultProjectStore() (*ProjectStore, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate user config directory: %w", err)
	}
	return NewProjectStore(filepath.Join(configDirectory, "GitGit", "projects.json")), nil
}

func NewProjectStore(path string) *ProjectStore {
	return &ProjectStore{path: filepath.Clean(path)}
}

func (s *ProjectStore) List() ([]RegisteredProject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *ProjectStore) Add(root string) ([]RegisteredProject, error) {
	root, err := canonicalProjectRoot(root)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	projects, err := s.read()
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if project.Root == root {
			return projects, nil
		}
	}
	projects = append(projects, RegisteredProject{Root: root, Name: filepath.Base(root)})
	if err := s.write(projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *ProjectStore) AddMany(roots []string) ([]RegisteredProject, int, error) {
	canonicalRoots := make([]string, 0, len(roots))
	seenInput := make(map[string]bool, len(roots))
	for _, root := range roots {
		canonical, err := canonicalProjectRoot(root)
		if err != nil {
			return nil, 0, err
		}
		if !seenInput[canonical] {
			seenInput[canonical] = true
			canonicalRoots = append(canonicalRoots, canonical)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	projects, err := s.read()
	if err != nil {
		return nil, 0, err
	}
	registered := make(map[string]bool, len(projects))
	for _, project := range projects {
		registered[project.Root] = true
	}
	added := 0
	for _, root := range canonicalRoots {
		if registered[root] {
			continue
		}
		projects = append(projects, RegisteredProject{Root: root, Name: filepath.Base(root)})
		registered[root] = true
		added++
	}
	if added > 0 {
		if err := s.write(projects); err != nil {
			return nil, 0, err
		}
	}
	return projects, added, nil
}

func (s *ProjectStore) SetFavorite(root string, favorite bool) ([]RegisteredProject, error) {
	root, err := canonicalProjectRoot(root)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	projects, err := s.read()
	if err != nil {
		return nil, err
	}
	for index := range projects {
		if projects[index].Root != root {
			continue
		}
		if projects[index].Favorite == favorite {
			return projects, nil
		}
		projects[index].Favorite = favorite
		if err := s.write(projects); err != nil {
			return nil, err
		}
		return projects, nil
	}
	return nil, errors.New("project is not registered")
}

func (s *ProjectStore) Remove(root string) ([]RegisteredProject, error) {
	root, err := projectRootLookupKey(root)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	projects, err := s.read()
	if err != nil {
		return nil, err
	}
	for index, project := range projects {
		if project.Root != root {
			continue
		}
		projects = append(projects[:index:index], projects[index+1:]...)
		if err := s.write(projects); err != nil {
			return nil, err
		}
		return projects, nil
	}
	return nil, errors.New("project is not registered")
}

// PruneUnavailable removes registered entries whose root no longer exists or
// no longer resolves to that project's Git worktree. It never removes a
// repository or worktree from disk.
func (s *ProjectStore) PruneUnavailable(ctx context.Context) ([]RegisteredProject, []RegisteredProject, error) {
	runner := gitexec.NewRunner()
	if _, err := runner.Version(ctx); err != nil {
		return nil, nil, fmt.Errorf("verify Git availability before pruning projects: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	projects, err := s.read()
	if err != nil {
		return nil, nil, err
	}

	retained := make([]RegisteredProject, 0, len(projects))
	removed := make([]RegisteredProject, 0)
	for _, project := range projects {
		available, availabilityErr := registeredProjectAvailable(ctx, runner, project.Root)
		if availabilityErr != nil {
			return nil, nil, availabilityErr
		}
		if available {
			retained = append(retained, project)
			continue
		}
		removed = append(removed, project)
	}
	if len(removed) > 0 {
		if err := s.write(retained); err != nil {
			return nil, nil, err
		}
	}
	return retained, removed, nil
}

func registeredProjectAvailable(ctx context.Context, runner *gitexec.Runner, root string) (bool, error) {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !info.IsDir()) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect registered project %q: %w", root, err)
	}

	repository, err := gitexec.OpenRepository(ctx, runner, root)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	registeredRoot, err := projectRootLookupKey(root)
	if err != nil {
		return false, err
	}
	repositoryRoot, err := projectRootLookupKey(repository.Root)
	if err != nil {
		return false, err
	}
	return registeredRoot == repositoryRoot, nil
}

func (s *ProjectStore) read() ([]RegisteredProject, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []RegisteredProject{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registered projects: %w", err)
	}
	var stored projectStoreFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("decode registered projects: %w", err)
	}
	if stored.Version != projectStoreVersion {
		return nil, fmt.Errorf("unsupported project store version %d", stored.Version)
	}
	seen := make(map[string]bool, len(stored.Projects))
	projects := make([]RegisteredProject, 0, len(stored.Projects))
	for _, project := range stored.Projects {
		root := filepath.Clean(strings.TrimSpace(project.Root))
		if root == "." || seen[root] {
			continue
		}
		name := strings.TrimSpace(project.Name)
		if name == "" {
			name = filepath.Base(root)
		}
		seen[root] = true
		projects = append(projects, RegisteredProject{Root: root, Name: name, Favorite: project.Favorite})
	}
	return projects, nil
}

func (s *ProjectStore) write(projects []RegisteredProject) error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create project store directory: %w", err)
	}
	data, err := json.MarshalIndent(projectStoreFile{Version: projectStoreVersion, Projects: projects}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode registered projects: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, ".projects-*.json")
	if err != nil {
		return fmt.Errorf("create project store update: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure project store update: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write project store update: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync project store update: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close project store update: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace project store: %w", err)
	}
	return nil
}

func canonicalProjectRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("project root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project root symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect project root: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("project root must be a directory")
	}
	return filepath.Clean(resolved), nil
}

func projectRootLookupKey(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("project root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(absolute), nil
}
